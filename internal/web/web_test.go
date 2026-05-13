package web

import (
	"context"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/levino/go-ranking/internal/auth"
	"github.com/levino/go-ranking/internal/service"
	"github.com/levino/go-ranking/internal/store"
)

func newTestWebServer(t *testing.T) (*httptest.Server, *service.Service, *auth.Signer) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "w.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := service.New(st)
	keyHex := strings.Repeat("b", 64)
	keyBytes, _ := hex.DecodeString(keyHex)
	signer := auth.NewSigner(keyBytes)
	oidc := auth.NewOIDC("https://example.invalid", "id", "secret", "https://example.test/auth/callback")
	srv, err := New(svc, signer, oidc)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, svc, signer
}

// loggedInClient mints a session cookie via the signer (skipping the OIDC
// dance) and returns an http.Client carrying it.
func loggedInClient(t *testing.T, ts *httptest.Server, signer *auth.Signer, userID int64) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	rec := httptest.NewRecorder()
	if err := signer.Issue(rec, userID, time.Hour); err != nil {
		t.Fatal(err)
	}
	srvURL, _ := url.Parse(ts.URL)
	jar.SetCookies(srvURL, rec.Result().Cookies())
	return c
}

func TestLoginRedirectsToAuthStart(t *testing.T) {
	// Single OIDC provider → /login goes straight into the auth flow.
	ts, _, _ := newTestWebServer(t)
	c := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := c.Get(ts.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/auth/start" {
		t.Errorf("got %d -> %s", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestUnauthenticatedRedirectsToLogin(t *testing.T) {
	ts, _, _ := newTestWebServer(t)
	c := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := c.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/login" {
		t.Errorf("got %d -> %s", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestIndexShowsMCPURL(t *testing.T) {
	ts, svc, signer := newTestWebServer(t)
	ctx := context.Background()
	u, _ := svc.Store.UpsertUserByOIDC(ctx, "u-sub", "u@example.com", "U")
	c := loggedInClient(t, ts, signer, u.ID)

	resp, err := c.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	// MCP URL derived from the OIDC redirect URL host.
	if !strings.Contains(string(body), "https://example.test/mcp") {
		t.Errorf("expected MCP URL in body, got: %s", body)
	}
}

func TestNonAdminCannotAccessGroup(t *testing.T) {
	ts, svc, signer := newTestWebServer(t)
	ctx := context.Background()
	_, _ = svc.CreateGroupWithSlug(ctx, "g", "G")
	stranger, _ := svc.Store.UpsertUserByOIDC(ctx, "stranger-sub", "stranger@example.com", "Stranger")

	c := loggedInClient(t, ts, signer, stranger.ID)
	resp, err := c.Get(ts.URL + "/g/g")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

func TestDashboardRendersForAdmin(t *testing.T) {
	ts, svc, signer := newTestWebServer(t)
	ctx := context.Background()
	g, _ := svc.CreateGroupWithSlug(ctx, "g", "G")
	u, _ := svc.Store.UpsertUserByOIDC(ctx, "u-sub", "u@example.com", "U")
	_ = svc.Store.AddGroupAdmin(ctx, u.ID, g.ID)
	_, _ = svc.Store.CreatePlayer(ctx, g.ID, "Pia", 800)

	c := loggedInClient(t, ts, signer, u.ID)
	resp, err := c.Get(ts.URL + "/g/g")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("dashboard %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "Pia") {
		t.Errorf("Pia not in body")
	}
}

func TestWebHasNoWriteEndpoints(t *testing.T) {
	// Sanity check: the web UI is read-only, all the old POST routes
	// have been removed. The mux should respond 405 (method not allowed)
	// or 404 to any POST except /logout and /auth/*.
	ts, _, _ := newTestWebServer(t)
	// Routes that should NOT accept POST (old session/PDF / admin/CRUD).
	for _, path := range []string{"/groups", "/g/g/players", "/g/g/sessions", "/g/g/admins/add"} {
		resp, err := http.Post(ts.URL+path, "application/x-www-form-urlencoded", strings.NewReader(""))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode < 400 {
			t.Errorf("POST %s should not be accepted, got %d", path, resp.StatusCode)
		}
	}
}
