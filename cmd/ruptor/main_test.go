package main

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/ruptor-dev/cli/internal/config"
	"github.com/ruptor-dev/cli/internal/ui"
	"github.com/stretchr/testify/assert"
)

func TestReportPathsFor(t *testing.T) {
	cases := []struct {
		name       string
		out        config.OutputConfig
		outputPath string
		want       []string
	}{
		{
			name:       "explicit outputPath overrides",
			out:        config.OutputConfig{Format: "both", Path: "./reports/"},
			outputPath: "/tmp/custom.json",
			want:       []string{"/tmp/custom.json"},
		},
		{
			name: "json format",
			out:  config.OutputConfig{Format: "json", Path: "./reports/"},
			want: []string{"./reports/chaos_report.json"},
		},
		{
			name: "html format",
			out:  config.OutputConfig{Format: "html", Path: "./reports/"},
			want: []string{"./reports/chaos_report.html"},
		},
		{
			name: "both format lists html and json",
			out:  config.OutputConfig{Format: "both", Path: "./reports/"},
			want: []string{"./reports/chaos_report.html", "./reports/chaos_report.json"},
		},
		{
			name: "unknown format returns empty",
			out:  config.OutputConfig{Format: "toml", Path: "./reports/"},
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := reportPathsFor(nil, tc.out, tc.outputPath)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("reportPathsFor: got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestWarnIfMCPModeUnscored_FiresForMCPAndAuto asserts the runtime
// warning (mcpUnscoredWarning) reaches ui.ErrWriter() when proxy.mode
// is "mcp" or "auto" — the two modes that can route traffic through
// the MCP handler. HTTP-only runs must stay silent so the copy does
// not apply to users who are unaffected by the observation gap. The
// warning is live until docs/specs/backlog/mcp-observations-evaluator.md
// lands; it is NOT a debug artifact. Warnings are routed to stderr so
// `ruptor run ... | jq` keeps stdout clean; the capture therefore goes
// through ui.SetErrWriter, not ui.SetWriter.
func TestWarnIfMCPModeUnscored_FiresForMCPAndAuto(t *testing.T) {
	cases := []struct {
		mode    string
		warn    bool
		comment string
	}{
		{mode: "mcp", warn: true, comment: "explicit MCP must warn"},
		{mode: "MCP", warn: true, comment: "mode is normalised — uppercase must warn too"},
		{mode: " mcp ", warn: true, comment: "surrounding whitespace must not hide MCP"},
		{mode: "auto", warn: true, comment: "auto may route to MCP at runtime — must warn"},
		{mode: "http", warn: false, comment: "pure HTTP is unaffected — no warning"},
		{mode: "", warn: false, comment: "default (empty) is HTTP — no warning"},
	}
	for _, tc := range cases {
		t.Run("mode="+tc.mode, func(t *testing.T) {
			var buf bytes.Buffer
			prev := ui.ErrWriter()
			ui.SetErrWriter(&buf)
			defer ui.SetErrWriter(prev)

			warnIfMCPModeUnscored(&config.ChaosConfig{
				Proxy: config.ProxyConfig{Mode: tc.mode},
			})

			got := buf.String()
			if tc.warn {
				assert.Contains(t, got, "docs/specs/backlog/mcp-observations-evaluator.md",
					"%s: warning must name the backlog spec path", tc.comment)
				assert.Contains(t, got, "Robustness",
					"%s: warning must explain the scoring impact", tc.comment)
			} else {
				assert.Empty(t, got, "%s: writer must receive nothing", tc.comment)
			}
		})
	}
}

// TestMCPUnscoredWarning_NamesBacklogSpec freezes the one contract
// callers rely on: the warning text points at the backlog spec that
// tracks the fix. A curious user must be able to grep their terminal
// for the path and land on the tracking doc.
func TestMCPUnscoredWarning_NamesBacklogSpec(t *testing.T) {
	assert.Contains(t, mcpUnscoredWarning,
		"docs/specs/backlog/mcp-observations-evaluator.md",
		"warning const must name the backlog spec so users can find the tracking doc")
}
