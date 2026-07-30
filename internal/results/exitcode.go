package results

// Exit codes are fixed — this tool runs in pipelines, and for automation these
// numbers are the interface. Do not add, reorder or reuse them.
// See docs/ADR/0006-exit-code-contract.md.
const (
	// ExitPass — every check passed (or was legitimately skipped).
	ExitPass = 0
	// ExitBlocker — at least one blocker-severity check failed.
	ExitBlocker = 1
	// ExitWarning — no blockers failed, but warnings failed or some checks
	// could not tell.
	ExitWarning = 2
	// ExitToolError — the tool could not do its job: bad config, unreadable
	// credentials file, a check that panicked. Says nothing about the
	// environment. Never read this as "the environment is fine".
	ExitToolError = 3
)

// ExitCode derives the process exit code from a result set.
//
// Precedence: tool error > blocker > warning > pass. A check that could not
// tell (StatusUnknown) never produces ExitBlocker, even at blocker severity —
// we did not see a failure, so we do not report one. It produces ExitWarning,
// so a pipeline still notices.
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
		// or done nothing but config arithmetic. This says what the code
		// actually means — nothing failed — without implying coverage the run
		// may not have had.
		return "no check failed"
	case ExitBlocker:
		return "one or more blockers failed"
	case ExitWarning:
		return "only warnings, or checks that could not tell"
	case ExitToolError:
		return "tool error"
	default:
		return "unknown exit code"
	}
}
