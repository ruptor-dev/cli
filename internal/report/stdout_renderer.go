package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/ruptor-dev/cli/pkg/types"
)

// StdoutRenderer pretty-prints reports to an io.Writer.
type StdoutRenderer struct {
	Writer io.Writer
}

// RenderChaos renders a reliability report in human-readable format.
func (s *StdoutRenderer) RenderChaos(report *types.ReliabilityReport) error {
	w := s.Writer

	fmt.Fprintf(w, "\n%s Ruptor Reliability Report %s\n", strings.Repeat("\u2550", 3), strings.Repeat("\u2550", 3))
	fmt.Fprintf(w, "Agent: %s  |  Run: %s\n", report.AgentName, report.RunAt.Format("2006-01-02 15:04:05"))
	// Score is persisted in 0.0–1.0; user-facing display is whole-percent.
	fmt.Fprintf(w, "Tests: %d  |  Passed: %d  |  Failed: %d  |  Score: %.0f%%\n", report.TotalTests, report.Passed, report.Failed, report.Score*100)
	fmt.Fprintf(w, "\n%s Results %s\n", strings.Repeat("\u2500", 3), strings.Repeat("\u2500", 3))

	for _, r := range report.Results {
		marker := "[PASS]"
		if !r.Passed {
			marker = "[FAIL]"
		}
		fmt.Fprintf(w, "%s %s (%s on %s) \u2014 %dms\n", marker, r.TestID, r.FaultType, r.Tool, r.DurationMs)

		if len(r.DetectedBehaviors) > 0 {
			behaviors := make([]string, len(r.DetectedBehaviors))
			for i, b := range r.DetectedBehaviors {
				behaviors[i] = string(b)
			}
			fmt.Fprintf(w, "       Behaviors: %s\n", strings.Join(behaviors, ", "))
		}

		if r.LLMJudgeVerdict != "" {
			fmt.Fprintf(w, "       Judge: %s", r.LLMJudgeVerdict)
			if r.LLMJudgeReason != "" {
				fmt.Fprintf(w, " \u2014 %s", r.LLMJudgeReason)
			}
			fmt.Fprintln(w)
		}

		if r.Error != "" {
			fmt.Fprintf(w, "       Error: %s\n", r.Error)
		}
	}

	fmt.Fprintln(w)
	return nil
}

// RenderSimulate renders a conversation simulation report in human-readable format.
func (s *StdoutRenderer) RenderSimulate(report *types.ConversationReport) error {
	w := s.Writer

	fmt.Fprintf(w, "\n%s Ruptor Simulation Report %s\n", strings.Repeat("\u2550", 3), strings.Repeat("\u2550", 3))
	fmt.Fprintf(w, "Agent: %s  |  Run: %s\n", report.AgentName, report.RunAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(w, "Simulations: %d  |  Goal Reached: %d  |  Avg Score: %.1f\n", report.TotalSims, report.GoalReached, report.AvgScore)
	fmt.Fprintf(w, "\n%s Results %s\n", strings.Repeat("\u2500", 3), strings.Repeat("\u2500", 3))

	for _, r := range report.Results {
		status := "[REACHED]"
		if !r.GoalReached {
			status = "[MISSED]"
		}
		fmt.Fprintf(w, "%s %s (persona: %s) \u2014 %dms\n", status, r.SimulationID, r.Persona, r.DurationMs)
		fmt.Fprintf(w, "       Goal: %s\n", r.Goal)
		fmt.Fprintf(w, "       Turns: %d/%d  |  Score: %d\n", r.TurnCount, r.MaxTurns, r.QualityScore)

		if len(r.Issues) > 0 {
			fmt.Fprintf(w, "       Issues: %s\n", strings.Join(r.Issues, ", "))
		}
	}

	fmt.Fprintln(w)
	return nil
}
