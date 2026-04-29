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
	// AuthToken is a shared bearer token; empty disables auth.
	AuthToken string
	// DefaultGroupSlug is used by tools that take a group slug parameter
	// when none is given.
	DefaultGroupSlug string
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
	// MCP allows a single request or a JSON-RPC batch.
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
		// notification — no body
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeResponses(w, wantSSE, []Response{*resp})
}

// handleSSEStream serves a no-op SSE channel — we have nothing to push,
// but Claude.ai expects GET /mcp to be available. We simply hold the
// connection open until the client disconnects.
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

// dispatch routes a single request to the right handler. Notifications
// (no ID) return nil.
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
			return nil // unknown notification, ignore
		}
		resp.Error = &RPCError{Code: codeMethodNotFound, Message: req.Method}
	}
	return resp
}

// resolveGroup loads the group referenced by the args (or the default).
func (s *Server) resolveGroup(ctx context.Context, args map[string]any) (*store.Group, error) {
	slug, _ := args["group"].(string)
	if slug == "" {
		slug = s.DefaultGroupSlug
	}
	if slug == "" {
		return nil, fmt.Errorf("missing group slug")
	}
	return s.Service.Store.GroupBySlug(ctx, slug)
}
