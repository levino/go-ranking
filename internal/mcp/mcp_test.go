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

func newTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return &Server{Service: service.New(st)}
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
	s := newTestServer(t)
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
	s := newTestServer(t)
	w := post(t, s.Handler(), `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("expected empty body, got %q", w.Body.String())
	}
}

func TestUnknownMethodReturnsError(t *testing.T) {
	s := newTestServer(t)
	w := post(t, s.Handler(), `{"jsonrpc":"2.0","id":1,"method":"foo/bar"}`)
	var resp Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != codeMethodNotFound {
		t.Fatalf("expected method-not-found, got %+v", resp.Error)
	}
}

func TestParseErrorReturnsParseCode(t *testing.T) {
	s := newTestServer(t)
	w := post(t, s.Handler(), `not json`)
	var resp Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != codeParseError {
		t.Fatalf("expected parse error, got %+v", resp.Error)
	}
}

func TestBatchRequestReturnsArray(t *testing.T) {
	s := newTestServer(t)
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

func TestResolveGroupUsesDefault(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()
	g, _ := s.Service.CreateGroup(ctx, "Default")
	s.DefaultGroupSlug = g.Slug

	got, err := s.resolveGroup(ctx, map[string]any{})
	if err != nil || got.ID != g.ID {
		t.Fatalf("default group not used: %v", err)
	}
}

func TestResolveGroupExplicitOverride(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()
	g1, _ := s.Service.CreateGroup(ctx, "One")
	g2, _ := s.Service.CreateGroup(ctx, "Two")
	s.DefaultGroupSlug = g1.Slug

	got, err := s.resolveGroup(ctx, map[string]any{"group": g2.Slug})
	if err != nil || got.ID != g2.ID {
		t.Fatalf("explicit override failed: %v", err)
	}
}
