package simulate

import (
	"context"
	"fmt"
	"strings"
)

// GoalChecker decides whether a conversation has reached its success
// criteria after a given agent response. It replaces the old substring
// match in checkGoalReached: that heuristic returned false whenever the
// agent paraphrased the criteria, which is the common case.
type GoalChecker interface {
	Check(ctx context.Context, agentResponse, goal, successCriteria string) (bool, error)
}

// LLMGoalChecker asks the LLM whether the most recent agent response
// satisfies the success criteria. One extra LLM call per turn — cheap
// relative to the user + agent calls that surround it.
type LLMGoalChecker struct {
	Client LLMClient
}

const goalCheckSystem = `You are a strict goal-completion evaluator. Given a user goal, its success criteria, and the most recent message from the assistant under test, reply with exactly one word: YES if the assistant has clearly fulfilled the success criteria in this message, or NO otherwise. Do not explain.`

// Check returns true when the LLM answers YES (case-insensitive). Any
// other answer — including ambiguity or errors from the model — is
// treated as "not reached". A nil Client or empty successCriteria
// always returns false.
func (c *LLMGoalChecker) Check(ctx context.Context, agentResponse, goal, successCriteria string) (bool, error) {
	if c.Client == nil || successCriteria == "" {
		return false, nil
	}

	user := fmt.Sprintf("Goal: %s\nSuccess criteria: %s\nAssistant message:\n%s", goal, successCriteria, agentResponse)
	msgs := []map[string]string{
		{"role": "system", "content": goalCheckSystem},
		{"role": "user", "content": user},
	}
	resp, err := c.Client.Complete(ctx, msgs, "")
	if err != nil {
		return false, fmt.Errorf("simulate: goal check: %w", err)
	}
	return strings.EqualFold(strings.TrimSpace(resp), "YES"), nil
}

// NoopGoalChecker always reports the goal as not reached. Used when no
// LLM client is available.
type NoopGoalChecker struct{}

func (NoopGoalChecker) Check(_ context.Context, _, _, _ string) (bool, error) { return false, nil }
