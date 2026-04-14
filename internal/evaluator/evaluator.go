package evaluator

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/faultforge/faultforge/internal/evaluator/llmjudge"
	"github.com/faultforge/faultforge/internal/evaluator/rules"
	"github.com/faultforge/faultforge/pkg/types"
)

// ChaosEvaluator evaluates agent behavior during chaos testing using
// rule-based detectors and an optional LLM judge.
type ChaosEvaluator struct {
	judge     llmjudge.Judge
	detectors []rules.Detector
	logger    *slog.Logger
}

// NewChaosEvaluator creates a new ChaosEvaluator with the given judge and configuration.
// The default detector set (loop, crash, recovery) is installed; use
// NewChaosEvaluatorWithDetectors to supply a custom set.
func NewChaosEvaluator(judge llmjudge.Judge, maxIterations int, logger *slog.Logger) *ChaosEvaluator {
	return NewChaosEvaluatorWithDetectors(judge, logger, []rules.Detector{
		&rules.LoopDetector{MaxIterations: maxIterations},
		&rules.CrashDetector{},
		&rules.RecoveryDetector{},
	})
}

// NewChaosEvaluatorWithDetectors creates a ChaosEvaluator with an explicit
// detector set. Useful for tests and for callers that want to add custom
// detectors (e.g. repetition, tone).
func NewChaosEvaluatorWithDetectors(judge llmjudge.Judge, logger *slog.Logger, detectors []rules.Detector) *ChaosEvaluator {
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
	e.logger.Info("evaluating chaos test",
		slog.String("test_id", testID),
		slog.String("fault_type", string(faultType)),
		slog.String("tool", tool),
	)

	// Run rule-based detectors.
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

	// Run LLM judge if prompt is provided.
	var verdict, reason string
	if prompt != "" {
		var err error
		verdict, reason, err = e.judge.EvaluateChaos(ctx, prompt, agentBehavior)
		if err != nil {
			return nil, fmt.Errorf("evaluator: running LLM judge: %w", err)
		}
	} else {
		verdict = "SKIPPED"
	}

	// Determine pass/fail: no crash behaviors and verdict is PASS or SKIPPED.
	hasCrash := false
	for _, b := range behaviors {
		if b == types.BehaviorCrash {
			hasCrash = true
			break
		}
	}
	passed := !hasCrash && (verdict == "PASS" || verdict == "SKIPPED")

	result := &types.TestResult{
		TestID:            testID,
		FaultType:         faultType,
		Tool:              tool,
		Passed:            passed,
		DetectedBehaviors: behaviors,
		LLMJudgeVerdict:   verdict,
		LLMJudgeReason:    reason,
	}

	e.logger.Info("chaos evaluation complete",
		slog.String("test_id", testID),
		slog.Bool("passed", passed),
		slog.String("verdict", verdict),
		slog.Int("behaviors", len(behaviors)),
	)

	return result, nil
}

// SimulateEvaluator evaluates conversation simulations using an LLM judge.
type SimulateEvaluator struct {
	judge  llmjudge.Judge
	logger *slog.Logger
}

// NewSimulateEvaluator creates a new SimulateEvaluator with the given judge.
func NewSimulateEvaluator(judge llmjudge.Judge, logger *slog.Logger) *SimulateEvaluator {
	return &SimulateEvaluator{
		judge:  judge,
		logger: logger,
	}
}

// Evaluate runs the LLM judge on a conversation simulation and assembles a SimulationResult.
func (e *SimulateEvaluator) Evaluate(
	ctx context.Context,
	simID, persona, goal string,
	history *types.ConversationHistory,
	prompt string,
) (*types.SimulationResult, error) {
	e.logger.Info("evaluating conversation simulation",
		slog.String("sim_id", simID),
		slog.String("persona", persona),
		slog.Int("turns", len(history.Turns)),
	)

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

	e.logger.Info("conversation evaluation complete",
		slog.String("sim_id", simID),
		slog.Int("score", score),
		slog.Int("issues", len(issues)),
	)

	return result, nil
}
