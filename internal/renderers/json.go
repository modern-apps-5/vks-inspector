package renderers

import (
	"encoding/json"
	"io"

	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// JSON renders the report as indented JSON.
//
// This is the format the future web UI consumes, the format `snapshot` writes,
// and the format `drift` reads. It is therefore the tool's real interface and
// the terminal renderer is the derived one — not the other way round. Any field
// a human can see in the terminal must be present here.
type JSON struct {
	Opts Options
}

// Name implements Renderer.
func (j *JSON) Name() string { return "json" }

// Render implements Renderer.
//
// Note what is NOT done here: no filtering of skipped results, ever, regardless
// of Options.ShowSkipped. A machine consumer must be able to distinguish "this
// check passed" from "this check never ran", and a JSON document that silently
// omits the latter is a document that lies by omission.
func (j *JSON) Render(w io.Writer, rep *results.Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	// SetEscapeHTML(false) keeps FQDNs and URLs readable in the output.
	enc.SetEscapeHTML(false)
	return enc.Encode(rep)
}
