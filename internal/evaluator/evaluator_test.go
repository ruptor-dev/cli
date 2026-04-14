package evaluator_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/ruptor-dev/cli/internal/evaluator"
	"github.com/ruptor-dev/cli/internal/evaluator/llmjudge"
	"github.com/ruptor-dev/cli/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChaosEvaluator_Evaluate(t *testing.T) {
	logger := slog.Default()
	judge := &llmjudge.NoopJudge{}

	tests := []struct {
		name           string
		testID         string
		faultType      types.FaultType
		tool           string
		statusCode     int
		iterations     int
		hadError       bool
		recovered      bool
		prompt         string
		agentBehavior  string
		wantPassed     bool
		wantVerdict    string
		wantBehaviors  []types.DetectedBehavior
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
			name:          "crash detected fails",
			testID:        "test-2",
			faultType:     types.FaultToolError,
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
			wantBehaviors: []types.DetectedBehavior{types.BehaviorInfiniteLoop, types.BehaviorCrash, types.BehaviorRecoveryFailed},
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
	logger := slog.Default()
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
