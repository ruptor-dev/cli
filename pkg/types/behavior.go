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
