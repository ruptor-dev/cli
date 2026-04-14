package types

import "net/http"

type FaultType string

const (
	FaultToolTimeout   FaultType = "tool_timeout"
	FaultSlowResponse  FaultType = "slow_response"
	FaultToolError     FaultType = "tool_error"
	FaultInvalidJSON   FaultType = "invalid_json"
	FaultEmptyResponse FaultType = "empty_response"
	FaultRateLimit     FaultType = "rate_limit"
	FaultLLMError      FaultType = "llm_error"
	FaultLLMTimeout    FaultType = "llm_timeout"
)

type Fault interface {
	Type() FaultType
	Inject(w http.ResponseWriter, r *http.Request) error
}
