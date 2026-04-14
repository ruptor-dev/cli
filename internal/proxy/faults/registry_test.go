package faults

import (
	"testing"

	"github.com/ruptor-dev/cli/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFaultRegistry_Register_And_Build(t *testing.T) {
	tests := []struct {
		name      string
		ft        types.FaultType
		cfg       FaultConfig
		wantType  types.FaultType
		wantErr   bool
		errSubstr string
	}{
		{
			name:     "build registered fault",
			ft:       types.FaultToolTimeout,
			cfg:      FaultConfig{},
			wantType: types.FaultToolTimeout,
		},
		{
			name:      "build unknown fault returns error",
			ft:        types.FaultType("nonexistent"),
			cfg:       FaultConfig{},
			wantErr:   true,
			errSubstr: "unknown fault type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewFaultRegistry()
			f, err := r.Build(tt.ft, tt.cfg)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
				assert.Nil(t, f)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantType, f.Type())
		})
	}
}

func TestNewFaultRegistry_Has_All_Six_Faults(t *testing.T) {
	r := NewFaultRegistry()

	allTypes := []types.FaultType{
		types.FaultToolTimeout,
		types.FaultSlowResponse,
		types.FaultToolError,
		types.FaultInvalidJSON,
		types.FaultEmptyResponse,
		types.FaultRateLimit,
	}

	for _, ft := range allTypes {
		t.Run(string(ft), func(t *testing.T) {
			_, ok := r.faults[ft]
			assert.True(t, ok, "expected fault type %s to be registered", ft)
		})
	}
}

func TestFaultRegistry_Build_Propagates_Factory_Error(t *testing.T) {
	r := NewFaultRegistry()

	// SlowResponseFault requires DelayMs > 0, so an empty config should fail.
	_, err := r.Build(types.FaultSlowResponse, FaultConfig{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "faults: building slow_response")
	assert.Contains(t, err.Error(), "DelayMs")
}
