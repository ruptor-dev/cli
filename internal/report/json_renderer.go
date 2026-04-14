package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/faultforge/faultforge/pkg/types"
)

// JSONRenderer writes reports as formatted JSON files.
type JSONRenderer struct {
	Path string
}

// RenderChaos writes a reliability report as JSON to {path}/chaos_report.json.
func (j *JSONRenderer) RenderChaos(report *types.ReliabilityReport) error {
	return j.writeJSON("chaos_report.json", report)
}

// RenderSimulate writes a conversation report as JSON to {path}/simulate_report.json.
func (j *JSONRenderer) RenderSimulate(report *types.ConversationReport) error {
	return j.writeJSON("simulate_report.json", report)
}

func (j *JSONRenderer) writeJSON(filename string, v any) error {
	if err := os.MkdirAll(j.Path, 0o755); err != nil {
		return fmt.Errorf("report: creating output directory: %w", err)
	}

	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("report: marshalling json: %w", err)
	}

	outPath := filepath.Join(j.Path, filename)
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return fmt.Errorf("report: writing json file: %w", err)
	}

	return nil
}
