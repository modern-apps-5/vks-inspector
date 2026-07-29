package main

import (
	"github.com/spf13/cobra"
)

// newSnapshotCmd captures the current state as a baseline artifact.
//
// Stubbed in phase 1. The artifact format already exists and is already
// produced: a baseline is a results.Report with Kind=vksinspect.baseline/v1,
// written by results.WriteBaseline. So `snapshot` is `check` with Mode=snapshot
// and a different serialiser — not a new subsystem.
func newSnapshotCmd(g *globalOpts) *cobra.Command {
	var out string

	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Capture current state as a baseline artifact (phase 3)",
		Long: `Capture the current observed state as a baseline artifact that a later
` + "`drift`" + ` run can diff against.

Not implemented in phase 1. The artifact format is already fixed — see
internal/results/baseline.go and docs/ADR/0009-baseline-artifact.md.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// TODO(phase-3): run the engine with checks.ModeSnapshot and write
			// via results.WriteBaseline. Open questions to resolve first:
			//   - should snapshot refuse to run when any check errors, so a
			//     partial baseline is never mistaken for a complete one? The
			//     current inclination is yes, with --force to override.
			//   - a baseline captured without credentials is a *different kind*
			//     of baseline from one captured with them. It must be labelled
			//     as such, or drift will report every credentialed check as
			//     "newly appeared".
			return notImplemented(cmd, "phase 3",
				"a baseline is a results.Report with kind=vksinspect.baseline/v1;\n"+
					"the writer already exists in internal/results/baseline.go.")
		},
	}
	cmd.Flags().StringVar(&out, "out", "baseline.json", "path to write the baseline artifact")
	return cmd
}
