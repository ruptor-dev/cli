package report

import (
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/faultforge/faultforge/pkg/types"
)

// HTMLRenderer renders reports as HTML files using Go templates.
type HTMLRenderer struct {
	Path string
}

const fallbackChaosHTML = `<!DOCTYPE html>
<html><head><title>FaultForge Reliability Report</title>
<style>body{font-family:sans-serif;margin:2em}table{border-collapse:collapse;width:100%}th,td{border:1px solid #ccc;padding:8px;text-align:left}.pass{color:green}.fail{color:red}</style>
</head><body>
<h1>FaultForge Reliability Report</h1>
<p><strong>Agent:</strong> {{.AgentName}} | <strong>Run:</strong> {{.RunAt.Format "2006-01-02 15:04:05"}} | <strong>Score:</strong> {{.Score}}%</p>
<p>Tests: {{.TotalTests}} | Passed: {{.Passed}} | Failed: {{.Failed}}</p>
<table><tr><th>Status</th><th>Test ID</th><th>Fault</th><th>Tool</th><th>Duration</th><th>Judge</th></tr>
{{range .Results}}<tr>
<td class="{{if .Passed}}pass{{else}}fail{{end}}">{{if .Passed}}PASS{{else}}FAIL{{end}}</td>
<td>{{.TestID}}</td><td>{{.FaultType}}</td><td>{{.Tool}}</td><td>{{.DurationMs}}ms</td>
<td>{{.LLMJudgeVerdict}}{{if .LLMJudgeReason}} &mdash; {{.LLMJudgeReason}}{{end}}</td>
</tr>{{end}}
</table></body></html>`

const fallbackSimulateHTML = `<!DOCTYPE html>
<html><head><title>FaultForge Simulation Report</title>
<style>body{font-family:sans-serif;margin:2em}table{border-collapse:collapse;width:100%}th,td{border:1px solid #ccc;padding:8px;text-align:left}.reached{color:green}.missed{color:red}</style>
</head><body>
<h1>FaultForge Simulation Report</h1>
<p><strong>Agent:</strong> {{.AgentName}} | <strong>Run:</strong> {{.RunAt.Format "2006-01-02 15:04:05"}} | <strong>Avg Score:</strong> {{printf "%.1f" .AvgScore}}</p>
<p>Simulations: {{.TotalSims}} | Goal Reached: {{.GoalReached}}</p>
<table><tr><th>Status</th><th>Simulation</th><th>Persona</th><th>Goal</th><th>Turns</th><th>Score</th><th>Duration</th></tr>
{{range .Results}}<tr>
<td class="{{if .GoalReached}}reached{{else}}missed{{end}}">{{if .GoalReached}}REACHED{{else}}MISSED{{end}}</td>
<td>{{.SimulationID}}</td><td>{{.Persona}}</td><td>{{.Goal}}</td>
<td>{{.TurnCount}}/{{.MaxTurns}}</td><td>{{.QualityScore}}</td><td>{{.DurationMs}}ms</td>
</tr>{{end}}
</table></body></html>`

// RenderChaos renders a reliability report as HTML.
func (h *HTMLRenderer) RenderChaos(report *types.ReliabilityReport) error {
	tmpl, err := h.loadTemplate("web/chaos_report.html.tmpl", fallbackChaosHTML)
	if err != nil {
		return fmt.Errorf("report: rendering html: %w", err)
	}
	return h.writeHTML("chaos_report.html", tmpl, report)
}

// RenderSimulate renders a conversation report as HTML.
func (h *HTMLRenderer) RenderSimulate(report *types.ConversationReport) error {
	tmpl, err := h.loadTemplate("web/simulate_report.html.tmpl", fallbackSimulateHTML)
	if err != nil {
		return fmt.Errorf("report: rendering html: %w", err)
	}
	return h.writeHTML("simulate_report.html", tmpl, report)
}

func (h *HTMLRenderer) loadTemplate(path, fallback string) (*template.Template, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		// Template file not found or not readable; use embedded fallback.
		slog.Warn("report: template file not readable, using fallback",
			slog.String("path", path),
			slog.String("error", err.Error()),
		)
		return template.New("report").Parse(fallback)
	}
	return template.New("report").Parse(string(data))
}

func (h *HTMLRenderer) writeHTML(filename string, tmpl *template.Template, data any) error {
	if err := os.MkdirAll(h.Path, 0o755); err != nil {
		return fmt.Errorf("report: rendering html: %w", err)
	}

	outPath := filepath.Join(h.Path, filename)
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("report: rendering html: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("report: rendering html: %w", err)
	}

	return nil
}
