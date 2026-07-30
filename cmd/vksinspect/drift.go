package main

import (
	"github.com/spf13/cobra"
)

// newDriftCmd re-runs against a stored baseline and reports what changed.
//
// Stubbed in phase 1. This is the command that shapes everything else: it is
// why Result.Observed carries Data rather than just a sentence, and why every
// result can be written to a file. See docs/ADR/0003-structured-observations.md.
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
			// results.DiffBaseline. Questions to settle first:
			//   - a change in Run.ConfigDigest means what we declared changed,
			//     which is not the environment drifting and has to be reported
			//     separately or the report misleads;
			//   - refusing to compare across different Run.Mode values;
			//   - what exit code drift uses. Reusing 1 and 2 by severity is the
			//     obvious choice, but it mixes "a check failed" with "something
			//     changed", and those are different questions.
			return notImplemented(cmd, "phase 4",
				"drift diffs two results.Reports; the diff types already exist in\n"+
					"internal/results/baseline.go (Change, DiffBaseline).")
		},
	}
	cmd.Flags().StringVar(&baseline, "baseline", "", "path to the saved baseline file")
	return cmd
}
