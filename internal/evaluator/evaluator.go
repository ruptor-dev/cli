package evaluator

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	"github.com/ruptor-dev/cli/internal/evaluator/llmjudge"
	"github.com/ruptor-dev/cli/internal/evaluator/rules"
	"github.com/ruptor-dev/cli/pkg/types"
)

// ChaosEvaluator classifies one experiment and overlays an LLM judge
// verdict on top. The rule-based classification itself lives in
// internal/evaluator/rules/classify.go so the TUI can call it without
// pulling in judge / network code.
type ChaosEvaluator struct {
	judge         llmjudge.Judge
	maxIterations int
	logger        zerolog.Logger
}

// NewChaosEvaluator creates a new ChaosEvaluator with the given judge
// and a per-experiment iteration cap (passed through to the
// ClassifyExperiment loop-detector rule).
func NewChaosEvaluator(judge llmjudge.Judge, maxIterations int, logger zerolog.Logger) *ChaosEvaluator {
	return &ChaosEvaluator{
		judge:         judge,
		maxIterations: maxIterations,
		logger:        logger,
	}
}

// Evaluate classifies one experiment by delegating to
// rules.ClassifyExperiment (the single source of truth for
// TUI + final report) and overlaying the LLM judge verdict on top.
//
// agentErr is the exit error from the agent child process (nil if
// the runner was not used or the agent exited cleanly).
func (e *ChaosEvaluator) Evaluate(
	ctx context.Context,
	testID string,
	faultType types.FaultType,
	tool string,
	statusCode int,
	iterations int,
	hadError, recovered bool,
	agentErr error,
	prompt, agentBehavior string,
) (*types.TestResult, error) {
	e.logger.Info().
		Str("test_id", testID).
		Str("fault_type", string(faultType)).
		Str("tool", tool).
		Msg("evaluating chaos test")

	v := rules.ClassifyExperiment(faultType, e.maxIterations, rules.Obs{
		Hits:           iterations,
		LastStatusCode: statusCode,
		HadError:       hadError,
		Recovered:      recovered,
	}, agentErr)

	verdict, reason, err := e.runJudge(ctx, prompt, agentBehavior)
	if err != nil {
		return nil, err
	}

	// LLM judge can only downgrade: if the rule engine said PASS and
	// the judge returned FAIL, the experiment fails. The judge cannot
	// promote a rule-based fail to a pass.
	passed := v.Passed && (verdict == "PASS" || verdict == "SKIPPED")

	errStr := ""
	if !passed && v.Reason != "" {
		errStr = v.Reason
	}

	result := &types.TestResult{
		TestID:            testID,
		FaultType:         faultType,
		Tool:              tool,
		Passed:            passed,
		DetectedBehaviors: v.Behaviors,
		LLMJudgeVerdict:   verdict,
		LLMJudgeReason:    reason,
		Error:             errStr,
	}

	e.logger.Info().
		Str("test_id", testID).
		Bool("passed", passed).
		Str("verdict", verdict).
		Int("behaviors", len(v.Behaviors)).
		Msg("chaos evaluation complete")

	return result, nil
}

func (e *ChaosEvaluator) runJudge(ctx context.Context, prompt, agentBehavior string) (verdict, reason string, err error) {
	if prompt == "" {
		return "SKIPPED", "", nil
	}
	v, r, err := e.judge.EvaluateChaos(ctx, prompt, agentBehavior)
	if err != nil {
		return "", "", fmt.Errorf("evaluator: running LLM judge: %w", err)
	}
	return v, r, nil
}

// SimulateEvaluator evaluates conversation simulations using an LLM judge.
type SimulateEvaluator struct {
	judge  llmjudge.Judge
	logger zerolog.Logger
}

// NewSimulateEvaluator creates a new SimulateEvaluator with the given judge.
func NewSimulateEvaluator(judge llmjudge.Judge, logger zerolog.Logger) *SimulateEvaluator {
	return &SimulateEvaluator{
		judge:  judge,
		logger: logger,
	}
}

// Evaluate runs the LLM judge on a conversation simulation and assembles
// a SimulationResult.
func (e *SimulateEvaluator) Evaluate(
	ctx context.Context,
	simID, persona, goal string,
	history *types.ConversationHistory,
	prompt string,
) (*types.SimulationResult, error) {
	e.logger.Info().
		Str("sim_id", simID).
		Str("persona", persona).
		Int("turns", len(history.Turns)).
		Msg("evaluating conversation simulation")

	score, issues, err := e.judge.EvaluateConversation(ctx, prompt, history)
	if err != nil {
		return nil, fmt.Errorf("evaluator: running conversation judge: %w", err)
	}

	result := &types.SimulationResult{
		SimulationID: simID,
		Persona:      persona,
		Goal:         goal,
		TurnCount:    len(history.Turns),
		QualityScore: score,
		Issues:       issues,
	}

	e.logger.Info().
		Str("sim_id", simID).
		Int("score", score).
		Int("issues", len(issues)).
		Msg("conversation evaluation complete")

	return result, nil
}
