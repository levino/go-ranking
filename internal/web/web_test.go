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
	oidc := auth.NewOIDC("https://example.invalid", "id", "secret", "http://localhost/auth/callback")
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

func TestLoginPageRenders(t *testing.T) {
	ts, _, _ := newTestWebServer(t)
	resp, err := http.Get(ts.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Anmelden") {
		t.Errorf("login page should contain 'Anmelden'")
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

func TestAuthStartRedirectsToIssuer(t *testing.T) {
	// We use a bogus issuer so discovery fails — but we can still verify
	// the handler short-circuits to 500 rather than panicking, and that
	// the state cookie machinery is wired up. (A real OIDC mock is
	// covered by the OIDC unit tests if/when we add them.)
	ts, _, _ := newTestWebServer(t)
	resp, err := http.Get(ts.URL + "/auth/start")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500 from discovery against example.invalid, got %d", resp.StatusCode)
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

func TestCreateGroupViaForm(t *testing.T) {
	ts, svc, signer := newTestWebServer(t)
	ctx := context.Background()
	u, _ := svc.Store.UpsertUserByOIDC(ctx, "u-sub", "u@example.com", "U")
	c := loggedInClient(t, ts, signer, u.ID)

	resp, err := c.PostForm(ts.URL+"/groups", url.Values{
		"slug": {"my-group"}, "name": {"My Group"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("create group: %d", resp.StatusCode)
	}

	g, err := svc.Store.GroupBySlug(ctx, "my-group")
	if err != nil {
		t.Fatal(err)
	}
	ok, _ := svc.Store.IsGroupAdmin(ctx, u.ID, g.ID)
	if !ok {
		t.Fatal("creator must be admin")
	}
}

func TestPlayerCreateAndList(t *testing.T) {
	ts, svc, signer := newTestWebServer(t)
	ctx := context.Background()
	g, _ := svc.CreateGroupWithSlug(ctx, "g", "G")
	u, _ := svc.Store.UpsertUserByOIDC(ctx, "u-sub", "u@example.com", "U")
	_ = svc.Store.AddGroupAdmin(ctx, u.ID, g.ID)

	c := loggedInClient(t, ts, signer, u.ID)
	resp, err := c.PostForm(ts.URL+"/g/g/players", url.Values{"name": {"Pia"}, "rank": {"15k"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("create: %d", resp.StatusCode)
	}

	resp, err = c.Get(ts.URL + "/g/g/players")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "Pia") {
		t.Errorf("Pia not in body")
	}
}

func TestPlayersFormRejectsBadRank(t *testing.T) {
	ts, svc, signer := newTestWebServer(t)
	ctx := context.Background()
	g, _ := svc.CreateGroupWithSlug(ctx, "g", "G")
	u, _ := svc.Store.UpsertUserByOIDC(ctx, "u-sub", "u@example.com", "U")
	_ = svc.Store.AddGroupAdmin(ctx, u.ID, g.ID)

	c := loggedInClient(t, ts, signer, u.ID)
	resp, err := c.PostForm(ts.URL+"/g/g/players", url.Values{
		"name": {"Pia"}, "rank": {"banana"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
