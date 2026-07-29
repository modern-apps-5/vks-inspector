package renderers

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// Terminal renders a human-readable report.
type Terminal struct {
	Opts Options
}

// Name implements Renderer.
func (t *Terminal) Name() string { return "terminal" }

// ANSI codes, applied only when Opts.Colour is set.
const (
	cReset  = "\x1b[0m"
	cRed    = "\x1b[31m"
	cGreen  = "\x1b[32m"
	cYellow = "\x1b[33m"
	cGrey   = "\x1b[90m"
	cBold   = "\x1b[1m"
)

func (t *Terminal) paint(code, s string) string {
	if !t.Opts.Colour {
		return s
	}
	return code + s + cReset
}

// statusTag returns a fixed-width status label. Fixed width so columns line up
// without a table library, and so the golden file stays readable in a diff.
func (t *Terminal) statusTag(r results.Result) string {
	switch r.Status {
	case results.StatusPass:
		return t.paint(cGreen, "PASS ")
	case results.StatusFail:
		if r.Severity == results.SeverityBlocker {
			return t.paint(cRed, "BLOCK")
		}
		return t.paint(cYellow, "WARN ")
	case results.StatusUnknown:
		return t.paint(cYellow, "UNKWN")
	case results.StatusError:
		return t.paint(cRed, "ERROR")
	default:
		return t.paint(cGrey, "SKIP ")
	}
}

// Render implements Renderer.
func (t *Terminal) Render(w io.Writer, rep *results.Report) error {
	bw := &errWriter{w: w}

	bw.printf("%s  %s\n", t.paint(cBold, "vksinspect"), rep.Tool.Version)
	bw.printf("  mode      %s\n", rep.Run.Mode)
	bw.printf("  topology  %s\n", rep.Run.Topology)
	bw.printf("  vantage   %s\n", rep.Run.Vantage)
	if rep.Run.Placeholder {
		bw.printf("  answers   %s\n", t.paint(cYellow, "PLACEHOLDER — example values, not a real environment"))
	}
	if rep.Run.Invasive {
		bw.printf("  probes    %s\n", t.paint(cYellow, "invasive probes ENABLED"))
	} else {
		bw.printf("  probes    read-only (non-invasive)\n")
	}
	bw.printf("\n")

	shown := 0
	for _, r := range bySeverityThenID(rep.Results) {
		if r.Status == results.StatusSkip && !t.Opts.ShowSkipped {
			continue
		}
		shown++
		t.renderResult(bw, r)
	}
	if shown == 0 {
		bw.printf("  (no results to display)\n")
	}

	bw.printf("\n")
	t.renderSummary(bw, rep)
	return bw.err
}

func (t *Terminal) renderResult(bw *errWriter, r results.Result) {
	target := ""
	if r.Target != "" {
		target = "  [" + r.Target + "]"
	}
	bw.printf("%s %s%s\n", t.statusTag(r), r.Title, target)
	bw.printf("      check     %s", r.CheckID)
	if len(r.RequirementIDs) > 0 {
		bw.printf("  (%s)", strings.Join(r.RequirementIDs, ", "))
	}
	bw.printf("\n")

	switch r.Status {
	case results.StatusSkip:
		bw.printf("      skipped   %s\n", r.Observed.Summary)
	case results.StatusError:
		bw.printf("      error     %s\n", r.Err)
	default:
		// Expected/observed are printed for passes too. A preflight report is
		// evidence, not just a verdict: "it passed" is far less useful to the
		// person reading it later than "it passed, and here is what was seen".
		bw.printf("      expected  %s\n", r.Expected.Summary)
		bw.printf("      observed  %s\n", r.Observed.Summary)
	}

	if r.Status != results.StatusPass && r.Status != results.StatusSkip && r.Remediation != "" {
		bw.printf("      fix       %s\n", wrapIndent(r.Remediation, 16, 78))
	}
	if t.Opts.Verbose && len(r.Evidence) > 0 {
		keys := make([]string, 0, len(r.Evidence))
		for k := range r.Evidence {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			bw.printf("      %-9s %v\n", "· "+k, r.Evidence[k])
		}
	}
	bw.printf("\n")
}

func (t *Terminal) renderSummary(bw *errWriter, rep *results.Report) {
	s := rep.Summary
	bw.printf("%s  %d checks: %d passed, %d failed, %d skipped, %d indeterminate, %d errors\n",
		t.paint(cBold, "summary"), s.Total, s.Pass, s.Fail, s.Skip, s.Unknown, s.Errors)

	// A run in which nothing actually executed must not read as a clean bill of
	// health. "No blockers" is technically true and deeply misleading when the
	// reason is that no check ran — a narrow --layer or --only, or credentials
	// that were never supplied.
	if s.Total > 0 && s.Pass == 0 && s.Fail == 0 && s.Unknown == 0 && s.Errors == 0 {
		bw.printf("          %s\n", t.paint(cYellow,
			"every check was skipped — this run inspected nothing and is not evidence of readiness"))
		bw.printf("          exit code %d (%s)\n",
			results.ExitCode(rep.Results), results.ExitCodeText(results.ExitCode(rep.Results)))
		return
	}

	// A green verdict on invented addresses is the most dangerous output this
	// tool can produce. Say so where the verdict is, not only in the header.
	if rep.Run.Placeholder {
		bw.printf("          %s\n", t.paint(cYellow,
			"graded against PLACEHOLDER answers — this is not a readiness assessment"))
	}

	switch code := results.ExitCode(rep.Results); code {
	case results.ExitPass:
		bw.printf("          %s\n", t.paint(cGreen, "no blockers, no warnings"))
	case results.ExitBlocker:
		bw.printf("          %s\n", t.paint(cRed, fmt.Sprintf("%d blocker(s) must be fixed before deployment", s.Blockers)))
	case results.ExitWarning:
		bw.printf("          %s\n", t.paint(cYellow, fmt.Sprintf("%d warning(s), %d indeterminate", s.Warnings, s.Unknown)))
	default:
		bw.printf("          %s\n", t.paint(cRed, "tool error — results are incomplete"))
	}

	if s.Skip > 0 && !t.Opts.ShowSkipped {
		bw.printf("          %s\n", t.paint(cGrey,
			fmt.Sprintf("%d check(s) skipped and not shown; re-run with --show-skipped", s.Skip)))
	}
	bw.printf("          exit code %d (%s)\n",
		results.ExitCode(rep.Results), results.ExitCodeText(results.ExitCode(rep.Results)))
}

// errWriter collapses repeated error checking in the render path.
type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) printf(format string, args ...any) {
	if e.err != nil {
		return
	}
	_, e.err = fmt.Fprintf(e.w, format, args...)
}

// wrapIndent wraps text to width, indenting continuation lines.
func wrapIndent(s string, indent, width int) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}
	pad := strings.Repeat(" ", indent)
	var b strings.Builder
	line := indent
	for i, word := range words {
		if i > 0 {
			if line+1+len(word) > width {
				b.WriteString("\n" + pad)
				line = indent
			} else {
				b.WriteString(" ")
				line++
			}
		}
		b.WriteString(word)
		line += len(word)
	}
	return b.String()
}
