package results_test

import (
	"testing"

	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// This is the reference TABLE TEST for the repo. New pure-logic tests should
// look like this one: a named case per behaviour, one assertion, no shared
// mutable state, and a name that reads as a sentence in the failure output.
//
// The exit-code contract is the right thing to pin first — it is the only part
// of the tool a CI pipeline actually depends on, and a regression here breaks
// gates silently rather than loudly.
func TestExitCode(t *testing.T) {
	t.Parallel()

	res := func(status results.Status, sev results.Severity) results.Result {
		return results.Result{Status: status, Severity: sev}
	}

	tests := []struct {
		name string
		in   []results.Result
		want int
	}{
		{
			name: "no results is a pass",
			in:   nil,
			want: results.ExitPass,
		},
		{
			name: "all passing is a pass",
			in: []results.Result{
				res(results.StatusPass, results.SeverityBlocker),
				res(results.StatusPass, results.SeverityWarning),
			},
			want: results.ExitPass,
		},
		{
			name: "skips do not degrade the exit code",
			in: []results.Result{
				res(results.StatusPass, results.SeverityBlocker),
				res(results.StatusSkip, results.SeverityBlocker),
			},
			want: results.ExitPass,
		},
		{
			name: "a failed blocker is exit 1",
			in: []results.Result{
				res(results.StatusPass, results.SeverityBlocker),
				res(results.StatusFail, results.SeverityBlocker),
			},
			want: results.ExitBlocker,
		},
		{
			name: "a failed warning is exit 2",
			in: []results.Result{
				res(results.StatusFail, results.SeverityWarning),
			},
			want: results.ExitWarning,
		},
		{
			name: "a failed info does not degrade the exit code",
			in: []results.Result{
				res(results.StatusFail, results.SeverityInfo),
			},
			want: results.ExitPass,
		},
		{
			name: "blocker outranks warning regardless of order",
			in: []results.Result{
				res(results.StatusFail, results.SeverityWarning),
				res(results.StatusFail, results.SeverityBlocker),
			},
			want: results.ExitBlocker,
		},
		{
			name: "warning does not downgrade an earlier blocker",
			in: []results.Result{
				res(results.StatusFail, results.SeverityBlocker),
				res(results.StatusFail, results.SeverityWarning),
			},
			want: results.ExitBlocker,
		},
		{
			// The load-bearing case. An indeterminate blocker must NOT report
			// as a failed blocker: we did not observe a failure, so we do not
			// get to assert one. It still has to be visible, hence exit 2.
			name: "an indeterminate blocker is exit 2, not exit 1",
			in: []results.Result{
				res(results.StatusUnknown, results.SeverityBlocker),
			},
			want: results.ExitWarning,
		},
		{
			name: "an indeterminate result does not mask a real blocker",
			in: []results.Result{
				res(results.StatusUnknown, results.SeverityBlocker),
				res(results.StatusFail, results.SeverityBlocker),
			},
			want: results.ExitBlocker,
		},
		{
			name: "a tool error outranks everything",
			in: []results.Result{
				res(results.StatusFail, results.SeverityBlocker),
				res(results.StatusError, results.SeverityInfo),
			},
			want: results.ExitToolError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := results.ExitCode(tt.in); got != tt.want {
				t.Errorf("ExitCode() = %d (%s), want %d (%s)",
					got, results.ExitCodeText(got), tt.want, results.ExitCodeText(tt.want))
			}
		})
	}
}

func TestSummarise(t *testing.T) {
	t.Parallel()

	in := []results.Result{
		{Status: results.StatusPass, Severity: results.SeverityBlocker},
		{Status: results.StatusFail, Severity: results.SeverityBlocker},
		{Status: results.StatusFail, Severity: results.SeverityWarning},
		{Status: results.StatusFail, Severity: results.SeverityInfo},
		{Status: results.StatusSkip, Severity: results.SeverityBlocker},
		{Status: results.StatusUnknown, Severity: results.SeverityWarning},
		{Status: results.StatusError, Severity: results.SeverityInfo},
	}

	got := results.Summarise(in)
	want := results.Summary{
		Total: 7, Pass: 1, Fail: 3, Skip: 1, Unknown: 1, Errors: 1,
		Blockers: 1, Warnings: 1,
	}
	if got != want {
		t.Errorf("Summarise() = %+v, want %+v", got, want)
	}
}
