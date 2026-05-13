package mcp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/levino/go-ranking/internal/service"
	"github.com/levino/go-ranking/internal/store"
)

// Server holds dependencies for handling MCP RPCs.
type Server struct {
	Service *service.Service

	// AuthToken is a shared bearer token; empty disables auth and the
	// server falls back to MCPUser (useful for tests).
	AuthToken string

	// MCPUser is the OIDC subject the shared token acts as. Every tool
	// call is checked against this user's group admin memberships. Must
	// be set whenever AuthToken is set.
	MCPUser string
}

// Handler returns an http.Handler that serves the MCP endpoint at /mcp.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /mcp", s.handleRPC)
	mux.HandleFunc("GET /mcp", s.handleSSEStream) // optional server stream
	return mux
}

func (s *Server) authorize(r *http.Request) bool {
	if s.AuthToken == "" {
		return true
	}
	got := r.Header.Get("Authorization")
	got = strings.TrimPrefix(got, "Bearer ")
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.AuthToken)) == 1
}

// callerUser resolves the user the current request acts as. In phase 1
// the user is fixed (MCPUser). When OAuth lands, this will inspect the
// access token claims instead.
func (s *Server) callerUser(ctx context.Context) (*store.User, error) {
	if s.MCPUser == "" {
		return nil, fmt.Errorf("MCP user not configured (set GO_LIGA_MCP_USER)")
	}
	u, err := s.Service.Store.UserByOIDC(ctx, s.MCPUser)
	if err != nil {
		return nil, fmt.Errorf("configured MCP user %q not in DB — log in via the web UI once to create the record", s.MCPUser)
	}
	return u, nil
}

func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeJSONError(w, nil, codeParseError, err.Error())
		return
	}
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
			if resp := s.dispatch(r.Context(), req); resp != nil {
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
	resp := s.dispatch(r.Context(), req)
	if resp == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeResponses(w, wantSSE, []Response{*resp})
}

func (s *Server) handleSSEStream(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
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
	user, err := s.callerUser(ctx)
	if err != nil {
		return nil, nil, err
	}
	slug, _ := args["group"].(string)
	if slug == "" {
		return nil, nil, fmt.Errorf("missing group slug — use list_my_groups to see options")
	}
	g, err := s.Service.Store.GroupBySlug(ctx, slug)
	if err != nil {
		return nil, nil, fmt.Errorf("unknown group: %s", slug)
	}
	ok, err := s.Service.Store.IsGroupAdmin(ctx, user.ID, g.ID)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, fmt.Errorf("you are not an admin of %q", slug)
	}
	return g, user, nil
}
