package main

import (
	"github.com/spf13/cobra"
)

// newDriftCmd re-runs against a stored baseline and reports what changed.
//
// Stubbed in phase 1. This is the command that constrains everything else: it
// is why Result.Observed carries structured Data rather than a sentence, and
// why every result is serialisable. See docs/ADR/0003-structured-observations.md.
func newDriftCmd(g *globalOpts) *cobra.Command {
	var baseline string

	cmd := &cobra.Command{
		Use:   "drift",
		Short: "Compare current state against a stored baseline (phase 4)",
		Long: `Re-run checks and report what changed relative to a stored baseline.

Not implemented in phase 1.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// TODO(phase-4): load the baseline with results.ReadBaseline, run
			// the engine in the baseline's own mode, and diff with
			// results.DiffBaseline. Semantics that must be settled first:
			//   - a change in Run.ConfigDigest means the declared intent
			//     changed, which is not environment drift and must be reported
			//     separately or the report is misleading;
			//   - refusing to diff across differing Run.Mode;
			//   - what exit code drift uses. Reusing 1/2 by severity is the
			//     obvious choice but conflates "a check failed" with "something
			//     changed", and those are different questions.
			return notImplemented(cmd, "phase 4",
				"drift diffs two results.Reports; the diff types already exist in\n"+
					"internal/results/baseline.go (Change, DiffBaseline).")
		},
	}
	cmd.Flags().StringVar(&baseline, "baseline", "", "path to the stored baseline artifact")
	return cmd
}
