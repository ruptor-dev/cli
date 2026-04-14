package faults

import (
	"net/http"

	"github.com/faultforge/faultforge/pkg/types"
)

// EmptyResponseFault returns HTTP 200 with an empty body.
type EmptyResponseFault struct {
	BaseFault
}

// NewEmptyResponseFault constructs an EmptyResponseFault.
func NewEmptyResponseFault(cfg FaultConfig) (types.Fault, error) {
	cfg.Type = types.FaultEmptyResponse
	return &EmptyResponseFault{BaseFault: BaseFault{Cfg: cfg}}, nil
}

// Inject writes an HTTP 200 with no body.
func (f *EmptyResponseFault) Inject(w http.ResponseWriter, _ *http.Request) error {
	w.WriteHeader(http.StatusOK)
	return nil
}
