package report

import (
	"os"

	"github.com/rs/zerolog"
	"github.com/ruptor-dev/cli/pkg/types"
)

// Renderer defines the interface for rendering chaos and simulation reports.
type Renderer interface {
	RenderChaos(report *types.ReliabilityReport) error
	RenderSimulate(report *types.ConversationReport) error
}

// MultiRenderer wraps multiple renderers and calls all of them in sequence.
type MultiRenderer struct {
	renderers []Renderer
}

// RenderChaos calls RenderChaos on each wrapped renderer, returning the first error.
func (m *MultiRenderer) RenderChaos(report *types.ReliabilityReport) error {
	for _, r := range m.renderers {
		if err := r.RenderChaos(report); err != nil {
			return err
		}
	}
	return nil
}

// RenderSimulate calls RenderSimulate on each wrapped renderer, returning the first error.
func (m *MultiRenderer) RenderSimulate(report *types.ConversationReport) error {
	for _, r := range m.renderers {
		if err := r.RenderSimulate(report); err != nil {
			return err
		}
	}
	return nil
}

// NewRendererFromFormat creates a Renderer for the given output format.
// Supported formats: "stdout", "json", "html", "both" (stdout + html).
// path is the output directory for json/html files. logger is used by
// renderers that emit diagnostics (currently only HTMLRenderer, for
// template-load failures); pass the process-wide logger from the
// caller.
func NewRendererFromFormat(format, path string, logger zerolog.Logger) Renderer {
	switch format {
	case "json":
		return &JSONRenderer{Path: path}
	case "html":
		return &HTMLRenderer{Path: path, Logger: logger}
	case "both":
		return &MultiRenderer{
			renderers: []Renderer{
				&StdoutRenderer{Writer: os.Stdout},
				&HTMLRenderer{Path: path, Logger: logger},
			},
		}
	default:
		return &StdoutRenderer{Writer: os.Stdout}
	}
}
