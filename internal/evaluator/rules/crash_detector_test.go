package rules_test

import (
	"testing"

	"github.com/ruptor-dev/cli/internal/evaluator/rules"
	"github.com/ruptor-dev/cli/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestCrashDetector_Detect(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       []types.DetectedBehavior
	}{
		{
			name:       "200 OK returns nil",
			statusCode: 200,
			want:       nil,
		},
		{
			name:       "404 Not Found returns nil",
			statusCode: 404,
			want:       nil,
		},
		{
			name:       "500 Internal Server Error returns crash",
			statusCode: 500,
			want:       []types.DetectedBehavior{types.BehaviorCrash},
		},
		{
			name:       "503 Service Unavailable returns crash",
			statusCode: 503,
			want:       []types.DetectedBehavior{types.BehaviorCrash},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &rules.CrashDetector{}
			got := d.Detect(rules.DetectionInput{StatusCode: tt.statusCode})
			assert.Equal(t, tt.want, got)
		})
	}
}
