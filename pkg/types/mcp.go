package types

// JSONRPCRequest is a JSON-RPC 2.0 request message.
// ID is either string, number, or null per JSON-RPC 2.0 §4;
// interface{} preserves the original JSON type across marshal
// round-trips.
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// JSONRPCResponse is a JSON-RPC 2.0 response message.
// Exactly one of Result or Error is set per JSON-RPC 2.0 §5 —
// enforced at the application layer, not by the type.
type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      interface{}   `json:"id,omitempty"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

// JSONRPCError is the error object in a JSON-RPC 2.0 error response.
// Code uses the reserved range defined by JSON-RPC 2.0 §5.1: the
// MCP handler emits -32700 (parse error), -32600 (invalid request),
// and -32603 (internal error) from that range.
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ToolCallParams holds the params for a tools/call JSON-RPC request.
type ToolCallParams struct {
	Name      string      `json:"name"`
	Arguments interface{} `json:"arguments,omitempty"`
}

// ToolCallResult holds the result of a tools/call JSON-RPC response.
type ToolCallResult struct {
	Content []ToolContent `json:"content"`
}

// ToolContent is a text content block in a tool call result.
// MCP v1 scope: only Type="text" is handled (see ADR-011);
// image/resource blocks land in v2.
type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}
