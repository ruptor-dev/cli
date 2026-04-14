package rules

import "github.com/ruptor-dev/cli/pkg/types"

// LoopDetector detects infinite loop behavior based on iteration count.
type LoopDetector struct {
	MaxIterations int
}

// Detect returns BehaviorInfiniteLoop if iterations >= MaxIterations.
func (d *LoopDetector) Detect(input DetectionInput) []types.DetectedBehavior {
	if input.Iterations >= d.MaxIterations {
		return []types.DetectedBehavior{types.BehaviorInfiniteLoop}
	}
	return nil
}
