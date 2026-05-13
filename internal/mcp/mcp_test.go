package mcp

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/levino/go-ranking/internal/auth"
	"github.com/levino/go-ranking/internal/service"
	"github.com/levino/go-ranking/internal/store"
)

// newTestServer builds an MCP server backed by an in-memory store and
// a deterministic signing key. Tests mint their own access tokens via
// signer.SignAccess; the OAuth AS endpoints have their own coverage.
func newTestServer(t *testing.T) (*Server, *store.User, *auth.Signer) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	u, err := st.UpsertUserByOIDC(context.Background(), "test-sub", "test@example.com", "Test User")
	if err != nil {
		t.Fatal(err)
	}
	keyBytes, _ := hex.DecodeString(strings.Repeat("c", 64))
	signer := auth.NewSigner(keyBytes)
	return &Server{
		Service:  service.New(st),
		Signer:   signer,
		Resource: "https://example.test/mcp",
	}, u, signer
}

// mintToken issues a valid access token for the given user. Mirrors
// what HandleToken would produce after a successful OAuth flow.
func mintToken(t *testing.T, s *Server, user *store.User) string {
	t.Helper()
	tok, err := s.Signer.SignAccess(auth.AccessClaims{
		Issuer:    s.publicBase(),
		Subject:   fmt.Sprintf("%d", user.ID),
		Audience:  s.Resource,
		ClientID:  "test-client",
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func withAuth(t *testing.T, h http.Handler, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestInitializeNegotiation(t *testing.T) {
	s, u, _ := newTestServer(t)
	tok := mintToken(t, s, u)
	w := withAuth(t, s.Handler(),
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`, tok)
	if w.Code != 200 {
		t.Fatalf("status %d body %s", w.Code, w.Body)
	}
	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("err %+v", resp.Error)
	}
}

func TestUnauthorizedHasWWWAuthenticate(t *testing.T) {
	s, _, _ := newTestServer(t)
	w := withAuth(t, s.Handler(), `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", w.Code)
	}
	wa := w.Header().Get("WWW-Authenticate")
	if !strings.Contains(wa, "Bearer") || !strings.Contains(wa, "resource_metadata=") {
		t.Errorf("WWW-Authenticate missing or malformed: %q", wa)
	}
}

func TestRejectsBadJWT(t *testing.T) {
	s, _, _ := newTestServer(t)
	w := withAuth(t, s.Handler(), `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, "not.a.jwt")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", w.Code)
	}
}

func TestRejectsExpiredJWT(t *testing.T) {
	s, u, signer := newTestServer(t)
	expired, _ := signer.SignAccess(auth.AccessClaims{
		Issuer: s.publicBase(), Subject: fmt.Sprintf("%d", u.ID),
		Audience: s.Resource, IssuedAt: time.Now().Add(-2 * time.Hour).Unix(),
		ExpiresAt: time.Now().Add(-time.Hour).Unix(),
	})
	w := withAuth(t, s.Handler(), `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, expired)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for expired token, got %d", w.Code)
	}
}

func TestProtectedResourceMetadata(t *testing.T) {
	s, _, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/.well-known/oauth-protected-resource", nil)
	w := httptest.NewRecorder()
	s.HandleProtectedResource(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	var meta map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["resource"] != "https://example.test/mcp" {
		t.Errorf("resource = %v", meta["resource"])
	}
	as, _ := meta["authorization_servers"].([]any)
	if len(as) != 1 || as[0] != "https://example.test" {
		t.Errorf("authorization_servers = %v", as)
	}
}

func TestAuthServerMetadataPointsAtSelf(t *testing.T) {
	s, _, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/.well-known/oauth-authorization-server", nil)
	w := httptest.NewRecorder()
	s.HandleAuthServerMetadata(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	var meta map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &meta)
	if meta["registration_endpoint"] != "https://example.test/oauth/register" {
		t.Errorf("registration_endpoint = %v", meta["registration_endpoint"])
	}
}

func TestDynamicClientRegistration(t *testing.T) {
	s, _, _ := newTestServer(t)
	body := `{"client_name":"Claude","redirect_uris":["https://claude.ai/cb"]}`
	req := httptest.NewRequest("POST", "/oauth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.HandleRegister(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", w.Code, w.Body)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if cid, _ := resp["client_id"].(string); cid == "" {
		t.Error("missing client_id")
	}
	if tem, _ := resp["token_endpoint_auth_method"].(string); tem != "none" {
		t.Errorf("token_endpoint_auth_method = %v", tem)
	}
}

func TestCreateGroupViaMCPMakesCallerAdmin(t *testing.T) {
	s, u, _ := newTestServer(t)
	tok := mintToken(t, s, u)
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_group","arguments":{"slug":"test-group","name":"Test"}}}`
	w := withAuth(t, s.Handler(), body, tok)
	if w.Code != 200 {
		t.Fatalf("status %d body %s", w.Code, w.Body)
	}
	g, err := s.Service.Store.GroupBySlug(context.Background(), "test-group")
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.Service.Store.IsGroupAdmin(context.Background(), u.ID, g.ID); !ok {
		t.Fatal("caller should be admin")
	}
}

func TestResolveAdminGroupRejectsNonAdmin(t *testing.T) {
	s, u, _ := newTestServer(t)
	tok := mintToken(t, s, u)
	_, _ = s.Service.CreateGroupWithSlug(context.Background(), "owned", "Owned")
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_players","arguments":{"group":"owned"}}}`
	w := withAuth(t, s.Handler(), body, tok)
	var resp Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	out, _ := resp.Result.(map[string]any)
	if isErr, _ := out["isError"].(bool); !isErr {
		t.Fatalf("expected isError=true, got %+v", out)
	}
}

func TestNumArg(t *testing.T) {
	cases := []struct {
		in any
		ok bool
		v  int
	}{
		{1.0, true, 1},
		{int64(2), true, 2},
		{"5", true, 5},
		{"oops", false, 0},
		{nil, false, 0},
	}
	for _, c := range cases {
		v, ok := numArg(c.in)
		if ok != c.ok || (ok && v != c.v) {
			t.Errorf("numArg(%v) = %d, %v; want %d, %v", c.in, v, ok, c.v, c.ok)
		}
	}
}

func TestBatchRequestReturnsArray(t *testing.T) {
	s, u, _ := newTestServer(t)
	tok := mintToken(t, s, u)
	body := `[
        {"jsonrpc":"2.0","id":1,"method":"tools/list"},
        {"jsonrpc":"2.0","id":2,"method":"ping"}
    ]`
	w := withAuth(t, s.Handler(), body, tok)
	if !bytes.HasPrefix(bytes.TrimSpace(w.Body.Bytes()), []byte("[")) {
		t.Fatalf("expected JSON array, got %q", w.Body.String())
	}
	var arr []Response
	if err := json.Unmarshal(w.Body.Bytes(), &arr); err != nil {
		t.Fatal(err)
	}
	if len(arr) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(arr))
	}
}
