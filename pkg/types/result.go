package types

import "time"

type TestResult struct {
	TestID            string
	FaultType         FaultType
	Tool              string
	Passed            bool
	DetectedBehaviors []DetectedBehavior
	LLMJudgeVerdict   string
	LLMJudgeReason    string
	DurationMs        int64
	Error             string
}

type SimulationResult struct {
	SimulationID string
	Persona      string
	Goal         string
	GoalReached  bool
	TurnCount    int
	MaxTurns     int
	QualityScore int
	Issues       []string
	DurationMs   int64
	History      *ConversationHistory
}

type Turn struct {
	Role      string
	Content   string
	Timestamp time.Time
}

type ConversationHistory struct {
	Turns []Turn
}

func (h *ConversationHistory) Add(role, content string) {
	h.Turns = append(h.Turns, Turn{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	})
}

func (h *ConversationHistory) ToOpenAIMessages() []map[string]string {
	msgs := make([]map[string]string, 0, len(h.Turns))
	for _, t := range h.Turns {
		msgs = append(msgs, map[string]string{
			"role":    t.Role,
			"content": t.Content,
		})
	}
	return msgs
}
