package faults

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/faultforge/faultforge/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvalidJSONFault_Inject(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		wantBody    string
		wantType    types.FaultType
		wantStatus  int
		wantDefault bool
	}{
		{
			name:       "uses configured payload",
			payload:    `{"broken: true`,
			wantBody:   `{"broken: true`,
			wantType:   types.FaultInvalidJSON,
			wantStatus: http.StatusOK,
		},
		{
			name:        "uses default broken JSON when no payload",
			payload:     "",
			wantBody:    defaultBrokenJSON,
			wantType:    types.FaultInvalidJSON,
			wantStatus:  http.StatusOK,
			wantDefault: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := NewInvalidJSONFault(FaultConfig{Payload: tt.payload})
			require.NoError(t, err)
			assert.Equal(t, tt.wantType, f.Type())

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)

			err = f.Inject(rec, req)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantBody, rec.Body.String())
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
		})
	}
}
