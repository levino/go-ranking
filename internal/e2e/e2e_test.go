// Package e2e exercises the web UI and the MCP API end-to-end against
// a real httptest.Server backed by a temporary SQLite database.
//
// The tests skip the real OIDC dance — they mint a session cookie
// directly via the Signer after upserting a user. This is the same
// state the server would be in after a successful OIDC callback.
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
	t      *testing.T
	srv    *httptest.Server
	svc    *service.Service
	mcp    *mcp.Server
	signer *auth.Signer
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
	oidc := auth.NewOIDC("https://example.invalid", "id", "secret", "http://localhost/auth/callback")
	webSrv, err := web.New(svc, signer, oidc)
	if err != nil {
		t.Fatal(err)
	}
	mcpSrv := &mcp.Server{Service: svc, AuthToken: "secret-token"}

	root := http.NewServeMux()
	root.Handle("/mcp", mcpSrv.Handler())
	root.Handle("/", webSrv.Handler())
	srv := httptest.NewServer(root)
	t.Cleanup(srv.Close)

	return &rig{t: t, srv: srv, svc: svc, mcp: mcpSrv, signer: signer}
}

// loginAs creates (or refreshes) a user via the OIDC upsert path and
// returns an http.Client with the session cookie already set.
func (r *rig) loginAs(subject, email, name string) (*http.Client, *store.User) {
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
	// Mint a cookie directly using the signer (skipping the OIDC flow).
	rec := httptest.NewRecorder()
	if err := r.signer.Issue(rec, u.ID, 24*time.Hour); err != nil {
		r.t.Fatal(err)
	}
	srvURL, _ := url.Parse(r.srv.URL)
	jar.SetCookies(srvURL, rec.Result().Cookies())
	return c, u
}

// ---- Web UI scenarios ----------------------------------------------------

func TestEndToEndOwnerFlow(t *testing.T) {
	rg := newRig(t)
	owner, ownerUser := rg.loginAs("oidc-owner", "owner@example.com", "Owner")

	// Owner creates a group via the public form (becomes admin).
	resp, err := owner.PostForm(rg.srv.URL+"/groups", url.Values{
		"slug": {"schule-linz"},
		"name": {"Schule Linz"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("create group: %d", resp.StatusCode)
	}
	g, err := rg.svc.Store.GroupBySlug(context.Background(), "schule-linz")
	if err != nil {
		t.Fatalf("group not created: %v", err)
	}
	ok, _ := rg.svc.Store.IsGroupAdmin(context.Background(), ownerUser.ID, g.ID)
	if !ok {
		t.Fatal("creator should be admin")
	}

	// Owner adds players.
	for _, p := range []struct{ name, rank string }{
		{"Anna", "10k"}, {"Ben", "15k"}, {"Clara", "20k"}, {"Dirk", "25k"},
	} {
		resp, err := owner.PostForm(rg.srv.URL+"/g/schule-linz/players", url.Values{
			"name": {p.name}, "rank": {p.rank},
		})
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	players, _ := rg.svc.Store.ListPlayers(context.Background(), g.ID, false)
	if len(players) != 4 {
		t.Fatalf("expected 4 players, got %d", len(players))
	}

	// Owner adds a co-admin (the new admin must already exist in DB).
	_, _ = rg.svc.Store.UpsertUserByOIDC(context.Background(), "oidc-co", "co@example.com", "Co Admin")
	resp, err = owner.PostForm(rg.srv.URL+"/g/schule-linz/admins/add", url.Values{
		"email": {"co@example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	admins, _ := rg.svc.Store.ListGroupAdmins(context.Background(), g.ID)
	if len(admins) != 2 {
		t.Fatalf("expected 2 admins, got %d", len(admins))
	}

	// Co-admin can now access the group.
	co, _ := rg.loginAs("oidc-co", "co@example.com", "Co Admin")
	resp, err = co.Get(rg.srv.URL + "/g/schule-linz")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("co-admin dashboard %d", resp.StatusCode)
	}
	if !bytes.Contains(body, []byte("Schule Linz")) {
		t.Errorf("dashboard missing group name")
	}

	// A logged-in stranger cannot.
	stranger, _ := rg.loginAs("oidc-stranger", "stranger@example.com", "Stranger")
	resp, err = stranger.Get(rg.srv.URL + "/g/schule-linz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("stranger expected 403, got %d", resp.StatusCode)
	}

	// Anonymous client gets redirected to /login.
	anon := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err = anon.Get(rg.srv.URL + "/g/schule-linz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("anon expected 302, got %d", resp.StatusCode)
	}
}

// ---- MCP scenarios -------------------------------------------------------

func TestEndToEndMCPRecordsGame(t *testing.T) {
	rg := newRig(t)
	ctx := context.Background()

	// Configure MCP to act as a specific user.
	_, u := rg.loginAs("oidc-mcp", "mcp@example.com", "MCP User")
	rg.mcp.MCPUser = u.OIDCSubject

	g, _ := rg.svc.CreateGroupWithSlug(ctx, "mcp", "MCP")
	_ = rg.svc.Store.AddGroupAdmin(ctx, u.ID, g.ID)
	a, _ := rg.svc.Store.CreatePlayer(ctx, g.ID, "Anna", 1000)
	b, _ := rg.svc.Store.CreatePlayer(ctx, g.ID, "Ben", 800)
	sess, err := rg.svc.CreateSession(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}

	res := mcpRPC(t, rg.srv.URL+"/mcp", "secret-token", "initialize", map[string]any{
		"protocolVersion": "2025-03-26",
		"clientInfo":      map[string]any{"name": "claude.ai", "version": "test"},
	})
	if res.Error != nil {
		t.Fatalf("init error: %+v", res.Error)
	}

	res = mcpRPC(t, rg.srv.URL+"/mcp", "secret-token", "tools/list", map[string]any{})
	if res.Error != nil {
		t.Fatalf("tools/list: %+v", res.Error)
	}
	resJSON, _ := json.Marshal(res.Result)
	for _, want := range []string{"record_game", "list_players", "ranking", "create_session",
		"get_session", "create_group", "list_my_groups", "add_admin"} {
		if !bytes.Contains(resJSON, []byte(want)) {
			t.Errorf("tools/list missing %q", want)
		}
	}

	res = mcpRPC(t, rg.srv.URL+"/mcp", "secret-token", "tools/call", map[string]any{
		"name": "record_game",
		"arguments": map[string]any{
			"passphrase":   sess.Passphrase,
			"black_number": 2,
			"white_number": 1,
			"board_size":   "13",
			"winner":       "black",
		},
	})
	if res.Error != nil {
		t.Fatalf("record_game error: %+v", res.Error)
	}
	out, _ := res.Result.(map[string]any)
	if isErr, _ := out["isError"].(bool); isErr {
		t.Fatalf("tool returned error result: %+v", out)
	}
	bAfter, _ := rg.svc.Store.PlayerByID(ctx, b.ID)
	aAfter, _ := rg.svc.Store.PlayerByID(ctx, a.ID)
	if bAfter.GoR <= 800 {
		t.Errorf("ben should have gained rating; %.1f", bAfter.GoR)
	}
	if aAfter.GoR >= 1000 {
		t.Errorf("anna should have lost rating; %.1f", aAfter.GoR)
	}

	res = mcpRPC(t, rg.srv.URL+"/mcp", "secret-token", "tools/call", map[string]any{
		"name":      "ranking",
		"arguments": map[string]any{"group": "mcp"},
	})
	if res.Error != nil {
		t.Fatalf("ranking error: %+v", res.Error)
	}
	rankJSON, _ := json.Marshal(res.Result)
	if !bytes.Contains(rankJSON, []byte("Anna")) || !bytes.Contains(rankJSON, []byte("Ben")) {
		t.Errorf("ranking missing players: %s", rankJSON)
	}
}

func TestMCPCreateGroupViaTool(t *testing.T) {
	rg := newRig(t)
	_, u := rg.loginAs("oidc-mcp", "mcp@example.com", "MCP User")
	rg.mcp.MCPUser = u.OIDCSubject

	res := mcpRPC(t, rg.srv.URL+"/mcp", "secret-token", "tools/call", map[string]any{
		"name":      "create_group",
		"arguments": map[string]any{"slug": "my-group", "name": "My Group"},
	})
	if res.Error != nil {
		t.Fatalf("create_group transport error: %+v", res.Error)
	}
	out, _ := res.Result.(map[string]any)
	if isErr, _ := out["isError"].(bool); isErr {
		t.Fatalf("create_group result error: %+v", out)
	}
	g, err := rg.svc.Store.GroupBySlug(context.Background(), "my-group")
	if err != nil {
		t.Fatal(err)
	}
	ok, _ := rg.svc.Store.IsGroupAdmin(context.Background(), u.ID, g.ID)
	if !ok {
		t.Fatal("MCP caller should be admin of the new group")
	}

	// list_my_groups should mention it.
	res = mcpRPC(t, rg.srv.URL+"/mcp", "secret-token", "tools/call", map[string]any{
		"name":      "list_my_groups",
		"arguments": map[string]any{},
	})
	body, _ := json.Marshal(res.Result)
	if !bytes.Contains(body, []byte("my-group")) {
		t.Errorf("list_my_groups missing new group: %s", body)
	}
}

func TestMCPRejectsNonAdmin(t *testing.T) {
	rg := newRig(t)
	_, u := rg.loginAs("oidc-mcp", "mcp@example.com", "MCP User")
	rg.mcp.MCPUser = u.OIDCSubject

	// Group exists but the MCP user is NOT its admin.
	_, _ = rg.svc.CreateGroupWithSlug(context.Background(), "foreign", "Foreign")

	res := mcpRPC(t, rg.srv.URL+"/mcp", "secret-token", "tools/call", map[string]any{
		"name":      "list_players",
		"arguments": map[string]any{"group": "foreign"},
	})
	out, _ := res.Result.(map[string]any)
	if isErr, _ := out["isError"].(bool); !isErr {
		t.Fatalf("expected isError=true for non-admin caller, got %+v", out)
	}
}

func TestMCPRequiresAuth(t *testing.T) {
	rg := newRig(t)
	req, _ := http.NewRequest("POST", rg.srv.URL+"/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
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
