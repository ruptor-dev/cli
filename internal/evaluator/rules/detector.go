package rules

import "github.com/ruptor-dev/cli/pkg/types"

// DetectionInput bundles the signals used by detectors. A single struct keeps
// the Detector interface stable as new detectors add new signal sources.
type DetectionInput struct {
	Iterations int
	StatusCode int
	HadError   bool
	Recovered  bool
}

// Detector inspects a DetectionInput and returns any behaviors it observes.
// Detectors are pure: the same input must always produce the same output.
type Detector interface {
	Detect(input DetectionInput) []types.DetectedBehavior
}
