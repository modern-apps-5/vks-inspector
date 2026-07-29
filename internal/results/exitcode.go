package results

// Exit codes are contractual — this tool runs in pipelines and the numbers are
// part of its API. Do not add, reorder or repurpose these.
// See docs/ADR/0006-exit-code-contract.md.
const (
	// ExitPass — every check passed (or was legitimately skipped).
	ExitPass = 0
	// ExitBlocker — at least one blocker-severity check failed.
	ExitBlocker = 1
	// ExitWarning — no blockers failed, but warnings failed or results were
	// indeterminate.
	ExitWarning = 2
	// ExitToolError — the tool could not do its job: bad config, unreadable
	// credentials file, a check that panicked. Says nothing about the
	// environment. Callers must not read this as "environment is fine".
	ExitToolError = 3
)

// ExitCode derives the process exit code from a result set.
//
// Precedence: tool error > blocker > warning > pass. An indeterminate result
// (StatusUnknown) never produces ExitBlocker even on a blocker-severity check —
// we did not observe a failure, so we do not assert one. It produces
// ExitWarning so a pipeline still notices.
func ExitCode(res []Result) int {
	code := ExitPass
	for _, r := range res {
		switch {
		case r.Status == StatusError:
			return ExitToolError
		case r.Status == StatusFail && r.Severity == SeverityBlocker:
			code = ExitBlocker
		case r.Status == StatusFail && r.Severity == SeverityWarning:
			if code != ExitBlocker {
				code = ExitWarning
			}
		case r.Status == StatusUnknown:
			if code != ExitBlocker {
				code = ExitWarning
			}
		}
	}
	return code
}

// ExitCodeText is for help output and docs.
func ExitCodeText(code int) string {
	switch code {
	case ExitPass:
		// Not "all checks passed": a run may have skipped most of its checks,
		// or run only config arithmetic. This states the contract — nothing
		// failed — without implying coverage the run may not have had.
		return "no check failed"
	case ExitBlocker:
		return "one or more blockers failed"
	case ExitWarning:
		return "warnings or indeterminate results only"
	case ExitToolError:
		return "tool error"
	default:
		return "unknown exit code"
	}
}
