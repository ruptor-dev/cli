package report

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
	"github.com/ruptor-dev/cli/pkg/types"
)

// HTMLRenderer renders reports as HTML files using Go templates.
type HTMLRenderer struct {
	Path string
}

const fallbackChaosHTML = `<!DOCTYPE html>
<html><head><title>Ruptor Reliability Report</title>
<style>body{font-family:sans-serif;margin:2em}table{border-collapse:collapse;width:100%}th,td{border:1px solid #ccc;padding:8px;text-align:left}.pass{color:green}.fail{color:red}</style>
</head><body>
<h1>Ruptor Reliability Report</h1>
<p><strong>Agent:</strong> {{.AgentName}} | <strong>Run:</strong> {{.RunAt.Format "2006-01-02 15:04:05"}} | <strong>Score:</strong> {{printf "%.0f" (pct .Score)}}%</p>
<p>Tests: {{.TotalTests}} | Passed: {{.Passed}} | Failed: {{.Failed}}</p>
<table><tr><th>Status</th><th>Test ID</th><th>Fault</th><th>Tool</th><th>Duration</th><th>Judge</th></tr>
{{range .Results}}<tr>
<td class="{{if .Passed}}pass{{else}}fail{{end}}">{{if .Passed}}PASS{{else}}FAIL{{end}}</td>
<td>{{.TestID}}</td><td>{{.FaultType}}</td><td>{{.Tool}}</td><td>{{.DurationMs}}ms</td>
<td>{{.LLMJudgeVerdict}}{{if .LLMJudgeReason}} &mdash; {{.LLMJudgeReason}}{{end}}</td>
</tr>{{end}}
</table></body></html>`

const fallbackSimulateHTML = `<!DOCTYPE html>
<html><head><title>Ruptor Simulation Report</title>
<style>body{font-family:sans-serif;margin:2em}table{border-collapse:collapse;width:100%}th,td{border:1px solid #ccc;padding:8px;text-align:left}.reached{color:green}.missed{color:red}</style>
</head><body>
<h1>Ruptor Simulation Report</h1>
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

// templateFuncs exposes helpers the report templates need. `pct`
// converts the canonical 0.0–1.0 Score into a 0–100 number so the
// template can format it as "67%". Kept here so the embedded fallback
// and the on-disk template share one source.
var templateFuncs = template.FuncMap{
	"pct": func(x float64) float64 { return x * 100 },
}

func (h *HTMLRenderer) loadTemplate(path, fallback string) (*template.Template, error) {
	t := template.New("report").Funcs(templateFuncs)
	data, err := os.ReadFile(path)
	if err != nil {
		// Template file not found or not readable; use embedded fallback.
		log.Warn().
			Str("path", path).
			Err(err).
			Msg("report: template file not readable, using fallback")
		return t.Parse(fallback)
	}
	return t.Parse(string(data))
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
