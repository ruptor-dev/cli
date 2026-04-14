package faults

import "github.com/faultforge/faultforge/pkg/types"

// FaultConfig holds the configuration for constructing any fault type.
type FaultConfig struct {
	Type        types.FaultType
	DelayMs     int
	StatusCode  int
	Body        string
	Payload     string
	Probability float64
	RetryAfterS int
}

// BaseFault provides the common Type() implementation shared by all faults.
type BaseFault struct {
	Cfg FaultConfig
}

// Type returns the fault type from the embedded configuration.
func (b *BaseFault) Type() types.FaultType {
	return b.Cfg.Type
}
