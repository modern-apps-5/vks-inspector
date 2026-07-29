package main

import (
	"github.com/spf13/cobra"
)

// newVerifyCmd is post-deploy verification: actual versus declared.
//
// Stubbed in phase 1. The wiring is deliberately identical to `check` — same
// config, same registry, same engine, different Mode — so that when it is
// enabled it inherits every check written since. Nothing about verify needs a
// parallel code path; if it turns out to need one, the check interface is wrong.
func newVerifyCmd(g *globalOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Post-deploy verification of a live environment against the declared config (phase 2)",
		Long: `Verify a deployed Supervisor / VKS environment against the intent declared in
--config: what was actually built versus what was asked for.

Not implemented in phase 1.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// TODO(phase-2): return runMode(cmd, g, checks.ModeVerify)
			//
			// Before enabling, verify mode needs:
			//   - a --kubeconfig flag and a checks.CapSupervisor capability, so
			//     checks can read what the Supervisor actually got;
			//   - credentialed clients (internal/clients/*), which are what make
			//     "actual" observable at all;
			//   - a decision, recorded as an ADR, on whether a verify run that
			//     finds a *better* state than declared (a wider pool, a higher
			//     MTU) is a pass or a drift. It is currently unresolved.
			return notImplemented(cmd, "phase 2",
				"verify will reuse `check`'s config, registry and engine with Mode=verify.\n"+
					"See docs/ADR/0002-mode-parametric-checks.md.")
		},
	}
	return cmd
}
