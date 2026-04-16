package main

import (
	"context"
	"reflect"
	"testing"

	"github.com/rs/zerolog"
	"github.com/ruptor-dev/cli/internal/config"
	"github.com/ruptor-dev/cli/internal/evaluator"
	"github.com/ruptor-dev/cli/internal/evaluator/llmjudge"
	"github.com/ruptor-dev/cli/internal/proxy"
	"github.com/ruptor-dev/cli/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportPathsFor(t *testing.T) {
	cases := []struct {
		name       string
		out        config.OutputConfig
		outputPath string
		want       []string
	}{
		{
			name:       "explicit outputPath overrides",
			out:        config.OutputConfig{Format: "both", Path: "./reports/"},
			outputPath: "/tmp/custom.json",
			want:       []string{"/tmp/custom.json"},
		},
		{
			name: "json format",
			out:  config.OutputConfig{Format: "json", Path: "./reports/"},
			want: []string{"./reports/chaos_report.json"},
		},
		{
			name: "html format",
			out:  config.OutputConfig{Format: "html", Path: "./reports/"},
			want: []string{"./reports/chaos_report.html"},
		},
		{
			name: "both format lists html and json",
			out:  config.OutputConfig{Format: "both", Path: "./reports/"},
			want: []string{"./reports/chaos_report.html", "./reports/chaos_report.json"},
		},
		{
			name: "unknown format returns empty",
			out:  config.OutputConfig{Format: "toml", Path: "./reports/"},
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := reportPathsFor(nil, tc.out, tc.outputPath)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("reportPathsFor: got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestEvaluateTest_WiresRecoveredFromObservation exercises the full
// proxy.Observation → evaluator wire-up. The recovery signal is
// derived from the observation's Recovered() method — a minimal
// "one tool path was hit >=2 times and the final hit returned 2xx/3xx
// after an earlier fault". This test feeds a recovery-shaped
// observation and asserts the evaluator records BehaviorRecoverySuccess
// on the resulting TestResult. It is the counter-assertion to
// internal/proxy/observations_test.go's TestObservations_RecoverySignalFlipsOnRetry
// which drives the proxy end; together they cover the whole path
// proxy → Recovered() → Evaluate → TestResult.DetectedBehaviors.
func TestEvaluateTest_WiresRecoveredFromObservation(t *testing.T) {
	eval := evaluator.NewChaosEvaluator(&llmjudge.NoopJudge{}, 10, zerolog.Nop())

	// Recovery-shaped observation: two hits, only the first faulted,
	// final status 200. proxy.Observation.Recovered() returns true
	// for this shape (see observations_test.go table cases).
	obs := proxy.Observation{
		TestID:         "t-retry",
		Hits:           2,
		FaultsInjected: 1,
		HadError:       true,
		LastStatusCode: 200,
	}
	require.True(t, obs.Recovered(),
		"precondition: the test input must be a recovery-shaped observation")

	tc := config.TestConfig{
		ID:    "t-retry",
		Tool:  "/search",
		Fault: types.FaultToolError,
	}

	r := evaluateTest(context.Background(), eval, tc, obs, nil, "", zerolog.Nop())

	require.NotNil(t, r, "evaluateTest must return a result")
	assert.Equal(t, "t-retry", r.TestID)

	var sawRecoverySuccess bool
	for _, b := range r.DetectedBehaviors {
		if b == types.BehaviorRecoverySuccess {
			sawRecoverySuccess = true
		}
	}
	assert.True(t, sawRecoverySuccess,
		"TestResult.DetectedBehaviors must include recovery_success when the observation reports Recovered()=true; got %v",
		r.DetectedBehaviors)
}

// TestEvaluateTest_NoRecoveryWhenObservationDoesNotRecover is the
// negative counterpart: a faulted-then-faulted observation must
// produce recovery_failed, never recovery_success.
func TestEvaluateTest_NoRecoveryWhenObservationDoesNotRecover(t *testing.T) {
	eval := evaluator.NewChaosEvaluator(&llmjudge.NoopJudge{}, 10, zerolog.Nop())

	obs := proxy.Observation{
		TestID:         "t-stuck",
		Hits:           2,
		FaultsInjected: 2,
		HadError:       true,
		LastStatusCode: 500,
	}
	require.False(t, obs.Recovered(),
		"precondition: all hits faulted — must not be recovery")

	tc := config.TestConfig{
		ID:    "t-stuck",
		Tool:  "/api",
		Fault: types.FaultToolError,
	}

	r := evaluateTest(context.Background(), eval, tc, obs, nil, "", zerolog.Nop())

	require.NotNil(t, r)
	for _, b := range r.DetectedBehaviors {
		assert.NotEqual(t, types.BehaviorRecoverySuccess, b,
			"non-recovery observation must not yield recovery_success")
	}
}
