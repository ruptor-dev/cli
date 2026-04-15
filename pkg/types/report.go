package types

import "time"

// ReportSchemaVersion is the version of the report JSON schema. See ADR-007:
// the platform accepts ReportSchemaVersion values "1.0", "1.1", "1.2"
// simultaneously, so bumping the minor is safe; bumping the major is a
// migration event.
const ReportSchemaVersion = "1.0"

type ReliabilityReport struct {
	SchemaVersion string    `json:"schema_version"`
	RuptorVersion string    `json:"ruptor_version"`
	AgentName     string    `json:"agent_name"`
	RunAt         time.Time `json:"run_at"`
	TotalTests    int       `json:"total_tests"`
	Passed        int       `json:"passed"`
	Failed        int       `json:"failed"`
	// Score is the Robustness Score in the canonical 0.0–1.0 range
	// (1.0 = every test passed). Renderers multiply by 100 to show
	// "67%". Persisted in this form so platform-side analytics never
	// have to guess between "67" (percent) and "0.67" (fraction).
	Score       float64      `json:"score"`
	DurationMs  int64        `json:"duration_ms"`
	Results     []TestResult `json:"results"`
}

type ConversationReport struct {
	SchemaVersion string             `json:"schema_version"`
	RuptorVersion string             `json:"ruptor_version"`
	AgentName     string             `json:"agent_name"`
	RunAt         time.Time          `json:"run_at"`
	TotalSims     int                `json:"total_sims"`
	GoalReached   int                `json:"goal_reached"`
	AvgScore      float64            `json:"avg_score"`
	Results       []SimulationResult `json:"results"`
}
