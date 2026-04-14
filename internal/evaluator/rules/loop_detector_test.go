package rules_test

import (
	"testing"

	"github.com/faultforge/faultforge/internal/evaluator/rules"
	"github.com/faultforge/faultforge/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestLoopDetector_Detect(t *testing.T) {
	tests := []struct {
		name          string
		maxIterations int
		iterations    int
		want          []types.DetectedBehavior
	}{
		{
			name:          "below max returns nil",
			maxIterations: 10,
			iterations:    5,
			want:          nil,
		},
		{
			name:          "at max returns infinite loop",
			maxIterations: 10,
			iterations:    10,
			want:          []types.DetectedBehavior{types.BehaviorInfiniteLoop},
		},
		{
			name:          "above max returns infinite loop",
			maxIterations: 10,
			iterations:    15,
			want:          []types.DetectedBehavior{types.BehaviorInfiniteLoop},
		},
		{
			name:          "zero iterations below max",
			maxIterations: 5,
			iterations:    0,
			want:          nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &rules.LoopDetector{MaxIterations: tt.maxIterations}
			got := d.Detect(rules.DetectionInput{Iterations: tt.iterations})
			assert.Equal(t, tt.want, got)
		})
	}
}
