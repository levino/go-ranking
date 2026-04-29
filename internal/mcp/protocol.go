// Package mcp implements a minimal Model Context Protocol HTTP/SSE
// server, suitable for use as a Claude.ai connector.
//
// The transport is the "streamable HTTP" variant: JSON-RPC 2.0
// request/response over POST /mcp, optionally streamed via
// Server-Sent Events when the client sets Accept: text/event-stream.
package mcp

import (
	"encoding/json"
	"errors"
)

// JSON-RPC 2.0 wire types.

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *RPCError) Error() string { return e.Message }

const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

// MCP-specific result types.

type InitializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      ServerInfo     `json:"serverInfo"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

type ToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type ToolCallResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

func textResult(s string) *ToolCallResult {
	return &ToolCallResult{Content: []ContentBlock{{Type: "text", Text: s}}}
}

func errorResult(s string) *ToolCallResult {
	return &ToolCallResult{Content: []ContentBlock{{Type: "text", Text: s}}, IsError: true}
}

// Protocol version we advertise. Clients negotiate by sending a
// version in `initialize`; we echo it back if compatible.
const supportedProtocolVersion = "2025-03-26"

func notImplemented(method string) error {
	return errors.New("method not implemented: " + method)
}
