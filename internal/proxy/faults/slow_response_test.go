package faults

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlowResponseFault_Inject(t *testing.T) {
	tests := []struct {
		name     string
		delayMs  int
		wantErr  bool
		minDelay time.Duration
	}{
		{
			name:     "delays for configured duration",
			delayMs:  50,
			minDelay: 40 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := NewSlowResponseFault(FaultConfig{DelayMs: tt.delayMs})
			require.NoError(t, err)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)

			start := time.Now()
			err = f.Inject(rec, req)
			elapsed := time.Since(start)

			require.NoError(t, err)
			assert.GreaterOrEqual(t, elapsed, tt.minDelay)
			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

func TestSlowResponseFault_ContextCancellation(t *testing.T) {
	f, err := NewSlowResponseFault(FaultConfig{DelayMs: 5000})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)

	// Cancel context almost immediately.
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err = f.Inject(rec, req)
	elapsed := time.Since(start)

	assert.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Less(t, elapsed, 1*time.Second, "should return quickly on cancellation")
}

func TestSlowResponseFault_InvalidConfig(t *testing.T) {
	_, err := NewSlowResponseFault(FaultConfig{DelayMs: 0})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DelayMs must be > 0")

	_, err = NewSlowResponseFault(FaultConfig{DelayMs: -10})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DelayMs must be > 0")
}
