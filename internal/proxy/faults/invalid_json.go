package faults

import (
	"net/http"

	"github.com/faultforge/faultforge/pkg/types"
)

const defaultBrokenJSON = `{"result": INVALID, "data": [1, 2,}`

// InvalidJSONFault returns HTTP 200 with a malformed JSON body.
type InvalidJSONFault struct {
	BaseFault
}

// NewInvalidJSONFault constructs an InvalidJSONFault. If no Payload is configured,
// a default broken JSON string is used.
func NewInvalidJSONFault(cfg FaultConfig) (types.Fault, error) {
	if cfg.Payload == "" {
		cfg.Payload = defaultBrokenJSON
	}
	cfg.Type = types.FaultInvalidJSON
	return &InvalidJSONFault{BaseFault: BaseFault{Cfg: cfg}}, nil
}

// Inject writes HTTP 200 with the malformed JSON payload.
func (f *InvalidJSONFault) Inject(w http.ResponseWriter, _ *http.Request) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(f.Cfg.Payload))
	return err
}
