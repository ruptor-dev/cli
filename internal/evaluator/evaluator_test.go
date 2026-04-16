package evaluator_test

import (
	"context"
	"testing"

	"github.com/ruptor-dev/cli/internal/evaluator"
	"github.com/ruptor-dev/cli/internal/evaluator/llmjudge"
	"github.com/ruptor-dev/cli/internal/ui"
	"github.com/ruptor-dev/cli/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChaosEvaluator_Evaluate(t *testing.T) {
	logger := ui.SilentLogger()
	judge := &llmjudge.NoopJudge{}

	tests := []struct {
		name          string
		testID        string
		faultType     types.FaultType
		tool          string
		statusCode    int
		iterations    int
		hadError      bool
		recovered     bool
		prompt        string
		agentBehavior string
		wantPassed    bool
		wantVerdict   string
		wantBehaviors []types.DetectedBehavior
	}{
		{
			name:          "clean run passes",
			testID:        "test-1",
			faultType:     types.FaultToolError,
			tool:          "search",
			statusCode:    200,
			iterations:    3,
			hadError:      false,
			recovered:     false,
			prompt:        "",
			agentBehavior: "",
			wantPassed:    true,
			wantVerdict:   "SKIPPED",
			wantBehaviors: nil,
		},
		{
			// Real upstream crash (invalid_json fault did NOT inject
			// this 500 — it's a genuine server error). CrashDetector
			// fires, agent did not recover → FAIL.
			name:          "real upstream crash fails",
			testID:        "test-2",
			faultType:     types.FaultInvalidJSON,
			tool:          "search",
			statusCode:    500,
			iterations:    1,
			hadError:      true,
			recovered:     false,
			prompt:        "",
			agentBehavior: "",
			wantPassed:    false,
			wantVerdict:   "SKIPPED",
			wantBehaviors: []types.DetectedBehavior{types.BehaviorCrash, types.BehaviorRecoveryFailed},
		},
		{
			// llm_error fault's 5xx is the INJECTED response; agent
			// handled it with one call and exited cleanly. The new
			// heuristic must treat this as PASS (prior behaviour
			// marked it FAIL because CrashDetector fired on any 5xx).
			name:          "graceful llm_error handling passes",
			testID:        "test-2b",
			faultType:     types.FaultLLMError,
			tool:          "/llm/complete",
			statusCode:    503,
			iterations:    1,
			hadError:      false,
			recovered:     false,
			prompt:        "",
			agentBehavior: "",
			wantPassed:    true,
			wantVerdict:   "SKIPPED",
			wantBehaviors: nil,
		},
		{
			// Same llm_error but agent retried twice — that's the
			// retry-loop anti-pattern the stricter threshold catches.
			name:          "llm_error retry loop fails",
			testID:        "test-2c",
			faultType:     types.FaultLLMError,
			tool:          "/llm/complete",
			statusCode:    503,
			iterations:    2,
			hadError:      false,
			recovered:     false,
			prompt:        "",
			agentBehavior: "",
			wantPassed:    false,
			wantVerdict:   "SKIPPED",
			wantBehaviors: []types.DetectedBehavior{types.BehaviorInfiniteLoop},
		},
		{
			name:          "loop detected with recovery",
			testID:        "test-3",
			faultType:     types.FaultSlowResponse,
			tool:          "compute",
			statusCode:    200,
			iterations:    10,
			hadError:      true,
			recovered:     true,
			prompt:        "",
			agentBehavior: "",
			wantPassed:    true,
			wantVerdict:   "SKIPPED",
			wantBehaviors: []types.DetectedBehavior{types.BehaviorInfiniteLoop, types.BehaviorRecoverySuccess},
		},
		{
			name:          "crash with loop fails",
			testID:        "test-4",
			faultType:     types.FaultToolTimeout,
			tool:          "api",
			statusCode:    502,
			iterations:    15,
			hadError:      true,
			recovered:     false,
			prompt:        "",
			agentBehavior: "",
			wantPassed:    false,
			wantVerdict:   "SKIPPED",
			wantBehaviors: []types.DetectedBehavior{types.BehaviorCrash, types.BehaviorInfiniteLoop, types.BehaviorRecoveryFailed},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := evaluator.NewChaosEvaluator(judge, 10, logger)
			result, err := e.Evaluate(
				context.Background(),
				tt.testID,
				tt.faultType,
				tt.tool,
				tt.statusCode,
				tt.iterations,
				tt.hadError,
				tt.recovered,
				nil, // agentErr — no runner in unit tests
				tt.prompt,
				tt.agentBehavior,
			)

			require.NoError(t, err)
			assert.Equal(t, tt.testID, result.TestID)
			assert.Equal(t, tt.faultType, result.FaultType)
			assert.Equal(t, tt.tool, result.Tool)
			assert.Equal(t, tt.wantPassed, result.Passed)
			assert.Equal(t, tt.wantVerdict, result.LLMJudgeVerdict)
			assert.Equal(t, tt.wantBehaviors, result.DetectedBehaviors)
		})
	}
}

func TestSimulateEvaluator_Evaluate(t *testing.T) {
	logger := ui.SilentLogger()
	judge := &llmjudge.NoopJudge{}

	e := evaluator.NewSimulateEvaluator(judge, logger)

	history := &types.ConversationHistory{}
	history.Add("user", "Hello")
	history.Add("assistant", "Hi there!")

	result, err := e.Evaluate(
		context.Background(),
		"sim-1",
		"friendly-user",
		"get help",
		history,
		"",
	)

	require.NoError(t, err)
	assert.Equal(t, "sim-1", result.SimulationID)
	assert.Equal(t, "friendly-user", result.Persona)
	assert.Equal(t, "get help", result.Goal)
	assert.Equal(t, 2, result.TurnCount)
	assert.Equal(t, 0, result.QualityScore)
	assert.Empty(t, result.Issues)
}
