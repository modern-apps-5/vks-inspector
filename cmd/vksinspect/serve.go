package main

import (
	"github.com/spf13/cobra"
)

// newServeCmd serves the local web UI.
//
// Stubbed in phase 1. The important architectural commitment is already made
// and does not need this command to exist: the UI is a JSON consumer with no
// privileged access to the check layer. It reads the same results.Report the
// --format json renderer produces. There is no "UI mode" for a check, and
// there will not be one.
func newServeCmd(g *globalOpts) *cobra.Command {
	var addr string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the local web UI (phase 5)",
		Long: `Serve a local web UI over the same reports the CLI produces.

Not implemented in phase 1.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// TODO(phase-5): embed assets with go:embed (the single-binary
			// constraint means no external asset directory) and serve:
			//   - the report JSON, unchanged;
			//   - the requirements matrix;
			//   - a trigger endpoint for a run.
			// Constraints that carry over: bind to loopback by default, no
			// outbound internet calls (no CDN fonts, no analytics), and the
			// UI must never receive credentials — it triggers runs, the binary
			// holds the secrets.
			return notImplemented(cmd, "phase 5",
				"the UI is just another JSON consumer of results.Report;\n"+
					"see docs/ADR/0004-pluggable-renderers.md.")
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:8080", "listen address (loopback by default)")
	return cmd
}
