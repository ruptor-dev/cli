package rules

import "github.com/faultforge/faultforge/pkg/types"

// RecoveryDetector detects recovery behavior based on error and recovery state.
type RecoveryDetector struct{}

// Detect returns recovery-related behaviors based on whether an error occurred
// and whether the agent recovered.
func (d *RecoveryDetector) Detect(input DetectionInput) []types.DetectedBehavior {
	if !input.HadError {
		return nil
	}
	if input.Recovered {
		return []types.DetectedBehavior{types.BehaviorRecoverySuccess}
	}
	return []types.DetectedBehavior{types.BehaviorRecoveryFailed}
}
