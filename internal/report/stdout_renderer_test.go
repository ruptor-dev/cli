package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ruptor-dev/cli/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleChaosReport() *types.ReliabilityReport {
	return &types.ReliabilityReport{
		AgentName:  "test-agent",
		RunAt:      time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
		TotalTests: 3,
		Passed:     2,
		Failed:     1,
		Score:      67,
		Results: []types.TestResult{
			{
				TestID:     "timeout_on_search",
				FaultType:  types.FaultToolTimeout,
				Tool:       "/search",
				Passed:     true,
				DurationMs: 150,
			},
			{
				TestID:            "bad_json_on_lookup",
				FaultType:         types.FaultInvalidJSON,
				Tool:              "/lookup",
				Passed:            false,
				DetectedBehaviors: []types.DetectedBehavior{types.BehaviorCrash, types.BehaviorRecoveryFailed},
				LLMJudgeVerdict:   "FAIL",
				LLMJudgeReason:    "Agent did not handle malformed response",
				DurationMs:        200,
			},
			{
				TestID:     "error_on_submit",
				FaultType:  types.FaultToolError,
				Tool:       "/submit",
				Passed:     true,
				DurationMs: 100,
				Error:      "connection reset",
			},
		},
	}
}

func sampleSimulateReport() *types.ConversationReport {
	return &types.ConversationReport{
		AgentName:   "test-agent",
		RunAt:       time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
		TotalSims:   2,
		GoalReached: 1,
		AvgScore:    72.5,
		Results: []types.SimulationResult{
			{
				SimulationID: "sim_booking",
				Persona:      "impatient-user",
				Goal:         "book a flight",
				GoalReached:  true,
				TurnCount:    5,
				MaxTurns:     10,
				QualityScore: 85,
				DurationMs:   3000,
			},
			{
				SimulationID: "sim_refund",
				Persona:      "confused-user",
				Goal:         "get a refund",
				GoalReached:  false,
				TurnCount:    10,
				MaxTurns:     10,
				QualityScore: 60,
				Issues:       []string{"loop detected", "no escalation"},
				DurationMs:   5000,
			},
		},
	}
}

func TestStdoutRenderer_RenderChaos(t *testing.T) {
	tests := []struct {
		name     string
		report   *types.ReliabilityReport
		contains []string
	}{
		{
			name:   "renders all key fields",
			report: sampleChaosReport(),
			contains: []string{
				"Ruptor Reliability Report",
				"test-agent",
				"Score: 67%",
				"Tests: 3",
				"Passed: 2",
				"Failed: 1",
				"[PASS] timeout_on_search",
				"[FAIL] bad_json_on_lookup",
				"tool_timeout",
				"/search",
				"150ms",
				"200ms",
				"Behaviors: crash, recovery_failed",
				"Judge: FAIL",
				"Agent did not handle malformed response",
				"Error: connection reset",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			r := &StdoutRenderer{Writer: &buf}

			err := r.RenderChaos(tc.report)
			require.NoError(t, err)

			output := buf.String()
			for _, s := range tc.contains {
				assert.Contains(t, output, s, "output should contain %q", s)
			}
		})
	}
}

func TestStdoutRenderer_RenderSimulate(t *testing.T) {
	var buf bytes.Buffer
	r := &StdoutRenderer{Writer: &buf}

	err := r.RenderSimulate(sampleSimulateReport())
	require.NoError(t, err)

	output := buf.String()
	expected := []string{
		"Ruptor Simulation Report",
		"test-agent",
		"Simulations: 2",
		"Goal Reached: 1",
		"Avg Score: 72.5",
		"[REACHED] sim_booking",
		"[MISSED] sim_refund",
		"impatient-user",
		"confused-user",
		"book a flight",
		"get a refund",
		"Turns: 5/10",
		"Score: 85",
		"Issues: loop detected, no escalation",
	}
	for _, s := range expected {
		assert.Contains(t, output, s, "output should contain %q", s)
	}
}

func TestJSONRenderer_RenderChaos(t *testing.T) {
	dir := t.TempDir()
	r := &JSONRenderer{Path: dir}

	report := sampleChaosReport()
	err := r.RenderChaos(report)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "chaos_report.json"))
	require.NoError(t, err)

	var got types.ReliabilityReport
	err = json.Unmarshal(data, &got)
	require.NoError(t, err)

	assert.Equal(t, "test-agent", got.AgentName)
	assert.Equal(t, 3, got.TotalTests)
	assert.Equal(t, 2, got.Passed)
	assert.Equal(t, 1, got.Failed)
	assert.Equal(t, 67, got.Score)
	assert.Len(t, got.Results, 3)
	assert.Equal(t, "timeout_on_search", got.Results[0].TestID)
	assert.True(t, got.Results[0].Passed)
	assert.False(t, got.Results[1].Passed)
}

func TestJSONRenderer_RenderSimulate(t *testing.T) {
	dir := t.TempDir()
	r := &JSONRenderer{Path: dir}

	err := r.RenderSimulate(sampleSimulateReport())
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "simulate_report.json"))
	require.NoError(t, err)

	var got types.ConversationReport
	err = json.Unmarshal(data, &got)
	require.NoError(t, err)

	assert.Equal(t, "test-agent", got.AgentName)
	assert.Equal(t, 2, got.TotalSims)
	assert.Equal(t, 1, got.GoalReached)
	assert.InDelta(t, 72.5, got.AvgScore, 0.01)
	assert.Len(t, got.Results, 2)
}

func TestJSONRenderer_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "output")
	r := &JSONRenderer{Path: dir}

	err := r.RenderChaos(sampleChaosReport())
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, "chaos_report.json"))
	assert.NoError(t, err)
}

func TestNewRendererFromFormat(t *testing.T) {
	tests := []struct {
		format   string
		wantType string
	}{
		{"stdout", "*report.StdoutRenderer"},
		{"json", "*report.JSONRenderer"},
		{"html", "*report.HTMLRenderer"},
		{"both", "*report.MultiRenderer"},
		{"unknown", "*report.StdoutRenderer"},
		{"", "*report.StdoutRenderer"},
	}

	for _, tc := range tests {
		t.Run(tc.format, func(t *testing.T) {
			r := NewRendererFromFormat(tc.format, "/tmp/test")
			got := fmt.Sprintf("%T", r)
			assert.Equal(t, tc.wantType, got)
		})
	}
}

func TestMultiRenderer(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	multi := &MultiRenderer{
		renderers: []Renderer{
			&StdoutRenderer{Writer: &buf1},
			&StdoutRenderer{Writer: &buf2},
		},
	}

	err := multi.RenderChaos(sampleChaosReport())
	require.NoError(t, err)
	assert.Contains(t, buf1.String(), "test-agent")
	assert.Contains(t, buf2.String(), "test-agent")

	buf1.Reset()
	buf2.Reset()

	err = multi.RenderSimulate(sampleSimulateReport())
	require.NoError(t, err)
	assert.Contains(t, buf1.String(), "test-agent")
	assert.Contains(t, buf2.String(), "test-agent")
}

func TestHTMLRenderer_RenderChaos_Fallback(t *testing.T) {
	dir := t.TempDir()
	r := &HTMLRenderer{Path: dir}

	err := r.RenderChaos(sampleChaosReport())
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "chaos_report.html"))
	require.NoError(t, err)

	html := string(data)
	assert.Contains(t, html, "test-agent")
	assert.Contains(t, html, "67%")
	assert.Contains(t, html, "timeout_on_search")
	assert.Contains(t, html, "FAIL")
}

func TestHTMLRenderer_RenderSimulate_Fallback(t *testing.T) {
	dir := t.TempDir()
	r := &HTMLRenderer{Path: dir}

	err := r.RenderSimulate(sampleSimulateReport())
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "simulate_report.html"))
	require.NoError(t, err)

	html := string(data)
	assert.Contains(t, html, "test-agent")
	assert.Contains(t, html, "sim_booking")
	assert.Contains(t, html, "REACHED")
	assert.Contains(t, html, "MISSED")
}
