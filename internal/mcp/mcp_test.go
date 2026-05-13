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

	"github.com/levino/go-ranking/internal/service"
	"github.com/levino/go-ranking/internal/store"
)

func newTestServer(t *testing.T) (*Server, *store.User) {
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
	return &Server{Service: service.New(st), MCPUser: u.OIDCSubject}, u
}

func post(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestInitializeNegotiation(t *testing.T) {
	s, _ := newTestServer(t)
	w := post(t, s.Handler(), `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
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

func TestNotificationHasNoBody(t *testing.T) {
	s, _ := newTestServer(t)
	w := post(t, s.Handler(), `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("expected empty body, got %q", w.Body.String())
	}
}

func TestUnknownMethodReturnsError(t *testing.T) {
	s, _ := newTestServer(t)
	w := post(t, s.Handler(), `{"jsonrpc":"2.0","id":1,"method":"foo/bar"}`)
	var resp Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != codeMethodNotFound {
		t.Fatalf("expected method-not-found, got %+v", resp.Error)
	}
}

func TestParseErrorReturnsParseCode(t *testing.T) {
	s, _ := newTestServer(t)
	w := post(t, s.Handler(), `not json`)
	var resp Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != codeParseError {
		t.Fatalf("expected parse error, got %+v", resp.Error)
	}
}

func TestBatchRequestReturnsArray(t *testing.T) {
	s, _ := newTestServer(t)
	body := `[
        {"jsonrpc":"2.0","id":1,"method":"tools/list"},
        {"jsonrpc":"2.0","id":2,"method":"ping"}
    ]`
	w := post(t, s.Handler(), body)
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

func TestCreateGroupViaMCPMakesCallerAdmin(t *testing.T) {
	s, u := newTestServer(t)
	ctx := context.Background()
	out, err := s.toolCreateGroup(ctx, map[string]any{"slug": "test-group", "name": "Test"})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("create_group failed: %+v", out)
	}
	g, err := s.Service.Store.GroupBySlug(ctx, "test-group")
	if err != nil {
		t.Fatalf("group not created: %v", err)
	}
	ok, err := s.Service.Store.IsGroupAdmin(ctx, u.ID, g.ID)
	if err != nil || !ok {
		t.Fatalf("caller is not admin: %v", err)
	}
}

func TestResolveAdminGroupRejectsNonAdmin(t *testing.T) {
	s, _ := newTestServer(t)
	ctx := context.Background()
	g, _ := s.Service.CreateGroup(ctx, "Other")
	// caller has no group_admins entry → reject
	_, _, err := s.resolveAdminGroup(ctx, map[string]any{"group": g.Slug})
	if err == nil {
		t.Fatal("expected admin check to fail")
	}
}
