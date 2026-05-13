package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levino/go-ranking/internal/auth"
	"github.com/levino/go-ranking/internal/service"
	"github.com/levino/go-ranking/internal/store"
)

// fakeOIDC stands up an httptest.Server that speaks just enough OIDC for
// the MCP server's authentication path: discovery + userinfo. Bearer
// tokens map 1:1 to subject claims via the supplied table.
func fakeOIDC(t *testing.T, tokens map[string]auth.UserInfo) *auth.OIDC {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 srv.URL,
			"authorization_endpoint": srv.URL + "/authorize",
			"token_endpoint":         srv.URL + "/token",
			"userinfo_endpoint":      srv.URL + "/userinfo",
			"jwks_uri":               srv.URL + "/keys",
		})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		info, ok := tokens[tok]
		if !ok {
			http.Error(w, `{"error":"invalid_token"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(info)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return auth.NewOIDC(srv.URL, "client", "secret", "http://localhost/cb")
}

func newTestServer(t *testing.T, tokens map[string]auth.UserInfo) (*Server, *store.User) {
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
	oidc := fakeOIDC(t, tokens)
	return &Server{
		Service:  service.New(st),
		OIDC:     oidc,
		Resource: "https://example.test/mcp",
	}, u
}

// withAuth posts an MCP request with the given bearer token (if any).
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
	tokens := map[string]auth.UserInfo{
		"tok-1": {Subject: "test-sub", Email: "test@example.com", Name: "Test User"},
	}
	s, _ := newTestServer(t, tokens)
	w := withAuth(t, s.Handler(),
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`, "tok-1")
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("err %+v", resp.Error)
	}
	r, _ := resp.Result.(map[string]any)
	if got := r["serverInfo"].(map[string]any)["name"]; got != "go-liga" {
		t.Errorf("server name = %v", got)
	}
}

func TestUnauthorizedHasWWWAuthenticate(t *testing.T) {
	// No token → 401 with WWW-Authenticate pointing at our metadata URL.
	s, _ := newTestServer(t, nil)
	w := withAuth(t, s.Handler(), `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", w.Code)
	}
	wa := w.Header().Get("WWW-Authenticate")
	if !strings.Contains(wa, "Bearer") || !strings.Contains(wa, "resource_metadata=") {
		t.Errorf("WWW-Authenticate missing or malformed: %q", wa)
	}
}

func TestProtectedResourceMetadata(t *testing.T) {
	s, _ := newTestServer(t, nil)
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
	if as, _ := meta["authorization_servers"].([]any); len(as) != 1 {
		t.Errorf("authorization_servers = %v", as)
	}
}

func TestCreateGroupViaMCPMakesCallerAdmin(t *testing.T) {
	tokens := map[string]auth.UserInfo{
		"tok-1": {Subject: "test-sub", Email: "test@example.com", Name: "Test User"},
	}
	s, _ := newTestServer(t, tokens)
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_group","arguments":{"slug":"test-group","name":"Test"}}}`
	w := withAuth(t, s.Handler(), body, "tok-1")
	if w.Code != 200 {
		t.Fatalf("status %d body %s", w.Code, w.Body)
	}
	g, err := s.Service.Store.GroupBySlug(context.Background(), "test-group")
	if err != nil {
		t.Fatal(err)
	}
	user, _ := s.Service.Store.UserByOIDC(context.Background(), "test-sub")
	if ok, _ := s.Service.Store.IsGroupAdmin(context.Background(), user.ID, g.ID); !ok {
		t.Fatal("caller should be admin")
	}
}

func TestResolveAdminGroupRejectsNonAdmin(t *testing.T) {
	tokens := map[string]auth.UserInfo{
		"tok-1": {Subject: "stranger-sub", Email: "stranger@example.com", Name: "Stranger"},
	}
	s, _ := newTestServer(t, tokens)
	_, _ = s.Service.CreateGroupWithSlug(context.Background(), "owned", "Owned")

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_players","arguments":{"group":"owned"}}}`
	w := withAuth(t, s.Handler(), body, "tok-1")
	if w.Code != 200 {
		t.Fatalf("expected 200 RPC envelope, got %d", w.Code)
	}
	var resp Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	out, _ := resp.Result.(map[string]any)
	if isErr, _ := out["isError"].(bool); !isErr {
		t.Fatalf("expected isError=true for non-admin caller, got %+v", out)
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
	tokens := map[string]auth.UserInfo{
		"tok-1": {Subject: "test-sub", Email: "test@example.com", Name: "Test User"},
	}
	s, _ := newTestServer(t, tokens)
	body := `[
        {"jsonrpc":"2.0","id":1,"method":"tools/list"},
        {"jsonrpc":"2.0","id":2,"method":"ping"}
    ]`
	w := withAuth(t, s.Handler(), body, "tok-1")
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
