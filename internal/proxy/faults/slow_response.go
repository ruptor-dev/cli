package faults

import (
	"fmt"
	"net/http"
	"time"

	"github.com/faultforge/faultforge/pkg/types"
)

// SlowResponseFault introduces an artificial delay before forwarding the response.
type SlowResponseFault struct {
	BaseFault
}

// NewSlowResponseFault constructs a SlowResponseFault, requiring DelayMs > 0.
func NewSlowResponseFault(cfg FaultConfig) (types.Fault, error) {
	if cfg.DelayMs <= 0 {
		return nil, fmt.Errorf("DelayMs must be > 0, got %d", cfg.DelayMs)
	}
	cfg.Type = types.FaultSlowResponse
	return &SlowResponseFault{BaseFault: BaseFault{Cfg: cfg}}, nil
}

// Inject sleeps for the configured duration, respecting context cancellation.
func (f *SlowResponseFault) Inject(w http.ResponseWriter, r *http.Request) error {
	delay := time.Duration(f.Cfg.DelayMs) * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		w.WriteHeader(http.StatusOK)
		return nil
	case <-r.Context().Done():
		return r.Context().Err()
	}
}
