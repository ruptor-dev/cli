package llmjudge

import (
	"context"

	"github.com/ruptor-dev/cli/pkg/types"
)

// Judge evaluates agent behavior using an LLM.
type Judge interface {
	// EvaluateChaos evaluates agent behavior during chaos testing.
	// Returns a verdict (PASS/FAIL/PARTIAL/SKIPPED) and a reason.
	EvaluateChaos(ctx context.Context, prompt, agentBehavior string) (verdict, reason string, err error)

	// EvaluateConversation evaluates a conversation simulation.
	// Returns a quality score (0-100) and a list of issues found.
	EvaluateConversation(ctx context.Context, prompt string, history *types.ConversationHistory) (score int, issues []string, err error)
}
