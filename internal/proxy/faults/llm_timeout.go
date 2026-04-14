package faults

import (
	"net/http"
	"time"

	"github.com/ruptor-dev/cli/pkg/types"
)

// defaultLLMTimeoutDelay is how long the fault hangs before surfacing a
// 504 when the caller does not set DelayMs. 30s matches the typical
// client-side LLM API timeout — long enough to exercise the agent's
// timeout / retry logic, short enough that a test run still completes.
const defaultLLMTimeoutDelay = 30 * time.Second

// LLMTimeoutFault simulates an LLM backend that fails to respond: the
// connection stays open without producing bytes, then eventually a 504
// is written. Distinct from tool_timeout because reports tag this as an
// LLM-backend stall — the agent's remediation path (switch model,
// re-prompt, fall back to cache) differs from a tool-side stall.
//
// Behaviour: block DelayMs milliseconds (default 30 s), respecting
// context cancellation so tests cancelling early don't leak goroutines,
// then write 504. A zero/negative DelayMs uses the default.
type LLMTimeoutFault struct {
	BaseFault
}

// NewLLMTimeoutFault constructs an LLMTimeoutFault. Validates nothing:
// all of the config is optional and the fault has sensible defaults.
func NewLLMTimeoutFault(cfg FaultConfig) (types.Fault, error) {
	cfg.Type = types.FaultLLMTimeout
	return &LLMTimeoutFault{BaseFault: BaseFault{Cfg: cfg}}, nil
}

// Inject hangs for the configured delay, then writes HTTP 504. Respects
// request context cancellation so the agent's deadline still fires
// cleanly.
func (f *LLMTimeoutFault) Inject(w http.ResponseWriter, r *http.Request) error {
	delay := time.Duration(f.Cfg.DelayMs) * time.Millisecond
	if delay <= 0 {
		delay = defaultLLMTimeoutDelay
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		w.WriteHeader(http.StatusGatewayTimeout)
		return nil
	case <-r.Context().Done():
		return r.Context().Err()
	}
}
