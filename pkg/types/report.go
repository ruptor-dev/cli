package types

import "time"

type ReliabilityReport struct {
	AgentName  string
	RunAt      time.Time
	TotalTests int
	Passed     int
	Failed     int
	Score      int
	Results    []TestResult
}

type ConversationReport struct {
	AgentName   string
	RunAt       time.Time
	TotalSims   int
	GoalReached int
	AvgScore    float64
	Results     []SimulationResult
}
