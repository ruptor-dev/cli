package faults

import (
	"net/http"

	"github.com/ruptor-dev/cli/pkg/types"
)

// defaultLLMErrorBody mimics the OpenAI Chat Completions error envelope so
// agents that speak that protocol hit the same deserialisation path they
// would in production.
const defaultLLMErrorBody = `{"error":{"message":"service unavailable","type":"server_error","code":"service_unavailable"}}`

// LLMErrorFault simulates a failed LLM backend call: the underlying model
// provider (OpenAI, Anthropic, etc.) returned a 5xx. Distinct from
// tool_error because it is labelled as LLM-path failure in reports, so
// robustness graphs can separate "tool API flaked" from "model API
// flaked" — the agent's recovery paths are often different.
type LLMErrorFault struct {
	BaseFault
}

// NewLLMErrorFault constructs an LLMErrorFault. Defaults:
//   - StatusCode 503 (Service Unavailable is the typical OpenAI / Anthropic
//     soft-fail code)
//   - Body a minimal OpenAI-shaped error envelope when none is configured
func NewLLMErrorFault(cfg FaultConfig) (types.Fault, error) {
	if cfg.StatusCode == 0 {
		cfg.StatusCode = http.StatusServiceUnavailable
	}
	if cfg.Body == "" {
		cfg.Body = defaultLLMErrorBody
	}
	cfg.Type = types.FaultLLMError
	return &LLMErrorFault{BaseFault: BaseFault{Cfg: cfg}}, nil
}

// Inject writes the configured status code and body to the response.
func (f *LLMErrorFault) Inject(w http.ResponseWriter, _ *http.Request) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(f.Cfg.StatusCode)
	_, err := w.Write([]byte(f.Cfg.Body))
	return err
}
