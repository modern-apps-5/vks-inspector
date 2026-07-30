package main

import (
	"github.com/spf13/cobra"
)

// newServeCmd serves the local web UI.
//
// Stubbed in phase 1. The decision that matters is already made and does not
// need this command to exist: the UI just reads JSON and gets no special access
// to the checks. It reads the same results.Report the --format json renderer
// produces. There is no "UI mode" on a check, and there will not be one.
func newServeCmd(g *globalOpts) *cobra.Command {
	var addr string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the local web UI (phase 5)",
		Long: `Serve a local web UI over the same reports the CLI produces.

Not built yet.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// TODO(phase-5): embed assets with go:embed (the single-binary
			// constraint means no external asset directory) and serve:
			//   - the report JSON, unchanged;
			//   - the requirements matrix;
			//   - a trigger endpoint for a run.
			// The rules that carry over: listen on loopback by default, no
			// internet calls (no CDN fonts, no analytics), and the UI never
			// receives credentials — it asks for runs, the binary holds the
			// secrets.
			return notImplemented(cmd, "phase 5",
				"the UI just reads results.Report as JSON, like anything else;\n"+
					"see docs/ADR/0004-pluggable-renderers.md.")
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:8080", "listen address (loopback by default)")
	return cmd
}
