package faults

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ruptor-dev/cli/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimeoutFault_Inject(t *testing.T) {
	tests := []struct {
		name       string
		cfg        FaultConfig
		wantStatus int
		wantType   types.FaultType
	}{
		{
			name:       "returns 504 gateway timeout",
			cfg:        FaultConfig{},
			wantStatus: http.StatusGatewayTimeout,
			wantType:   types.FaultToolTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := NewTimeoutFault(tt.cfg)
			require.NoError(t, err)
			assert.Equal(t, tt.wantType, f.Type())

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)

			err = f.Inject(rec, req)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}
