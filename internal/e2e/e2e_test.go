// Package e2e exercises the web UI (read-only) and the MCP API (the
// only writer) end-to-end against a real httptest.Server backed by a
// temporary SQLite database.
//
// The MCP-side tests stand up a fake OIDC server speaking discovery +
// userinfo, so the real Userinfo-based auth path runs end to end.
package e2e

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
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
	"github.com/levino/go-ranking/internal/mcp"
	"github.com/levino/go-ranking/internal/service"
	"github.com/levino/go-ranking/internal/store"
	"github.com/levino/go-ranking/internal/web"
)

// ---- Test rig ------------------------------------------------------------

type rig struct {
	t       *testing.T
	srv     *httptest.Server
	svc     *service.Service
	mcp     *mcp.Server
	signer  *auth.Signer
	tokens  map[string]auth.UserInfo // bearer token → userinfo
	mcpURL  string
}

// stubOIDC stands up a fake OIDC server (discovery + userinfo) and
// returns an OIDC client pointing at it. Tokens map 1:1 to userinfo.
func stubOIDC(t *testing.T, tokens map[string]auth.UserInfo) *auth.OIDC {
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
	return auth.NewOIDC(srv.URL, "client", "secret", "https://example.test/auth/callback")
}

func newRig(t *testing.T) *rig {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "e2e.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := service.New(st)
	keyHex := strings.Repeat("a", 64)
	keyBytes, _ := hex.DecodeString(keyHex)
	signer := auth.NewSigner(keyBytes)
	tokens := map[string]auth.UserInfo{}
	oidc := stubOIDC(t, tokens)
	webSrv, err := web.New(svc, signer, oidc)
	if err != nil {
		t.Fatal(err)
	}
	mcpSrv := &mcp.Server{
		Service:  svc,
		OIDC:     oidc,
		Resource: "https://example.test/mcp",
	}

	root := http.NewServeMux()
	root.Handle("/mcp", mcpSrv.Handler())
	root.HandleFunc("GET /.well-known/oauth-protected-resource", mcpSrv.HandleProtectedResource)
	root.Handle("/", webSrv.Handler())
	srv := httptest.NewServer(root)
	t.Cleanup(srv.Close)

	return &rig{t: t, srv: srv, svc: svc, mcp: mcpSrv, signer: signer, tokens: tokens, mcpURL: srv.URL + "/mcp"}
}

// loginAs creates/refreshes a user and returns a web client with a
// session cookie plus a fake OIDC bearer token that maps to that user.
func (r *rig) loginAs(subject, email, name string) (*http.Client, *store.User, string) {
	r.t.Helper()
	u, err := r.svc.Store.UpsertUserByOIDC(context.Background(), subject, email, name)
	if err != nil {
		r.t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	c := &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	rec := httptest.NewRecorder()
	if err := r.signer.Issue(rec, u.ID, 24*time.Hour); err != nil {
		r.t.Fatal(err)
	}
	srvURL, _ := url.Parse(r.srv.URL)
	jar.SetCookies(srvURL, rec.Result().Cookies())

	tok := "tok-" + subject
	r.tokens[tok] = auth.UserInfo{Subject: subject, Email: email, Name: name}
	return c, u, tok
}

// ---- MCP-driven CRUD: full lifecycle -------------------------------------

func TestEndToEndMCPLifecycle(t *testing.T) {
	rg := newRig(t)
	_, owner, ownerTok := rg.loginAs("oidc-owner", "owner@example.com", "Owner")

	// 1. create_group → owner is admin
	mustToolOK(t, rg, ownerTok, "create_group", map[string]any{"slug": "g1", "name": "Group One"})
	g, err := rg.svc.Store.GroupBySlug(context.Background(), "g1")
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := rg.svc.Store.IsGroupAdmin(context.Background(), owner.ID, g.ID); !ok {
		t.Fatal("creator should be admin")
	}

	// 2. add_player x4
	for _, p := range []struct{ name, rank string }{
		{"Anna", "10k"}, {"Ben", "15k"}, {"Clara", "20k"}, {"Dirk", "25k"},
	} {
		mustToolOK(t, rg, ownerTok, "add_player", map[string]any{"group": "g1", "name": p.name, "rank": p.rank})
	}
	players, _ := rg.svc.Store.ListPlayers(context.Background(), g.ID, true)
	if len(players) != 4 {
		t.Fatalf("expected 4 players, got %d", len(players))
	}

	// 3. update_player: deactivate Dirk
	mustToolOK(t, rg, ownerTok, "update_player", map[string]any{
		"group": "g1", "name": "Dirk", "active": false,
	})
	active, _ := rg.svc.Store.ListPlayers(context.Background(), g.ID, false)
	if len(active) != 3 {
		t.Fatalf("expected 3 active players, got %d", len(active))
	}

	// 4. create_session
	out := mustToolOK(t, rg, ownerTok, "create_session", map[string]any{"group": "g1"})
	pass := extractPassphrase(t, out)

	// 5. record_game: Ben (#2, 15k) Black vs Anna (#1, 10k) White; Ben wins.
	mustToolOK(t, rg, ownerTok, "record_game", map[string]any{
		"passphrase":   pass,
		"black_number": 2,
		"white_number": 1,
		"board_size":   "13",
		"winner":       "black",
	})

	// 6. list_sessions sees one session with one game.
	out = mustToolOK(t, rg, ownerTok, "list_sessions", map[string]any{"group": "g1"})
	if !strings.Contains(out, pass) || !strings.Contains(out, "1 Partien") {
		t.Errorf("list_sessions output not as expected: %s", out)
	}

	// 7. Co-admin: someone else logs in (so they exist in users) → owner adds them.
	_, _, _ = rg.loginAs("oidc-co", "co@example.com", "Co")
	mustToolOK(t, rg, ownerTok, "add_admin", map[string]any{"group": "g1", "email": "co@example.com"})
	admins, _ := rg.svc.Store.ListGroupAdmins(context.Background(), g.ID)
	if len(admins) != 2 {
		t.Fatalf("expected 2 admins, got %d", len(admins))
	}

	// 8. remove_admin (the co-admin, not self).
	mustToolOK(t, rg, ownerTok, "remove_admin", map[string]any{"group": "g1", "email": "co@example.com"})
	admins, _ = rg.svc.Store.ListGroupAdmins(context.Background(), g.ID)
	if len(admins) != 1 {
		t.Fatalf("expected 1 admin after remove, got %d", len(admins))
	}

	// 9. Removing oneself when only admin → error (sole-admin guard).
	out = callTool(t, rg, ownerTok, "remove_admin", map[string]any{"group": "g1", "email": "owner@example.com"})
	if !strings.Contains(out, "last admin") {
		t.Errorf("sole-admin guard not triggered: %s", out)
	}
}

// ---- Web UI is read-only -------------------------------------------------

func TestWebUIReadOnly(t *testing.T) {
	rg := newRig(t)
	owner, _, ownerTok := rg.loginAs("oidc-owner", "owner@example.com", "Owner")

	// Seed data via MCP.
	mustToolOK(t, rg, ownerTok, "create_group", map[string]any{"slug": "g1", "name": "Group One"})
	mustToolOK(t, rg, ownerTok, "add_player", map[string]any{"group": "g1", "name": "Anna", "rank": "10k"})
	mustToolOK(t, rg, ownerTok, "create_session", map[string]any{"group": "g1"})

	// Dashboard renders.
	resp, err := owner.Get(rg.srv.URL + "/g/g1")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !bytes.Contains(body, []byte("Anna")) {
		t.Fatalf("dashboard %d, body %s", resp.StatusCode, body)
	}

	// PDFs render.
	g, _ := rg.svc.Store.GroupBySlug(context.Background(), "g1")
	sess, _ := rg.svc.Store.ListSessions(context.Background(), g.ID)
	pass := sess[0].Passphrase
	for _, suffix := range []string{"matrix.pdf", "scorecard.pdf"} {
		u := rg.srv.URL + "/g/g1/sessions/" + pass + "/" + suffix
		resp, err := owner.Get(u)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("%s status %d", suffix, resp.StatusCode)
		}
		if !bytes.HasPrefix(b, []byte("%PDF-")) {
			t.Fatalf("%s not a PDF", suffix)
		}
	}

	// A stranger gets 403.
	stranger, _, _ := rg.loginAs("oidc-stranger", "stranger@example.com", "Stranger")
	resp, err = stranger.Get(rg.srv.URL + "/g/g1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("stranger expected 403, got %d", resp.StatusCode)
	}
}

// ---- MCP auth: discovery + 401 contract ----------------------------------

func TestMCPUnauthenticatedReturnsResourceMetadata(t *testing.T) {
	rg := newRig(t)
	req, _ := http.NewRequest("POST", rg.mcpURL,
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	wa := resp.Header.Get("WWW-Authenticate")
	if !strings.Contains(wa, "resource_metadata=") {
		t.Errorf("WWW-Authenticate missing resource_metadata: %q", wa)
	}
}

func TestProtectedResourceEndpoint(t *testing.T) {
	rg := newRig(t)
	resp, err := http.Get(rg.srv.URL + "/.well-known/oauth-protected-resource")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	var meta map[string]any
	if err := json.Unmarshal(body, &meta); err != nil {
		t.Fatal(err)
	}
	if meta["resource"] == nil || meta["authorization_servers"] == nil {
		t.Errorf("metadata incomplete: %s", body)
	}
}

// ---- Helpers -------------------------------------------------------------

type rpcRes struct {
	Result any `json:"result,omitempty"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func mcpRPC(t *testing.T, url, token, method string, params map[string]any) *rpcRes {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	})
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("rpc %s: status %d body %s", method, resp.StatusCode, b)
	}
	var r rpcRes
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatalf("rpc %s: bad json %s", method, b)
	}
	return &r
}

func callTool(t *testing.T, rg *rig, token, name string, args map[string]any) string {
	res := mcpRPC(t, rg.mcpURL, token, "tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})
	if res.Error != nil {
		t.Fatalf("transport error calling %s: %+v", name, res.Error)
	}
	out, _ := res.Result.(map[string]any)
	content, _ := out["content"].([]any)
	var sb strings.Builder
	for _, c := range content {
		if m, ok := c.(map[string]any); ok {
			if s, ok := m["text"].(string); ok {
				sb.WriteString(s)
			}
		}
	}
	return sb.String()
}

func mustToolOK(t *testing.T, rg *rig, token, name string, args map[string]any) string {
	t.Helper()
	res := mcpRPC(t, rg.mcpURL, token, "tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})
	if res.Error != nil {
		t.Fatalf("transport error calling %s: %+v", name, res.Error)
	}
	out, _ := res.Result.(map[string]any)
	if isErr, _ := out["isError"].(bool); isErr {
		body, _ := json.Marshal(out)
		t.Fatalf("%s returned error: %s", name, body)
	}
	content, _ := out["content"].([]any)
	var sb strings.Builder
	for _, c := range content {
		if m, ok := c.(map[string]any); ok {
			if s, ok := m["text"].(string); ok {
				sb.WriteString(s)
			}
		}
	}
	return sb.String()
}

func extractPassphrase(t *testing.T, createSessionOutput string) string {
	t.Helper()
	line := strings.SplitN(createSessionOutput, "\n", 2)[0]
	pass := strings.TrimSpace(strings.TrimPrefix(line, "Neue Session: "))
	if pass == "" {
		t.Fatalf("could not parse passphrase from %q", createSessionOutput)
	}
	return pass
}
