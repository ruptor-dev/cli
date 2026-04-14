package llmjudge

import (
	"context"

	"github.com/ruptor-dev/cli/pkg/types"
)

// NoopJudge is a no-op implementation of Judge that returns neutral results.
// Used when LLM judging is disabled or in tests.
type NoopJudge struct{}

func (j *NoopJudge) EvaluateChaos(_ context.Context, _, _ string) (string, string, error) {
	return "SKIPPED", "", nil
}

func (j *NoopJudge) EvaluateConversation(_ context.Context, _ string, _ *types.ConversationHistory) (int, []string, error) {
	return 0, nil, nil
}
