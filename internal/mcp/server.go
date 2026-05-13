package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/levino/go-ranking/internal/auth"
	"github.com/levino/go-ranking/internal/service"
	"github.com/levino/go-ranking/internal/store"
)

// Server is an MCP HTTP endpoint that authenticates callers via OAuth
// 2.0 bearer tokens issued by an OIDC authorization server (Zitadel).
// On 401 it emits the RFC 9728 WWW-Authenticate header pointing at the
// resource-metadata document, which MCP clients use to discover the
// authorization server.
//
// To support MCP clients that insist on Dynamic Client Registration
// (Claude.ai), the server also exposes a thin OAuth-AS facade — see
// oauth.go — that proxies authorize/token to Zitadel and returns a
// pre-registered client id from any "registration" request.
type Server struct {
	Service *service.Service
	Signer  *auth.Signer // shared with web — same HMAC key signs sessions and OAuth JWTs
	OIDC    *auth.OIDC   // upstream IdP; we redirect to it from /auth/start

	Resource string // canonical public URL of this MCP endpoint
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
//
// We advertise OURSELVES as the authorization server (via the OAuth-AS
// facade in oauth.go) rather than Zitadel directly — that gives us a
// place to handle Dynamic Client Registration for clients that need it.
func (s *Server) HandleProtectedResource(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"resource":                 s.Resource,
		"authorization_servers":    []string{s.publicBase()},
		"bearer_methods_supported": []string{"header"},
		"scopes_supported":         []string{"openid", "email", "profile"},
	})
}

// authenticate resolves the request's Bearer token (an access token we
// issued via /oauth/token) to a user. The token is an HS256 JWT we
// validate locally — no upstream call needed on the hot path.
func (s *Server) authenticate(r *http.Request) (*store.User, error) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return nil, errors.New("missing bearer")
	}
	tok := strings.TrimPrefix(h, "Bearer ")
	if tok == "" {
		return nil, errors.New("empty bearer")
	}
	claims, err := s.Signer.VerifyAccess(tok, s.publicBase(), s.Resource)
	if err != nil {
		return nil, fmt.Errorf("verify token: %w", err)
	}
	var uid int64
	if _, err := fmt.Sscanf(claims.Subject, "%d", &uid); err != nil {
		return nil, fmt.Errorf("bad subject %q", claims.Subject)
	}
	user, err := s.Service.Store.UserByID(r.Context(), uid)
	if err != nil {
		return nil, fmt.Errorf("user lookup: %w", err)
	}
	return user, nil
}

// unauthorized writes a spec-compliant 401 response that points MCP
// clients at the resource-metadata document, so they can discover the
// authorization server and initiate the OAuth flow.
func (s *Server) unauthorized(w http.ResponseWriter, reason string) {
	metaURL := s.publicBase() + "/.well-known/oauth-protected-resource"
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(
		`Bearer realm="%s", resource_metadata="%s"`, s.publicBase(), metaURL))
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
