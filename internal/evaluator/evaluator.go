package evaluator

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	"github.com/ruptor-dev/cli/internal/evaluator/llmjudge"
	"github.com/ruptor-dev/cli/internal/evaluator/rules"
	"github.com/ruptor-dev/cli/pkg/types"
)

// ChaosEvaluator evaluates agent behavior during chaos testing using
// rule-based detectors and an optional LLM judge.
type ChaosEvaluator struct {
	judge     llmjudge.Judge
	detectors []rules.Detector
	logger    zerolog.Logger
}

// NewChaosEvaluator creates a new ChaosEvaluator with the given judge and
// configuration. The default detector set (loop, crash, recovery) is
// installed; use NewChaosEvaluatorWithDetectors to supply a custom set.
func NewChaosEvaluator(judge llmjudge.Judge, maxIterations int, logger zerolog.Logger) *ChaosEvaluator {
	return NewChaosEvaluatorWithDetectors(judge, logger, []rules.Detector{
		&rules.LoopDetector{MaxIterations: maxIterations},
		&rules.CrashDetector{},
		&rules.RecoveryDetector{},
	})
}

// NewChaosEvaluatorWithDetectors creates a ChaosEvaluator with an explicit
// detector set. Useful for tests and for callers that want to add custom
// detectors (e.g. repetition, tone).
func NewChaosEvaluatorWithDetectors(judge llmjudge.Judge, logger zerolog.Logger, detectors []rules.Detector) *ChaosEvaluator {
	return &ChaosEvaluator{
		judge:     judge,
		detectors: detectors,
		logger:    logger,
	}
}

// Evaluate runs all detectors and the LLM judge, assembling a TestResult.
func (e *ChaosEvaluator) Evaluate(
	ctx context.Context,
	testID string,
	faultType types.FaultType,
	tool string,
	statusCode int,
	iterations int,
	hadError, recovered bool,
	prompt, agentBehavior string,
) (*types.TestResult, error) {
	e.logger.Info().
		Str("test_id", testID).
		Str("fault_type", string(faultType)).
		Str("tool", tool).
		Msg("evaluating chaos test")

	behaviors := e.runDetectors(iterations, statusCode, hadError, recovered)

	verdict, reason, err := e.runJudge(ctx, prompt, agentBehavior)
	if err != nil {
		return nil, err
	}

	passed := !containsCrash(behaviors) && (verdict == "PASS" || verdict == "SKIPPED")

	result := &types.TestResult{
		TestID:            testID,
		FaultType:         faultType,
		Tool:              tool,
		Passed:            passed,
		DetectedBehaviors: behaviors,
		LLMJudgeVerdict:   verdict,
		LLMJudgeReason:    reason,
	}

	e.logger.Info().
		Str("test_id", testID).
		Bool("passed", passed).
		Str("verdict", verdict).
		Int("behaviors", len(behaviors)).
		Msg("chaos evaluation complete")

	return result, nil
}

func (e *ChaosEvaluator) runDetectors(iterations, statusCode int, hadError, recovered bool) []types.DetectedBehavior {
	input := rules.DetectionInput{
		Iterations: iterations,
		StatusCode: statusCode,
		HadError:   hadError,
		Recovered:  recovered,
	}
	var behaviors []types.DetectedBehavior
	for _, d := range e.detectors {
		if b := d.Detect(input); b != nil {
			behaviors = append(behaviors, b...)
		}
	}
	return behaviors
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

func containsCrash(behaviors []types.DetectedBehavior) bool {
	for _, b := range behaviors {
		if b == types.BehaviorCrash {
			return true
		}
	}
	return false
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
