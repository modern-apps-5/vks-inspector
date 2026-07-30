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
	bw.printf("  ran from  %s\n", rep.Run.Vantage)
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
		// Expected/observed are printed for passes too. A report is a record,
		// not just a verdict: "it passed" is far less useful to whoever reads
		// it later than "it passed, and here is what was seen".
		//
		// On a failure the observation is relabelled "problem" and printed
		// first. A reader scanning a red block wants the fault, not a restated
		// requirement they have to diff against it.
		if r.Status == results.StatusPass {
			bw.printf("      expected  %s\n", r.Expected.Summary)
			bw.printf("      observed  %s\n", r.Observed.Summary)
		} else {
			bw.printf("      problem   %s\n", wrapIndent(r.Observed.Summary, 16, 78))
			bw.printf("      expected  %s\n", wrapIndent(r.Expected.Summary, 16, 78))
		}
	}

	if r.Status != results.StatusPass && r.Status != results.StatusSkip && r.Impact != "" {
		bw.printf("      impact    %s\n", wrapIndent(r.Impact, 16, 78))
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

// renderCoverage says what this run was actually able to inspect.
//
// It sits immediately above the verdict on purpose. "4 passed, 0 failed" and
// "this environment is ready" are different claims, and anyone handed only the
// first will assume the second. A run that never opened a socket has to say so
// next to its own green tick, not in a footnote.
func (t *Terminal) renderCoverage(bw *errWriter, rep *results.Report) {
	c := rep.Run.Coverage

	if !c.EnvironmentContacted {
		bw.printf("%s\n", t.paint(cYellow,
			"NOTHING IN THIS RUN CONTACTED YOUR ENVIRONMENT."))
		bw.printf("         %d config-only check(s) graded the addressing you declared.\n", c.ConfigOnly)
		bw.printf("         No packet was sent. No management-plane API was called.\n")
		bw.printf("         %s\n\n", t.paint(cYellow,
			"A pass here does NOT mean the environment is ready."))
		return
	}

	bw.printf("checks    %d ran of %d in this build — %d config-only, %d network probe(s), %d API check(s)\n",
		c.Executed, c.ChecksInBuild, c.ConfigOnly, c.NetworkProbes, c.APIChecks)

	// Name the access this run did not have. Otherwise the reader sees the pass
	// count and has to work out for themselves that five checks never ran.
	if len(c.MissingCapabilities) > 0 {
		names := make([]string, 0, len(c.MissingCapabilities))
		for k := range c.MissingCapabilities {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			bw.printf("          %s\n", t.paint(cYellow, fmt.Sprintf(
				"no %s access — %d check(s) could not run, so nothing above covers it",
				k, c.MissingCapabilities[k])))
		}
	}
	bw.printf("\n")
}

func (t *Terminal) renderSummary(bw *errWriter, rep *results.Report) {
	s := rep.Summary
	t.renderCoverage(bw, rep)
	bw.printf("%s  %d checks: %d passed, %d failed, %d skipped, %d unknown, %d errors\n",
		t.paint(cBold, "summary"), s.Total, s.Pass, s.Fail, s.Skip, s.Unknown, s.Errors)

	// A run in which nothing actually executed must not read as a clean bill of
	// health. "No blockers" is technically true and deeply misleading when the
	// reason is that no check ran — a narrow --layer or --only, or credentials
	// that were never supplied.
	if s.Total > 0 && s.Pass == 0 && s.Fail == 0 && s.Unknown == 0 && s.Errors == 0 {
		bw.printf("          %s\n", t.paint(cYellow,
			"every check was skipped — this run inspected nothing, so it says nothing about readiness"))
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
		if !rep.Run.Coverage.EnvironmentContacted {
			// "all checks passed" is true and misleading in the same breath
			// when the checks were arithmetic. Say what passed.
			bw.printf("          %s\n", t.paint(cGreen,
				"the declared configuration is internally consistent"))
			bw.printf("          %s\n", t.paint(cGrey,
				"the environment itself has not been inspected"))
		} else {
			bw.printf("          %s\n", t.paint(cGreen, "no blockers, no warnings"))
		}
	case results.ExitBlocker:
		bw.printf("          %s\n", t.paint(cRed, fmt.Sprintf("%d blocker(s) must be fixed before deployment", s.Blockers)))
	case results.ExitWarning:
		bw.printf("          %s\n", t.paint(cYellow, fmt.Sprintf("%d warning(s), %d could not be determined", s.Warnings, s.Unknown)))
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
