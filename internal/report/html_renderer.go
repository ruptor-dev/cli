package report

import (
	"embed"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/ruptor-dev/cli/pkg/types"
)

// HTMLRenderer renders reports as HTML files using Go templates.
type HTMLRenderer struct {
	Path string
}

//go:embed templates/chaos_report.html.tmpl templates/simulate_report.html.tmpl
var embeddedTemplates embed.FS

// RenderChaos renders a reliability report as HTML.
func (h *HTMLRenderer) RenderChaos(report *types.ReliabilityReport) error {
	tmpl, err := h.loadTemplate("web/chaos_report.html.tmpl", "templates/chaos_report.html.tmpl")
	if err != nil {
		return fmt.Errorf("report: rendering html: %w", err)
	}
	return h.writeHTML("chaos_report.html", tmpl, report)
}

// RenderSimulate renders a conversation report as HTML.
func (h *HTMLRenderer) RenderSimulate(report *types.ConversationReport) error {
	tmpl, err := h.loadTemplate("web/simulate_report.html.tmpl", "templates/simulate_report.html.tmpl")
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
	"scoreClass": func(score float64) string {
		p := score * 100
		switch {
		case p >= 80:
			return "good"
		case p >= 60:
			return "mid"
		default:
			return "bad"
		}
	},
	"humanDur": func(ms int64) string {
		if ms <= 0 {
			return "—"
		}
		if ms < 1000 {
			return fmt.Sprintf("%dms", ms)
		}
		if ms < 60_000 {
			return fmt.Sprintf("%.1fs", float64(ms)/1000)
		}
		m := ms / 60_000
		s := (ms % 60_000) / 1000
		return fmt.Sprintf("%dm%02ds", m, s)
	},
	"anyJudge": func(results []types.TestResult) bool {
		for _, r := range results {
			v := strings.ToUpper(strings.TrimSpace(r.LLMJudgeVerdict))
			if v != "" && v != "SKIPPED" && v != "SKIP" {
				return true
			}
		}
		return false
	},
	"failureReason": func(r types.TestResult) string {
		if strings.TrimSpace(r.Error) != "" {
			return r.Error
		}
		if strings.TrimSpace(r.LLMJudgeReason) != "" {
			return r.LLMJudgeReason
		}
		if len(r.DetectedBehaviors) > 0 {
			parts := make([]string, 0, len(r.DetectedBehaviors))
			for _, b := range r.DetectedBehaviors {
				parts = append(parts, humanizeBehavior(b))
			}
			return strings.Join(parts, " · ")
		}
		return "agent did not handle the injected fault"
	},
}

// loadTemplate prefers an on-disk override at `path` (useful for local
// template hacking during development) and falls back to the embedded
// copy baked into the binary via go:embed. The embedded version is the
// production default; users never ship with a web/ directory alongside
// the binary.
func (h *HTMLRenderer) loadTemplate(onDiskPath, embeddedPath string) (*template.Template, error) {
	t := template.New("report").Funcs(templateFuncs)

	if data, err := os.ReadFile(onDiskPath); err == nil {
		return t.Parse(string(data))
	}

	data, err := embeddedTemplates.ReadFile(embeddedPath)
	if err != nil {
		log.Error().Err(err).Str("embedded", embeddedPath).Msg("report: embedded template missing")
		return nil, err
	}
	return t.Parse(string(data))
}

// humanizeBehavior turns the evaluator's machine-readable codes into
// short reviewer-friendly phrases suitable for the FAIL row reason
// line in the HTML report. Kept alongside the template funcs so the
// mapping lives in the one place the report renders from.
func humanizeBehavior(b types.DetectedBehavior) string {
	switch b {
	case types.BehaviorCrash:
		return "agent errored on the injected response"
	case types.BehaviorRecoveryFailed:
		return "no retry or fallback path"
	case types.BehaviorRecoverySuccess:
		return "agent recovered"
	case types.BehaviorInfiniteLoop:
		return "stuck in a retry loop"
	case types.BehaviorHallucination:
		return "hallucinated a result instead of surfacing the failure"
	case types.BehaviorFallbackUsed:
		return "used a fallback path"
	case types.BehaviorTimeout:
		return "agent timed out before the fault was released"
	default:
		return string(b)
	}
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
