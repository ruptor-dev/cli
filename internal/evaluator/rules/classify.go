package rules

import (
	"fmt"

	"github.com/ruptor-dev/cli/pkg/types"
)

// Obs bundles the per-test proxy signals consumed by ClassifyExperiment.
// It is the rule-layer view of internal/proxy.Observation — a flat struct
// so the evaluator package does not take an import on internal/proxy.
type Obs struct {
	// Hits is the number of requests the agent made against the tool
	// path under test (fault-injected + passthrough).
	Hits int
	// LastStatusCode is the HTTP status returned on the most recent hit.
	LastStatusCode int
	// HadError is true when at least one hit was a fault response
	// (HTTP >= 400) or the recovery signal fired.
	HadError bool
	// Recovered is true when the agent recovered from an injected
	// fault: a later successful hit followed an earlier faulted one.
	Recovered bool
}

// Verdict is the result of classifying an experiment: the set of
// detected behaviors, a pass/fail decision, and a human-readable reason
// populated only on failure.
type Verdict struct {
	Behaviors []types.DetectedBehavior
	Passed    bool
	Reason    string
}

// ClassifyExperiment applies the deterministic rule set to a single
// experiment's observation and returns a Verdict. Callers supply the
// configured fault type, the maximum-iterations threshold used for
// loop detection on non-error faults, and the observation. The fourth
// parameter is reserved for additional context (e.g. agent transcript)
// and is currently unused.
//
// The classification rules:
//
//   - Zero hits: the agent never reached the proxy. No behaviors, PASS.
//   - Crash: a server-error (>=500) response that is not the fault's
//     expected shape. Error faults (tool_error, llm_error) treat any
//     5xx as expected. Timeout faults (tool_timeout, llm_timeout) treat
//     only 504 as expected; other 5xx is a crash. All other faults
//     treat any 5xx as a crash.
//   - Infinite loop: hit count >= threshold. For error faults with an
//     observed error the threshold is the retry bound (2); otherwise
//     the threshold is maxIterations.
//   - Recovery: when HadError is true, Recovered=true yields
//     BehaviorRecoverySuccess and Recovered=false yields
//     BehaviorRecoveryFailed.
//
// An experiment PASSES when no crash was observed and either no loop
// fired or the agent recovered from its fault. Otherwise it FAILS and
// Reason is set to a short explanation of the decision.
func ClassifyExperiment(
	faultType types.FaultType,
	maxIterations int,
	obs Obs,
	_ interface{},
) Verdict {
	var behaviors []types.DetectedBehavior

	// Zero hits — agent did not exercise the tool path. Nothing to
	// classify; upstream reporters flag this separately.
	if obs.Hits == 0 {
		return Verdict{Behaviors: behaviors, Passed: true}
	}

	// Crash detection: fault-type-aware. For error faults 5xx is the
	// injected response, not a crash. For timeout faults 504 is the
	// injected response. All other faults treat any 5xx as a crash.
	crashed := false
	if obs.LastStatusCode >= 500 {
		switch faultType {
		case types.FaultToolError, types.FaultLLMError:
			// 5xx is the expected injected response — not a crash.
		case types.FaultToolTimeout, types.FaultLLMTimeout:
			if obs.LastStatusCode != 504 {
				crashed = true
			}
		default:
			crashed = true
		}
	}
	if crashed {
		behaviors = append(behaviors, types.BehaviorCrash)
	}

	// Loop detection: tighter bound for error faults with an observed
	// error (retry threshold 2), otherwise the configured max iterations.
	loopThreshold := maxIterations
	if obs.HadError {
		switch faultType {
		case types.FaultToolError, types.FaultLLMError:
			loopThreshold = 2
		}
	}
	looped := obs.Hits >= loopThreshold
	if looped {
		behaviors = append(behaviors, types.BehaviorInfiniteLoop)
	}

	// Recovery: only meaningful when an error was observed.
	if obs.HadError {
		if obs.Recovered {
			behaviors = append(behaviors, types.BehaviorRecoverySuccess)
		} else {
			behaviors = append(behaviors, types.BehaviorRecoveryFailed)
		}
	}

	// Pass/fail: crash is always a failure. A loop is a failure unless
	// the agent recovered.
	passed := !crashed && (!looped || obs.Recovered)

	reason := ""
	if !passed {
		switch {
		case crashed:
			reason = fmt.Sprintf(
				"crash: status %d is not expected for fault %s",
				obs.LastStatusCode, faultType,
			)
		case looped:
			reason = fmt.Sprintf(
				"infinite loop: %d hits exceeded threshold %d",
				obs.Hits, loopThreshold,
			)
		default:
			reason = "experiment failed"
		}
	}

	return Verdict{
		Behaviors: behaviors,
		Passed:    passed,
		Reason:    reason,
	}
}
