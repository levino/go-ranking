// Package e2e exercises the web UI (read-only) and the MCP API (the
// only writer) end-to-end against a real httptest.Server backed by a
// temporary SQLite database.
//
// We bypass the actual OAuth flows on both sides — test users get a
// minted session cookie (web) and a minted access-token JWT (MCP),
// using the same signer the production code uses.
package e2e

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	t      *testing.T
	srv    *httptest.Server
	svc    *service.Service
	mcp    *mcp.Server
	signer *auth.Signer
	mcpURL string
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
	oidc := auth.NewOIDC("https://example.invalid", "id", "secret", "https://example.test/auth/callback")
	webSrv, err := web.New(svc, signer, oidc)
	if err != nil {
		t.Fatal(err)
	}
	// Use the eventual httptest URL as the MCP resource so the JWT
	// audience matches when we hit /mcp.
	srv := httptest.NewServer(nil)
	t.Cleanup(srv.Close)
	mcpSrv := &mcp.Server{
		Service:  svc,
		Signer:   signer,
		OIDC:     oidc,
		Resource: srv.URL + "/mcp",
	}
	root := http.NewServeMux()
	root.Handle("/mcp", mcp.CORS(mcpSrv.Handler()))
	root.Handle("/.well-known/oauth-protected-resource",
		mcp.CORS(http.HandlerFunc(mcpSrv.HandleProtectedResource)))
	root.Handle("/.well-known/oauth-authorization-server",
		mcp.CORS(http.HandlerFunc(mcpSrv.HandleAuthServerMetadata)))
	root.Handle("/oauth/register", mcp.CORS(http.HandlerFunc(mcpSrv.HandleRegister)))
	root.Handle("/oauth/authorize", mcp.CORS(http.HandlerFunc(mcpSrv.HandleAuthorize)))
	root.Handle("/oauth/token", mcp.CORS(http.HandlerFunc(mcpSrv.HandleToken)))
	root.Handle("/", webSrv.Handler())
	srv.Config.Handler = root

	return &rig{t: t, srv: srv, svc: svc, mcp: mcpSrv, signer: signer, mcpURL: srv.URL + "/mcp"}
}

// loginAs creates/refreshes a user and returns a web client with a
// session cookie + a minted MCP access token for that user.
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

	// Mint an access token for /mcp with the right issuer + audience.
	base := strings.TrimSuffix(r.mcpURL, "/mcp")
	tok, err := r.signer.SignAccess(auth.AccessClaims{
		Issuer:    base,
		Subject:   fmt.Sprintf("%d", u.ID),
		Audience:  r.mcpURL,
		ClientID:  "test-client",
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		r.t.Fatal(err)
	}
	return c, u, tok
}

// ---- MCP-driven CRUD: lifecycle without sessions ------------------------

func TestEndToEndMCPLifecycle(t *testing.T) {
	rg := newRig(t)
	_, owner, ownerTok := rg.loginAs("oidc-owner", "owner@example.com", "Owner")

	mustToolOK(t, rg, ownerTok, "create_group", map[string]any{"slug": "g1", "name": "Group One"})
	g, _ := rg.svc.Store.GroupBySlug(context.Background(), "g1")
	if ok, _ := rg.svc.Store.IsGroupAdmin(context.Background(), owner.ID, g.ID); !ok {
		t.Fatal("creator should be admin")
	}

	for _, p := range []struct{ name, rank string }{
		{"Anna", "10k"}, {"Ben", "15k"}, {"Clara", "20k"}, {"Dirk", "25k"},
	} {
		mustToolOK(t, rg, ownerTok, "add_player", map[string]any{"group": "g1", "name": p.name, "rank": p.rank})
	}
	players, _ := rg.svc.Store.ListPlayers(context.Background(), g.ID, true)
	if len(players) != 4 {
		t.Fatalf("expected 4 players, got %d", len(players))
	}

	mustToolOK(t, rg, ownerTok, "update_player", map[string]any{
		"group": "g1", "name": "Dirk", "active": false,
	})
	active, _ := rg.svc.Store.ListPlayers(context.Background(), g.ID, false)
	if len(active) != 3 {
		t.Fatalf("expected 3 active players, got %d", len(active))
	}

	// Recommend handicap → ensures Ben (15k) plays Black against Anna (10k).
	out := mustToolOK(t, rg, ownerTok, "recommend_handicap", map[string]any{
		"group": "g1", "player_a": "Anna", "player_b": "Ben", "board": "13",
	})
	if !strings.Contains(out, "Ben spielt Schwarz") {
		t.Errorf("recommend should put Ben as Black: %s", out)
	}

	// Record a game with a manual handicap of 4.
	out = mustToolOK(t, rg, ownerTok, "record_game", map[string]any{
		"group": "g1", "black": "Ben", "white": "Anna",
		"board": "13", "handicap": 4, "winner": "black",
	})
	if !strings.Contains(out, "Eingetragen") {
		t.Errorf("record_game output unexpected: %s", out)
	}
	games, _ := rg.svc.Store.ListRecentGames(context.Background(), g.ID, 10)
	if len(games) != 1 {
		t.Fatalf("expected 1 game stored, got %d", len(games))
	}

	_, _, _ = rg.loginAs("oidc-co", "co@example.com", "Co")
	mustToolOK(t, rg, ownerTok, "add_admin", map[string]any{"group": "g1", "email": "co@example.com"})
	admins, _ := rg.svc.Store.ListGroupAdmins(context.Background(), g.ID)
	if len(admins) != 2 {
		t.Fatalf("expected 2 admins, got %d", len(admins))
	}

	mustToolOK(t, rg, ownerTok, "remove_admin", map[string]any{"group": "g1", "email": "co@example.com"})
	admins, _ = rg.svc.Store.ListGroupAdmins(context.Background(), g.ID)
	if len(admins) != 1 {
		t.Fatalf("expected 1 admin after remove, got %d", len(admins))
	}

	out = callTool(t, rg, ownerTok, "remove_admin", map[string]any{"group": "g1", "email": "owner@example.com"})
	if !strings.Contains(out, "last admin") {
		t.Errorf("sole-admin guard not triggered: %s", out)
	}
}

// ---- Tablet UI: full flow on a single page ------------------------------

func TestTabletPlayFlow(t *testing.T) {
	rg := newRig(t)
	owner, _, ownerTok := rg.loginAs("oidc-owner", "owner@example.com", "Owner")
	mustToolOK(t, rg, ownerTok, "create_group", map[string]any{"slug": "g1", "name": "Group One"})
	mustToolOK(t, rg, ownerTok, "add_player", map[string]any{"group": "g1", "name": "Anna", "rank": "10k"})
	mustToolOK(t, rg, ownerTok, "add_player", map[string]any{"group": "g1", "name": "Ben", "rank": "15k"})

	g, _ := rg.svc.Store.GroupBySlug(context.Background(), "g1")
	players, _ := rg.svc.Store.ListPlayers(context.Background(), g.ID, true)
	var anna, ben *store.Player
	for i := range players {
		switch players[i].Name {
		case "Anna":
			anna = &players[i]
		case "Ben":
			ben = &players[i]
		}
	}

	// Pairing calculator
	u := fmt.Sprintf("%s/g/g1/play?action=recommend&p1=%d&p2=%d&board=13", rg.srv.URL, anna.ID, ben.ID)
	resp, _ := owner.Get(u)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Contains(body, []byte("Ben")) || !bytes.Contains(body, []byte("Schwarz")) {
		t.Errorf("recommendation not rendered: %s", body)
	}

	// Add new player inline
	resp, _ = owner.PostForm(rg.srv.URL+"/g/g1/play/players", url.Values{
		"name": {"Spontaneous Kid"}, "rank": {"30k"},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("add player: %d", resp.StatusCode)
	}
	all, _ := rg.svc.Store.ListPlayers(context.Background(), g.ID, true)
	if len(all) != 3 {
		t.Fatalf("expected 3 players after add, got %d", len(all))
	}

	// Record a game
	resp, _ = owner.PostForm(rg.srv.URL+"/g/g1/play/record", url.Values{
		"black": {fmt.Sprintf("%d", ben.ID)}, "white": {fmt.Sprintf("%d", anna.ID)},
		"board": {"13"}, "handicap": {"4"}, "winner": {"black"},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("record_game form: %d", resp.StatusCode)
	}
	games, _ := rg.svc.Store.ListRecentGames(context.Background(), g.ID, 10)
	if len(games) != 1 {
		t.Fatalf("expected 1 game, got %d", len(games))
	}

	stranger, _, _ := rg.loginAs("oidc-stranger", "stranger@example.com", "Stranger")
	resp, _ = stranger.Get(rg.srv.URL + "/g/g1/play")
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("stranger expected 403, got %d", resp.StatusCode)
	}
}

// ---- OAuth contract ------------------------------------------------------

func TestUnauthenticatedMCPReturnsResourceMetadata(t *testing.T) {
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

func TestOAuthAuthorizationServerMetadata(t *testing.T) {
	rg := newRig(t)
	resp, err := http.Get(rg.srv.URL + "/.well-known/oauth-authorization-server")
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
	for _, key := range []string{"issuer", "authorization_endpoint", "token_endpoint", "registration_endpoint"} {
		if meta[key] == nil {
			t.Errorf("missing %q in AS metadata", key)
		}
	}
}

func TestCORSPreflight(t *testing.T) {
	rg := newRig(t)
	req, _ := http.NewRequest("OPTIONS", rg.mcpURL, nil)
	req.Header.Set("Origin", "https://claude.ai")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Authorization, Content-Type")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight expected 204, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "https://claude.ai" {
		t.Error("missing or wrong Access-Control-Allow-Origin")
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

