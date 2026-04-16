package mcp

// ObservationSink is the contract the MCP handler uses to hand per-test
// observations to an owning proxy. A sink implementation is typically the
// surrounding *proxy.Proxy: defining the interface here (consumer side)
// lets the MCP handler record observations without importing the proxy
// package, which would create an import cycle (proxy already imports mcp
// for auto-dispatch).
//
// RecordFault is called every time the MCP handler injects a fault on a
// matched tools/call. Implementations MUST treat every RecordFault call
// as an error — MCP fault responses ride on HTTP 200 and carry the error
// in the JSON-RPC envelope, so the status code alone cannot disambiguate.
//
// RecordPassthrough is called for matched-but-not-fired calls that reach
// the upstream MCP server, so the evaluator can tell "agent exercised
// this tool but got a healthy response" from "agent never touched the
// tool at all".
type ObservationSink interface {
	RecordFault(testID string, statusCode int)
	RecordPassthrough(testID string, statusCode int)
}
