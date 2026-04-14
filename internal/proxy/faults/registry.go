package faults

import (
	"fmt"

	"github.com/faultforge/faultforge/pkg/types"
)

// FaultFactory is a constructor function that produces a Fault from a FaultConfig.
type FaultFactory func(cfg FaultConfig) (types.Fault, error)

// FaultRegistry maps fault types to their factory functions.
type FaultRegistry struct {
	faults map[types.FaultType]FaultFactory
}

// NewFaultRegistry creates a registry pre-loaded with all built-in fault types.
func NewFaultRegistry() *FaultRegistry {
	r := &FaultRegistry{
		faults: make(map[types.FaultType]FaultFactory),
	}

	r.Register(types.FaultToolTimeout, NewTimeoutFault)
	r.Register(types.FaultSlowResponse, NewSlowResponseFault)
	r.Register(types.FaultToolError, NewToolErrorFault)
	r.Register(types.FaultInvalidJSON, NewInvalidJSONFault)
	r.Register(types.FaultEmptyResponse, NewEmptyResponseFault)
	r.Register(types.FaultRateLimit, NewRateLimitFault)

	return r
}

// Register adds a fault factory for the given type.
func (r *FaultRegistry) Register(ft types.FaultType, factory FaultFactory) {
	r.faults[ft] = factory
}

// Build constructs a fault of the given type using the provided configuration.
// It returns a descriptive error if the fault type is not registered.
func (r *FaultRegistry) Build(ft types.FaultType, cfg FaultConfig) (types.Fault, error) {
	factory, ok := r.faults[ft]
	if !ok {
		return nil, fmt.Errorf("faults: building %s: unknown fault type", ft)
	}

	f, err := factory(cfg)
	if err != nil {
		return nil, fmt.Errorf("faults: building %s: %w", ft, err)
	}

	return f, nil
}
