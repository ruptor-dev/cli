package faults

import (
	"net/http"
	"time"

	"github.com/ruptor-dev/cli/pkg/types"
)

// TimeoutFault simulates a tool that does not respond. When DelayMs is
// set, the handler holds the connection for that duration (respecting
// the request's context for client-side cancellation) before writing
// the 504; this matches how a real hung upstream behaves and lets
// experiments exercise the agent's own timeout path. When DelayMs is
// zero or negative the handler returns 504 immediately, preserving
// the original behaviour for unit tests and quick smoke checks.
type TimeoutFault struct {
	BaseFault
}

// NewTimeoutFault constructs a TimeoutFault from the given config.
func NewTimeoutFault(cfg FaultConfig) (types.Fault, error) {
	cfg.Type = types.FaultToolTimeout
	return &TimeoutFault{BaseFault: BaseFault{Cfg: cfg}}, nil
}

// Inject writes an HTTP 504 response, optionally after a hold.
func (f *TimeoutFault) Inject(w http.ResponseWriter, r *http.Request) error {
	if f.Cfg.DelayMs > 0 {
		timer := time.NewTimer(time.Duration(f.Cfg.DelayMs) * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-r.Context().Done():
			return r.Context().Err()
		}
	}
	w.WriteHeader(http.StatusGatewayTimeout)
	return nil
}
