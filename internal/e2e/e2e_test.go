// Package e2e exercises the web UI and the MCP API end-to-end against
// a real httptest.Server backed by a temporary SQLite database.
//
// The tests cover the full lifecycle that a teacher and Claude
// (via MCP) would go through:
//
//  1. Bootstrap admin, create a group and a teacher user.
//  2. Teacher logs in, creates players, generates a session.
//  3. Teacher downloads the matrix and score card PDFs.
//  4. Claude (MCP) records a few games, ratings update.
//  5. The dashboard reflects the updated ratings.
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
	webSrv, err := web.New(svc, signer)
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

func (r *rig) seedAdmin(username, password string) {
	r.t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		r.t.Fatal(err)
	}
	if _, err := r.svc.Store.CreateUser(context.Background(), username, hash, nil, true); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) loginClient(username, password string) *http.Client {
	r.t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := c.PostForm(r.srv.URL+"/login", url.Values{
		"username": {username}, "password": {password},
	})
	if err != nil {
		r.t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		r.t.Fatalf("login expected 302, got %d: %s", resp.StatusCode, body)
	}
	if loc := resp.Header.Get("Location"); loc != "/" {
		r.t.Fatalf("login redirect to %s", loc)
	}
	return c
}

// ---- Web UI scenarios ----------------------------------------------------

func TestEndToEndTeacherFlow(t *testing.T) {
	rg := newRig(t)
	rg.seedAdmin("admin", "secret")

	admin := rg.loginClient("admin", "secret")

	// Admin creates a group.
	resp, err := admin.PostForm(rg.srv.URL+"/admin/groups", url.Values{"name": {"Schule Linz"}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("create group: %d", resp.StatusCode)
	}
	g, err := rg.svc.Store.GroupBySlug(context.Background(), "schule-linz")
	if err != nil {
		t.Fatalf("group not created: %v", err)
	}

	// Admin creates a teacher user for that group.
	resp, err = admin.PostForm(rg.srv.URL+"/admin/users", url.Values{
		"username": {"teacher"},
		"password": {"pw1234"},
		"group_id": {fmt.Sprintf("%d", g.ID)},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Teacher logs in.
	teacher := rg.loginClient("teacher", "pw1234")

	// Teacher creates four players.
	for _, p := range []struct{ name, rank string }{
		{"Anna", "10k"},
		{"Ben", "15k"},
		{"Clara", "20k"},
		{"Dirk", "25k"},
	} {
		resp, err := teacher.PostForm(rg.srv.URL+"/g/schule-linz/players", url.Values{
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

	// Teacher creates a session.
	resp, err = teacher.PostForm(rg.srv.URL+"/g/schule-linz/sessions", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("session create: %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "/g/schule-linz/sessions/") {
		t.Fatalf("redirect %q", loc)
	}
	pass := strings.TrimPrefix(loc, "/g/schule-linz/sessions/")

	// Dashboard renders.
	resp, err = teacher.Get(rg.srv.URL + "/g/schule-linz")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("dashboard %d", resp.StatusCode)
	}
	for _, want := range []string{"Anna", "Ben", "Clara", "Dirk", "Schule Linz"} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("dashboard missing %q", want)
		}
	}

	// PDFs.
	for _, suffix := range []string{"matrix.pdf", "scorecard.pdf"} {
		u := fmt.Sprintf("%s/g/schule-linz/sessions/%s/%s", rg.srv.URL, pass, suffix)
		resp, err := teacher.Get(u)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("%s status %d", suffix, resp.StatusCode)
		}
		if !bytes.HasPrefix(b, []byte("%PDF-")) {
			t.Fatalf("%s not a PDF: first bytes %q", suffix, b[:8])
		}
	}

	// Anonymous client can't fetch the dashboard.
	anon := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err = anon.Get(rg.srv.URL + "/g/schule-linz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("anon dashboard expected 302, got %d", resp.StatusCode)
	}

	// A teacher of one group cannot see another group.
	other, _ := rg.svc.CreateGroup(context.Background(), "Other School")
	_ = other
	resp, err = teacher.Get(rg.srv.URL + "/g/other-school")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign group expected 403, got %d", resp.StatusCode)
	}
}

// ---- MCP scenarios -------------------------------------------------------

func TestEndToEndMCPRecordsGame(t *testing.T) {
	rg := newRig(t)
	ctx := context.Background()
	g, _ := rg.svc.CreateGroup(ctx, "MCP")
	a, _ := rg.svc.Store.CreatePlayer(ctx, g.ID, "Anna", 1000)
	b, _ := rg.svc.Store.CreatePlayer(ctx, g.ID, "Ben", 800)
	sess, err := rg.svc.CreateSession(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initialize handshake.
	res := mcpRPC(t, rg.srv.URL+"/mcp", "secret-token", "initialize", map[string]any{
		"protocolVersion": "2025-03-26",
		"clientInfo":      map[string]any{"name": "claude.ai", "version": "test"},
	})
	if res.Error != nil {
		t.Fatalf("init error: %+v", res.Error)
	}

	// 2. Tools list.
	res = mcpRPC(t, rg.srv.URL+"/mcp", "secret-token", "tools/list", map[string]any{})
	if res.Error != nil {
		t.Fatalf("tools/list: %+v", res.Error)
	}
	resJSON, _ := json.Marshal(res.Result)
	for _, want := range []string{"record_game", "list_players", "ranking", "create_session", "get_session"} {
		if !bytes.Contains(resJSON, []byte(want)) {
			t.Errorf("tools/list missing %q", want)
		}
	}

	// 3. Record a game: Ben (#2, weaker) plays Black, Anna plays White, Ben wins.
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
	// Ratings actually changed in the DB.
	bAfter, _ := rg.svc.Store.PlayerByID(ctx, b.ID)
	aAfter, _ := rg.svc.Store.PlayerByID(ctx, a.ID)
	if bAfter.GoR <= 800 {
		t.Errorf("ben should have gained rating; %.1f", bAfter.GoR)
	}
	if aAfter.GoR >= 1000 {
		t.Errorf("anna should have lost rating; %.1f", aAfter.GoR)
	}

	// 4. Ranking via MCP returns the updated values.
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

	// 5. Tool failure (bad passphrase) yields an isError result, not a transport error.
	res = mcpRPC(t, rg.srv.URL+"/mcp", "secret-token", "tools/call", map[string]any{
		"name":      "record_game",
		"arguments": map[string]any{"passphrase": "no-such", "black_number": 1, "white_number": 2, "board_size": "9", "winner": "black"},
	})
	if res.Error != nil {
		t.Fatalf("transport error for bad input: %+v", res.Error)
	}
	out, _ = res.Result.(map[string]any)
	if isErr, _ := out["isError"].(bool); !isErr {
		t.Errorf("expected isError=true for bad passphrase")
	}
}

func TestMCPRequiresAuth(t *testing.T) {
	rg := newRig(t)
	// No bearer token at all.
	req, _ := http.NewRequest("POST", rg.srv.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestMCPSSEResponse(t *testing.T) {
	rg := newRig(t)
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	req, _ := http.NewRequest("POST", rg.srv.URL+"/mcp", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content-type %q", got)
	}
	b, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(b, []byte("event: message")) {
		t.Fatalf("missing SSE framing: %q", b)
	}
	if !bytes.Contains(b, []byte(`"jsonrpc":"2.0"`)) {
		t.Fatalf("payload missing JSON-RPC envelope: %q", b)
	}
}

func TestMCPCreateSessionThenRecord(t *testing.T) {
	// Demonstrates the full Claude-driven workflow:
	// 1. create_session  -> get passphrase
	// 2. record_game x N
	// 3. get_session     -> verify game list
	rg := newRig(t)
	ctx := context.Background()
	g, _ := rg.svc.CreateGroup(ctx, "Round")
	_, _ = rg.svc.Store.CreatePlayer(ctx, g.ID, "P1", 1000)
	_, _ = rg.svc.Store.CreatePlayer(ctx, g.ID, "P2", 1000)
	_, _ = rg.svc.Store.CreatePlayer(ctx, g.ID, "P3", 1000)

	res := mcpRPC(t, rg.srv.URL+"/mcp", "secret-token", "tools/call", map[string]any{
		"name":      "create_session",
		"arguments": map[string]any{"group": "round"},
	})
	if res.Error != nil {
		t.Fatal(res.Error)
	}
	out, _ := res.Result.(map[string]any)
	content, _ := out["content"].([]any)
	if len(content) == 0 {
		t.Fatal("no content in create_session result")
	}
	first, _ := content[0].(map[string]any)
	text, _ := first["text"].(string)
	// passphrase is on the first line after "Neue Session: ".
	pass := strings.TrimSpace(strings.TrimPrefix(strings.SplitN(text, "\n", 2)[0], "Neue Session: "))
	if pass == "" {
		t.Fatalf("could not parse passphrase from %q", text)
	}

	for _, args := range []map[string]any{
		{"passphrase": pass, "black_number": 1, "white_number": 2, "board_size": "9", "winner": "black"},
		{"passphrase": pass, "black_number": 3, "white_number": 1, "board_size": "13", "winner": "white"},
	} {
		res = mcpRPC(t, rg.srv.URL+"/mcp", "secret-token", "tools/call", map[string]any{
			"name": "record_game", "arguments": args,
		})
		if res.Error != nil {
			t.Fatalf("record_game: %v", res.Error)
		}
		o, _ := res.Result.(map[string]any)
		if isErr, _ := o["isError"].(bool); isErr {
			t.Fatalf("record_game returned error result: %+v", o)
		}
	}

	res = mcpRPC(t, rg.srv.URL+"/mcp", "secret-token", "tools/call", map[string]any{
		"name":      "get_session",
		"arguments": map[string]any{"passphrase": pass},
	})
	if res.Error != nil {
		t.Fatal(res.Error)
	}
	body, _ := json.Marshal(res.Result)
	if !bytes.Contains(body, []byte("Partien (2)")) {
		t.Errorf("get_session should report 2 games; got: %s", body)
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
