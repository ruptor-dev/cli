package rules

import "github.com/faultforge/faultforge/pkg/types"

// LoopDetector detects infinite loop behavior based on iteration count.
type LoopDetector struct {
	MaxIterations int
}

// Detect returns BehaviorInfiniteLoop if iterations >= MaxIterations.
func (d *LoopDetector) Detect(iterations int) []types.DetectedBehavior {
	if iterations >= d.MaxIterations {
		return []types.DetectedBehavior{types.BehaviorInfiniteLoop}
	}
	return nil
}
