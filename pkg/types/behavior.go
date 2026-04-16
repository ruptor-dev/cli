package types

type DetectedBehavior string

const (
	BehaviorCrash           DetectedBehavior = "crash"
	BehaviorInfiniteLoop    DetectedBehavior = "infinite_loop"
	BehaviorRecoverySuccess DetectedBehavior = "recovery_success"
	BehaviorRecoveryFailed  DetectedBehavior = "recovery_failed"
	BehaviorHallucination   DetectedBehavior = "hallucination"
	BehaviorFallbackUsed    DetectedBehavior = "fallback_used"
	BehaviorTimeout         DetectedBehavior = "timeout"
)

// Human renders the machine-readable behavior code as a short
// reviewer-friendly phrase. Single source of truth — consumed by both
// the HTML report template and the shared ClassifyExperiment function
// in internal/evaluator/rules.
func (b DetectedBehavior) Human() string {
	switch b {
	case BehaviorCrash:
		return "agent errored on the injected response"
	case BehaviorRecoveryFailed:
		return "no retry or fallback path"
	case BehaviorRecoverySuccess:
		return "agent recovered"
	case BehaviorInfiniteLoop:
		return "stuck in a retry loop"
	case BehaviorHallucination:
		return "hallucinated a result instead of surfacing the failure"
	case BehaviorFallbackUsed:
		return "used a fallback path"
	case BehaviorTimeout:
		return "agent timed out waiting for tool response"
	default:
		return string(b)
	}
}
