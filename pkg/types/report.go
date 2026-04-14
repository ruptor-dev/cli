package types

import "time"

// ReportSchemaVersion is the version of the report JSON schema. See ADR-007:
// the platform accepts ReportSchemaVersion values "1.0", "1.1", "1.2"
// simultaneously, so bumping the minor is safe; bumping the major is a
// migration event.
const ReportSchemaVersion = "1.0"

type ReliabilityReport struct {
	SchemaVersion string       `json:"schema_version"`
	RuptorVersion string       `json:"ruptor_version"`
	AgentName     string       `json:"agent_name"`
	RunAt         time.Time    `json:"run_at"`
	TotalTests    int          `json:"total_tests"`
	Passed        int          `json:"passed"`
	Failed        int          `json:"failed"`
	Score         int          `json:"score"`
	Results       []TestResult `json:"results"`
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
