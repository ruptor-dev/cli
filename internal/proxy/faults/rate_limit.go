package faults

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/faultforge/faultforge/pkg/types"
)

// RateLimitFault returns HTTP 429 with a Retry-After header and JSON body.
type RateLimitFault struct {
	BaseFault
}

// NewRateLimitFault constructs a RateLimitFault, requiring RetryAfterS > 0.
func NewRateLimitFault(cfg FaultConfig) (types.Fault, error) {
	if cfg.RetryAfterS <= 0 {
		return nil, fmt.Errorf("RetryAfterS must be > 0, got %d", cfg.RetryAfterS)
	}
	cfg.Type = types.FaultRateLimit
	return &RateLimitFault{BaseFault: BaseFault{Cfg: cfg}}, nil
}

// Inject writes an HTTP 429 response with Retry-After header and JSON error body.
func (f *RateLimitFault) Inject(w http.ResponseWriter, _ *http.Request) error {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", strconv.Itoa(f.Cfg.RetryAfterS))
	w.WriteHeader(http.StatusTooManyRequests)

	body := fmt.Sprintf(`{"error": "rate limit exceeded", "retry_after": %d}`, f.Cfg.RetryAfterS)
	_, err := w.Write([]byte(body))
	return err
}
