// Package renderers turns a Report into output.
//
// Renderers are the only place in the tool that formats for a human. Checks
// never print; the engine never prints. That separation is what lets the same
// run feed a terminal, a CI JUnit collector and — later — a web UI, which is
// just another JSON consumer and gets no special support.
// See docs/ADR/0004-pluggable-renderers.md.
package renderers

import (
	"fmt"
	"io"
	"sort"

	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// Renderer writes a report to a stream.
//
// A Renderer must be pure with respect to the report: given the same Report it
// must produce byte-identical output. No timestamps of its own, no map
// iteration order, no colour unless told. Golden tests depend on it, and so
// does anyone diffing two saved reports.
type Renderer interface {
	// Name is the --format value that selects this renderer.
	Name() string
	// Render writes the report. It must not close w.
	Render(w io.Writer, rep *results.Report) error
}

// Options tune rendering. Kept separate from the Renderer so the same renderer
// instance is reusable and so tests can force deterministic settings.
type Options struct {
	// Colour enables ANSI colour. Callers should default it to
	// "stdout is a TTY and NO_COLOR is unset".
	Colour bool
	// Verbose includes evidence and passing detail.
	Verbose bool
	// ShowSkipped includes skipped checks in human output. On by default in
	// JSON always — a machine consumer must be able to tell "passed" from
	// "never ran", and hiding skips is how a report starts lying.
	ShowSkipped bool
}

// New returns the renderer for a format name.
func New(format string, opts Options) (Renderer, error) {
	switch format {
	case "", "terminal", "text", "human":
		return &Terminal{Opts: opts}, nil
	case "json":
		return &JSON{Opts: opts}, nil
	case "junit", "junit-xml":
		return &JUnit{Opts: opts}, nil
	default:
		return nil, fmt.Errorf("unknown format %q (known: %v)", format, Formats())
	}
}

// Formats lists the selectable format names.
func Formats() []string { return []string{"terminal", "json", "junit"} }

// bySeverityThenID orders results for human display: the things that stop a
// deployment first, then the things that will bite later, then the rest.
func bySeverityThenID(res []results.Result) []results.Result {
	out := append([]results.Result(nil), res...)
	rank := func(r results.Result) int {
		switch {
		case r.Status == results.StatusError:
			return 0
		case r.Status == results.StatusFail && r.Severity == results.SeverityBlocker:
			return 1
		case r.Status == results.StatusFail && r.Severity == results.SeverityWarning:
			return 2
		case r.Status == results.StatusUnknown:
			return 3
		case r.Status == results.StatusFail:
			return 4
		case r.Status == results.StatusPass:
			return 5
		default: // skip
			return 6
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := rank(out[i]), rank(out[j])
		if ri != rj {
			return ri < rj
		}
		if out[i].CheckID != out[j].CheckID {
			return out[i].CheckID < out[j].CheckID
		}
		return out[i].Target < out[j].Target
	})
	return out
}
