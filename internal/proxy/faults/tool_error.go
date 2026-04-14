package faults

import (
	"net/http"

	"github.com/ruptor-dev/cli/pkg/types"
)

// ToolErrorFault returns a configurable HTTP error status with a JSON body.
type ToolErrorFault struct {
	BaseFault
}

// NewToolErrorFault constructs a ToolErrorFault, defaulting StatusCode to 500.
func NewToolErrorFault(cfg FaultConfig) (types.Fault, error) {
	if cfg.StatusCode == 0 {
		cfg.StatusCode = http.StatusInternalServerError
	}
	cfg.Type = types.FaultToolError
	return &ToolErrorFault{BaseFault: BaseFault{Cfg: cfg}}, nil
}

// Inject writes the configured status code and body.
func (f *ToolErrorFault) Inject(w http.ResponseWriter, _ *http.Request) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(f.Cfg.StatusCode)
	if f.Cfg.Body != "" {
		_, err := w.Write([]byte(f.Cfg.Body))
		return err
	}
	return nil
}
