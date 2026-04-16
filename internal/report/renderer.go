package report

import (
	"os"

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
// path is the output directory for json/html files.
func NewRendererFromFormat(format, path string) Renderer {
	switch format {
	case "json":
		return &JSONRenderer{Path: path}
	case "html":
		return &HTMLRenderer{Path: path}
	case "both":
		// "both" means HTML + JSON. The verbose text block that
		// used to print to stdout duplicated what the completion
		// summary already shows and the HTML covers in detail; the
		// completion summary is the single source of truth at the
		// tty while the HTML/JSON files carry every row.
		return &MultiRenderer{
			renderers: []Renderer{
				&HTMLRenderer{Path: path},
				&JSONRenderer{Path: path},
			},
		}
	default:
		return &StdoutRenderer{Writer: os.Stdout}
	}
}
