package main

import (
	"github.com/spf13/cobra"

	"github.com/modern-apps-5/vks-inspector/internal/checks"
	"github.com/modern-apps-5/vks-inspector/internal/checks/all"
	"github.com/modern-apps-5/vks-inspector/internal/engine"
	"github.com/modern-apps-5/vks-inspector/internal/probes"
)

// newCheckCmd is preflight: grade the declared intent against what can be
// observed before anything is deployed.
//
// This is the only mode implemented in phase 1, and it is deliberately thin —
// almost everything here is shared with verify and snapshot. If this function
// starts growing preflight-specific logic, that logic belongs in a check.
func newCheckCmd(g *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Preflight validation against the intended config",
		Long: `Run preflight checks against the environment described by --config, before
any Supervisor or VKS deployment starts.

Read-only. Non-invasive unless --invasive is given.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runMode(cmd, g, checks.ModePreflight)
		},
	}
}

// runMode is the shared body of check / verify / snapshot. Having one body is
// the enforcement mechanism for "the same check unit runs in every mode".
func runMode(cmd *cobra.Command, g *globalOpts, mode checks.Mode) error {
	cfg, err := g.loadConfig()
	if err != nil {
		return err
	}
	credSet, err := g.loadCreds()
	if err != nil {
		return err
	}

	rep, err := engine.Run(cmd.Context(), all.Registry(), engine.Options{
		Mode:     mode,
		Config:   cfg,
		Creds:    credSet,
		Probes:   probes.System{},
		Invasive: g.invasive,
		Only:     g.only,
		Skip:     g.skip,
		Timeout:  g.timeout,
		// Clients are left nil in phase 1: no credentialed client is
		// implemented, so every credentialed check reports as a skip with a
		// reason rather than pretending to have inspected anything.
	})
	if err != nil {
		return err
	}
	return exitWith(g, rep)
}
