package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/modern-apps-5/vks-inspector/internal/checks"
	"github.com/modern-apps-5/vks-inspector/internal/checks/all"
	"github.com/modern-apps-5/vks-inspector/internal/config"
	"github.com/modern-apps-5/vks-inspector/internal/engine"
	"github.com/modern-apps-5/vks-inspector/internal/probes"
	"github.com/modern-apps-5/vks-inspector/internal/prompt"
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
		Short: "Preflight validation of an environment before Supervisor enablement",
		Long: `Check whether an environment is ready. Give it a vCenter endpoint and it will
ask what it needs to know, interrogate vCenter, and report.

  vksinspect check --vcenter vcenter.corp.local
  vksinspect check --config lab01.yaml            # non-interactive, for pipelines

Most of what is checked are Supervisor enablement prerequisites — there is no
VKS without a Supervisor. Use --layer to narrow.

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
	cfg, err := g.resolveConfig(cmd)
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
		Layer:    g.layer,
		Probes:   probes.System{},
		Invasive: g.invasive,
		Only:     g.only,
		Skip:     g.skip,
		Timeout:  g.timeout,
		// Clients are left nil until the vCenter client lands: every
		// credentialed check reports as a skip with a reason rather than
		// pretending to have inspected anything.
	})
	if err != nil {
		return err
	}
	return exitWith(g, rep)
}

// resolveConfig assembles the config from every source, in precedence order:
// an existing file, then flags, then whatever is still missing gets asked.
//
// The result is always a complete config.Config regardless of which path got
// there, which is what lets one command serve both an interactive first run and
// a pipeline. See internal/prompt.
func (g *globalOpts) resolveConfig(cmd *cobra.Command) (*config.Config, error) {
	cfg := &config.Config{APIVersion: config.APIVersion, Kind: config.Kind}

	if g.configPath != "" {
		loaded, err := config.Load(g.configPath)
		if err != nil {
			return nil, err
		}
		cfg = loaded
	}

	// Flags override the file. A flag is a more specific, more recent statement
	// of intent than a file that may have been written weeks ago.
	if g.vcenter != "" {
		cfg.Infrastructure.VCenter.FQDN = g.vcenter
		if cfg.Infrastructure.VCenter.CredentialRef == "" {
			cfg.Infrastructure.VCenter.CredentialRef = "vcenter"
		}
	}
	if g.topology != "" {
		t, err := config.ParseTopology(g.topology)
		if err != nil {
			return nil, err
		}
		cfg.Topology = t
	}

	// Nothing to interrogate and nothing declared is a usage error, not a
	// question to ask — the tool would have no idea what it was looking at.
	if g.configPath == "" && g.vcenter == "" && cfg.Infrastructure.VCenter.FQDN == "" {
		return nil, fmt.Errorf(
			"give the tool something to inspect: --vcenter <fqdn> to start interactively, " +
				"or --config <file> to run from a saved config")
	}

	interactive := !g.nonInteractive && isTTY(os.Stdin)
	p := prompt.New(os.Stdin, os.Stderr, interactive)

	// TODO(next): connect to vCenter here and populate Discovered before
	// eliciting, so the datacenter, cluster, VDS list and registered NSX
	// Manager are reported rather than asked for. The seam is in place and
	// Elicit already renders what it is given.
	var discovered *prompt.Discovered

	if err := prompt.Elicit(p, cfg, discovered); err != nil {
		return nil, err
	}

	// Validate what was assembled through exactly the same path a file takes.
	// An interactively-built config that a file-based run would have rejected
	// is a bug, not a convenience.
	if err := config.Validate(cfg); err != nil {
		return nil, err
	}

	if g.saveConfig != "" {
		if err := saveConfig(cfg, g.saveConfig); err != nil {
			return nil, err
		}
		fmt.Fprintf(os.Stderr, "\n  saved answers to %s\n  re-run non-interactively:  vksinspect %s --config %s\n\n",
			g.saveConfig, cmd.Name(), g.saveConfig)
	}
	return cfg, nil
}

// saveConfig writes the assembled config.
//
// It refuses to overwrite. An operator who has just answered twenty questions
// should not be able to destroy a colleague's config with a mistyped filename.
func saveConfig(cfg *config.Config, path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists; choose another path or remove it", path)
	}
	blob, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("serialise config: %w", err)
	}
	header := "# Generated by vksinspect. Contains no credentials — those come from\n" +
		"# the environment or a separate credentials file. See config/credentials.example.yaml\n"
	return os.WriteFile(path, append([]byte(header), blob...), 0o644)
}
