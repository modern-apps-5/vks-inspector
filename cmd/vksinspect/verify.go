package main

import (
	"github.com/spf13/cobra"
)

// newVerifyCmd checks a deployed environment against what was declared.
//
// Stubbed in phase 1. The wiring is deliberately the same as `check` — same
// config, same registry, same engine, different Mode — so that when it is
// switched on it gets every check written since then for free. Nothing about
// verify needs its own code path; if it turns out to, the check interface is
// wrong.
func newVerifyCmd(g *globalOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Check a live environment against the declared config (phase 2)",
		Long: `Check a deployed Supervisor / VKS environment against what --config declares:
what was actually built versus what was asked for.

Not built yet.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// TODO(phase-2): return runMode(cmd, g, checks.ModeVerify)
			//
			// Before switching this on, verify mode needs:
			//   - a --kubeconfig flag and a checks.CapSupervisor entry, so
			//     checks can read what the Supervisor actually got;
			//   - the credentialed clients (internal/clients/*), which are the
			//     only way to see what is actually there;
			//   - a decision, written up as an ADR, on whether a verify run
			//     that finds a *better* state than declared (a wider pool, a
			//     higher MTU) is a pass or a drift. Still open.
			return notImplemented(cmd, "phase 2",
				"verify will reuse `check`'s config, registry and engine with Mode=verify.\n"+
					"See docs/ADR/0002-mode-parametric-checks.md.")
		},
	}
	return cmd
}
