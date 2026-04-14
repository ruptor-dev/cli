package faults

import (
	"net/http"

	"github.com/faultforge/faultforge/pkg/types"
)

// TimeoutFault immediately returns an HTTP 504 Gateway Timeout,
// simulating a tool that does not respond.
type TimeoutFault struct {
	BaseFault
}

// NewTimeoutFault constructs a TimeoutFault from the given config.
func NewTimeoutFault(cfg FaultConfig) (types.Fault, error) {
	cfg.Type = types.FaultToolTimeout
	return &TimeoutFault{BaseFault: BaseFault{Cfg: cfg}}, nil
}

// Inject writes an HTTP 504 response.
func (f *TimeoutFault) Inject(w http.ResponseWriter, _ *http.Request) error {
	w.WriteHeader(http.StatusGatewayTimeout)
	return nil
}
