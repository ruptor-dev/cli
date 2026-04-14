package rules

import "github.com/faultforge/faultforge/pkg/types"

// CrashDetector detects crash behavior based on HTTP status codes.
type CrashDetector struct{}

// Detect returns BehaviorCrash if the status code indicates a server error (>= 500).
func (d *CrashDetector) Detect(input DetectionInput) []types.DetectedBehavior {
	if input.StatusCode >= 500 {
		return []types.DetectedBehavior{types.BehaviorCrash}
	}
	return nil
}
