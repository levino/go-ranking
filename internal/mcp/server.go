package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/levino/go-ranking/internal/auth"
	"github.com/levino/go-ranking/internal/service"
	"github.com/levino/go-ranking/internal/store"
)

// Server is an MCP HTTP endpoint that authenticates callers via OAuth
// 2.0 bearer tokens issued by an OIDC authorization server (Zitadel).
// On 401 it emits the RFC 9728 WWW-Authenticate header pointing at the
// resource-metadata document, which MCP clients use to discover the
// authorization server.
type Server struct {
	Service  *service.Service
	OIDC     *auth.OIDC
	Resource string // canonical public URL of this MCP endpoint

	// userinfo cache to avoid hitting Zitadel on every MCP call.
	// Tokens are typically valid for ~1h; we cache for 60s so that
	// revocations propagate quickly but the hot path stays fast.
	cacheTTL time.Duration
	mu       sync.Mutex
	cache    map[string]cachedUser
}

type cachedUser struct {
	user *store.User
	exp  time.Time
}

// userCtxKey is the context key for the authenticated user. Tools read
// the caller from request context rather than from a server field.
type userCtxKey struct{}

func userFromCtx(ctx context.Context) (*store.User, bool) {
	u, ok := ctx.Value(userCtxKey{}).(*store.User)
	return u, ok
}

// Handler returns an http.Handler that serves the MCP endpoint at /mcp.
// The protected-resource discovery endpoint is served separately by the
// root mux (see cmd/server/main.go) because it lives at /.well-known/...
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /mcp", s.handleRPC)
	mux.HandleFunc("GET /mcp", s.handleSSEStream)
	return mux
}

// HandleProtectedResource serves the RFC 9728 protected-resource
// metadata document. Register this on the root mux at
// `/.well-known/oauth-protected-resource`.
func (s *Server) HandleProtectedResource(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"resource":                 s.Resource,
		"authorization_servers":    []string{s.OIDC.Issuer},
		"bearer_methods_supported": []string{"header"},
		"scopes_supported":         []string{"openid", "email", "profile"},
	})
}

// authenticate resolves the request's Bearer token to a user. It calls
// Zitadel's userinfo endpoint (which acts as a validation oracle: a
// successful response means the token is valid and not revoked) and
// upserts the user record from the resulting claims.
func (s *Server) authenticate(r *http.Request) (*store.User, error) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return nil, errors.New("missing bearer")
	}
	tok := strings.TrimPrefix(h, "Bearer ")
	if tok == "" {
		return nil, errors.New("empty bearer")
	}
	if u := s.cacheLookup(tok); u != nil {
		return u, nil
	}
	info, err := s.OIDC.Userinfo(r.Context(), tok)
	if err != nil {
		return nil, fmt.Errorf("userinfo: %w", err)
	}
	user, err := s.Service.Store.UpsertUserByOIDC(r.Context(), info.Subject, info.Email, info.DisplayName())
	if err != nil {
		return nil, err
	}
	s.cacheStore(tok, user)
	return user, nil
}

func (s *Server) cacheLookup(tok string) *store.User {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.cache[tok]
	if !ok || time.Now().After(c.exp) {
		return nil
	}
	return c.user
}

func (s *Server) cacheStore(tok string, u *store.User) {
	ttl := s.cacheTTL
	if ttl == 0 {
		ttl = 60 * time.Second
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cache == nil {
		s.cache = map[string]cachedUser{}
	}
	s.cache[tok] = cachedUser{user: u, exp: time.Now().Add(ttl)}
}

// unauthorized writes a spec-compliant 401 response that points MCP
// clients at the resource-metadata document, so they can discover the
// authorization server and initiate the OAuth flow.
func (s *Server) unauthorized(w http.ResponseWriter, reason string) {
	metaURL := strings.TrimSuffix(s.Resource, "/mcp") + "/.well-known/oauth-protected-resource"
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(
		`Bearer realm="%s", resource_metadata="%s"`, s.OIDC.Issuer, metaURL))
	http.Error(w, "unauthorized: "+reason, http.StatusUnauthorized)
}

func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	user, err := s.authenticate(r)
	if err != nil {
		s.unauthorized(w, err.Error())
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeJSONError(w, nil, codeParseError, err.Error())
		return
	}
	ctx := context.WithValue(r.Context(), userCtxKey{}, user)
	trimmed := strings.TrimSpace(string(body))
	wantSSE := strings.Contains(r.Header.Get("Accept"), "text/event-stream")
	if strings.HasPrefix(trimmed, "[") {
		var batch []Request
		if err := json.Unmarshal(body, &batch); err != nil {
			writeJSONError(w, nil, codeParseError, err.Error())
			return
		}
		var out []Response
		for _, req := range batch {
			if resp := s.dispatch(ctx, req); resp != nil {
				out = append(out, *resp)
			}
		}
		writeResponses(w, wantSSE, out)
		return
	}
	var req Request
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONError(w, nil, codeParseError, err.Error())
		return
	}
	resp := s.dispatch(ctx, req)
	if resp == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeResponses(w, wantSSE, []Response{*resp})
}

func (s *Server) handleSSEStream(w http.ResponseWriter, r *http.Request) {
	if _, err := s.authenticate(r); err != nil {
		s.unauthorized(w, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	flusher.Flush()
	<-r.Context().Done()
}

func writeResponses(w http.ResponseWriter, sse bool, resps []Response) {
	if sse {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, _ := w.(http.Flusher)
		for _, resp := range resps {
			b, _ := json.Marshal(resp)
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", b)
			if flusher != nil {
				flusher.Flush()
			}
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if len(resps) == 1 {
		_ = json.NewEncoder(w).Encode(resps[0])
		return
	}
	_ = json.NewEncoder(w).Encode(resps)
}

func writeJSONError(w http.ResponseWriter, id json.RawMessage, code int, msg string) {
	resp := Response{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: code, Message: msg}}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) dispatch(ctx context.Context, req Request) *Response {
	resp := &Response{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		resp.Result = InitializeResult{
			ProtocolVersion: supportedProtocolVersion,
			Capabilities: map[string]any{
				"tools": map[string]any{"listChanged": false},
			},
			ServerInfo: ServerInfo{Name: "go-liga", Version: "0.1.0"},
		}
	case "notifications/initialized":
		return nil
	case "ping":
		resp.Result = map[string]any{}
	case "tools/list":
		resp.Result = ToolsListResult{Tools: toolDefs()}
	case "tools/call":
		var p ToolCallParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			resp.Error = &RPCError{Code: codeInvalidParams, Message: err.Error()}
			return resp
		}
		out, err := s.callTool(ctx, p)
		if err != nil {
			resp.Result = errorResult(err.Error())
			return resp
		}
		resp.Result = out
	default:
		if req.ID == nil {
			return nil
		}
		resp.Error = &RPCError{Code: codeMethodNotFound, Message: req.Method}
	}
	return resp
}

// resolveAdminGroup loads the group referenced by args and verifies the
// caller is one of its admins.
func (s *Server) resolveAdminGroup(ctx context.Context, args map[string]any) (*store.Group, *store.User, error) {
	user, ok := userFromCtx(ctx)
	if !ok {
		return nil, nil, errors.New("no caller in context")
	}
	slug, _ := args["group"].(string)
	if slug == "" {
		return nil, nil, fmt.Errorf("missing group slug — use list_my_groups to see options")
	}
	g, err := s.Service.Store.GroupBySlug(ctx, slug)
	if err != nil {
		return nil, nil, fmt.Errorf("unknown group: %s", slug)
	}
	ok, err = s.Service.Store.IsGroupAdmin(ctx, user.ID, g.ID)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, fmt.Errorf("you are not an admin of %q", slug)
	}
	return g, user, nil
}
