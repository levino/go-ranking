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

	"github.com/levino/go-ranking/internal/auth"
	"github.com/levino/go-ranking/internal/service"
	"github.com/levino/go-ranking/internal/store"
)

func newTestWebServer(t *testing.T) (*httptest.Server, *service.Service) {
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
	srv, err := New(svc, signer)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, svc
}

func TestLoginPageRenders(t *testing.T) {
	ts, _ := newTestWebServer(t)
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
	ts, _ := newTestWebServer(t)
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

func TestLoginRejectsBadCredentials(t *testing.T) {
	ts, svc := newTestWebServer(t)
	hash, _ := auth.HashPassword("right")
	_, _ = svc.Store.CreateUser(context.Background(), "alice", hash, nil, true)

	c := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := c.PostForm(ts.URL+"/login", url.Values{
		"username": {"alice"}, "password": {"wrong"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 (re-rendered login), got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Ungültige Anmeldedaten") {
		t.Error("expected error message in body")
	}
}

func TestNonAdminCannotAccessAdmin(t *testing.T) {
	ts, svc := newTestWebServer(t)
	g, _ := svc.CreateGroup(context.Background(), "G")
	hash, _ := auth.HashPassword("pw")
	_, _ = svc.Store.CreateUser(context.Background(), "t", hash, &g.ID, false)

	c := loggedIn(t, ts, "t", "pw")
	resp, err := c.Get(ts.URL + "/admin")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

func TestPlayerCreateAndList(t *testing.T) {
	ts, svc := newTestWebServer(t)
	g, _ := svc.CreateGroup(context.Background(), "G")
	hash, _ := auth.HashPassword("pw")
	_, _ = svc.Store.CreateUser(context.Background(), "t", hash, &g.ID, false)

	c := loggedIn(t, ts, "t", "pw")
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
	ts, svc := newTestWebServer(t)
	g, _ := svc.CreateGroup(context.Background(), "G")
	hash, _ := auth.HashPassword("pw")
	_, _ = svc.Store.CreateUser(context.Background(), "t", hash, &g.ID, false)

	c := loggedIn(t, ts, "t", "pw")
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

func loggedIn(t *testing.T, ts *httptest.Server, user, pass string) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := c.PostForm(ts.URL+"/login", url.Values{"username": {user}, "password": {pass}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("login %d", resp.StatusCode)
	}
	return c
}
