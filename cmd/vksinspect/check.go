package main

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/modern-apps-5/vks-inspector/internal/checks"
	"github.com/modern-apps-5/vks-inspector/internal/checks/all"
	"github.com/modern-apps-5/vks-inspector/internal/clients"
	vcenterclient "github.com/modern-apps-5/vks-inspector/internal/clients/vcenter"
	"github.com/modern-apps-5/vks-inspector/internal/config"
	"github.com/modern-apps-5/vks-inspector/internal/creds"
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
		Short: "Check an environment before enabling the Supervisor",
		Long: `Check whether an environment is ready. Give it a vCenter endpoint and it will
ask for what it needs, read what it can from vCenter, and report.

  vksinspect check --vcenter vcenter.corp.local
  vksinspect check --config lab01.yaml            # asks nothing, for pipelines

Most of what it checks is what the Supervisor needs — there is no VKS without a
Supervisor. Use --layer to narrow that.

Read-only, and it disturbs nothing unless you pass --invasive.`,
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
	// Apply --insecure-skip-tls-verify to the set the CHECKS see, not just to
	// the copy handed to the client. Without this the certificate checks never
	// learn that verification was disabled and go on vouching for a chain
	// nobody verified.
	if g.insecureTLS {
		credSet.SetInsecureAll()
	}

	// Connect what we can. A client that fails to build yields a missing
	// capability, which yields skipped checks with a reason — never a failed
	// check. "We could not log in" is a statement about the tool's access, not
	// about the customer's environment.
	clientSet, closers := g.buildClients(cmd.Context(), cfg, credSet)
	defer func() {
		for _, closeFn := range closers {
			_ = closeFn(context.WithoutCancel(cmd.Context()))
		}
	}()

	rep, err := engine.Run(cmd.Context(), all.Registry(), engine.Options{
		Mode:    mode,
		Config:  cfg,
		Creds:   credSet,
		Layer:   g.layer,
		Clients: clientSet,
		// Memoised: several checks legitimately need the same observation
		// (dns.forward and dns.resolver-agreement resolve the same names;
		// tls.chain and tls.expiry inspect the same certificates). Without
		// this a run probes every target twice.
		Probes:      probes.NewMemo(probes.System{Timeout: g.probeTimeout}),
		Invasive:    g.invasive,
		InsecureTLS: g.insecureTLS,
		Only:        g.only,
		Skip:        g.skip,
		Timeout:     g.timeout,
	})
	if err != nil {
		return err
	}
	return exitWith(g, rep)
}

// buildClients connects the management-plane clients the config calls for.
//
// Failures are reported and swallowed, never fatal: a run without vCenter
// credentials must still grade the address plan and report the vCenter checks
// as skips. Returns the client set plus closers, because a session left open on
// a customer's vCenter is litter.
func (g *globalOpts) buildClients(ctx context.Context, cfg *config.Config, credSet *creds.Set) (checks.Clients, []func(context.Context) error) {
	var set checks.Clients
	var closers []func(context.Context) error

	endpoint := cfg.Infrastructure.VCenter.FQDN
	if endpoint == "" {
		return set, closers
	}

	ref := cfg.Infrastructure.VCenter.CredentialRef
	if ref == "" {
		ref = "vcenter"
	}
	if credSet.MissingFile != "" {
		// Absent is normal before the first save, but it is also what a typo'd
		// path looks like. Say which file, so the operator can tell.
		fmt.Fprintf(os.Stderr, "\n  note: %s does not exist yet\n", credSet.MissingFile)
	}
	opts := clients.DefaultOptions()
	opts.Timeout = g.timeout

	// Try, and on an authentication failure offer to re-enter. A wrong stored
	// password is otherwise a dead end: the tool loads it, fails, and never
	// asks again — which is precisely the trap this loop exists to avoid.
	for attempt := 0; attempt < 3; attempt++ {
		cred, ok := credSet.Get(ref)
		if ok && cred.Username == "" && cred.Token == "" {
			ok = false // an entry with no principal is not a usable credential
		}
		if !ok || g.relogin {
			asked, err := g.askForCredentials(endpoint, ref, credSet)
			if err != nil {
				fmt.Fprintf(os.Stderr,
					"\n  ⚠ no credentials for %s — vCenter checks will be skipped.\n"+
						"    Set %sVCENTER_USERNAME / %sVCENTER_PASSWORD, pass --credentials,\n"+
						"    or re-run interactively to be prompted.\n\n",
					endpoint, creds.EnvPrefix, creds.EnvPrefix)
				return set, closers
			}
			cred = asked
			g.relogin = false
		}
		if g.insecureTLS {
			cred.InsecureSkipVerify = true
		}

		c := vcenterclient.New(endpoint, cred, opts)
		err := c.Connect(ctx)
		if err == nil {
			set.VCenter = c
			closers = append(closers, c.Close)
			return set, closers
		}

		// Reported by vc.api-reachable too, but saying it here means the
		// operator sees it before the report scrolls — it is the reason for
		// five skips.
		fmt.Fprintf(os.Stderr, "\n  ⚠ could not connect to %s:\n     %v\n", endpoint, err)

		if !isAuthFailure(err) || g.nonInteractive || !isTTY(os.Stdin) {
			fmt.Fprintf(os.Stderr, "    vCenter checks will be skipped.\n\n")
			return set, closers
		}

		p := prompt.New(os.Stdin, os.Stderr, true)
		again, perr := p.Confirm("The credentials were rejected. Enter them again?", true)
		if perr != nil || !again {
			fmt.Fprintf(os.Stderr, "    vCenter checks will be skipped.\n\n")
			return set, closers
		}
		g.relogin = true
	}

	fmt.Fprintf(os.Stderr, "    vCenter checks will be skipped.\n\n")
	return set, closers
}

// isAuthFailure reports whether an error is the server rejecting credentials,
// as opposed to the endpoint being unreachable. Re-prompting for a password
// helps in the first case and wastes the operator's time in the second.
func isAuthFailure(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, s := range []string{
		"incorrect user name or password",
		"cannot complete login",
		"invalid credentials",
		"authenticate to vcenter",
		"permission to perform this operation was denied",
	} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// askForCredentials prompts for a username and password, and offers to save
// them.
//
// The secret never enters the config, never reaches a report or a baseline, and
// is written only to a 0600 file the operator explicitly agrees to. Saving is
// opt-in: writing someone's password to disk without asking is not a decision
// this tool gets to make for them.
func (g *globalOpts) askForCredentials(endpoint, ref string, set *creds.Set) (creds.Credential, error) {
	if g.nonInteractive || !isTTY(os.Stdin) {
		return creds.Credential{}, fmt.Errorf("cannot prompt for credentials")
	}

	p := prompt.New(os.Stdin, os.Stderr, true)
	// --defaults applies here too, so Enter accepts the suggested username. The
	// password can never have a default and is always typed.
	p.UseExamples = g.useDefaults
	p.Section("Credentials for " + endpoint)
	if _, ok := set.Get(ref); ok {
		p.Info("Replacing the stored credentials for this endpoint.")
	} else {
		p.Info("Not found in the environment or a credentials file.")
	}
	p.Info("A read-only account is enough — this tool performs no writes.")
	if _, err := netip.ParseAddr(hostOf(endpoint)); err == nil {
		p.Info("")
		p.Info("%s Note: this endpoint is an IP address, not a hostname.", "⚑")
		p.Info("  Certificate validation compares against the address you connect to,")
		p.Info("  so it will fail even with a perfectly good certificate. Declaring the")
		p.Info("  FQDN instead avoids that.")
	}

	user, err := p.Ask("Username", "readonly@vsphere.local", "", nil)
	if err != nil {
		return creds.Credential{}, err
	}
	pass, err := p.Password("Password (not echoed)")
	if err != nil {
		return creds.Credential{}, err
	}

	cred := creds.Credential{Username: user, Password: pass}
	if existing, ok := set.Get(ref); ok && existing.InsecureSkipVerify {
		cred.InsecureSkipVerify = true // keep a prior decision about verification
	}

	// Self-signed management-plane certificates are the norm in labs, and a
	// tool that simply refuses to connect to them is useless there. Ask, rather
	// than making the operator discover a flag.
	if g.insecureTLS {
		cred.InsecureSkipVerify = true
	} else {
		skip, err := p.Confirm(
			"Skip TLS certificate verification for this endpoint? (needed for self-signed certs)", false)
		if err == nil && skip {
			cred.InsecureSkipVerify = true
			p.Info("Certificate checks for %s will be reported as informational —", endpoint)
			p.Info("an unverified connection cannot evidence a valid chain.")
		}
	}
	set.Put(ref, cred)

	save, err := p.Confirm("Save these credentials (replacing any stored copy)?", true)
	if err != nil || !save {
		return cred, nil
	}

	path := g.credsPath
	if path == "" {
		if path, err = creds.DefaultPath(); err != nil {
			p.Info("could not work out where to save: %v", err)
			return cred, nil
		}
	}
	if err := set.Save(path); err != nil {
		// Failing to save must not lose the credentials for this run.
		p.Info("could not save: %v", err)
		return cred, nil
	}
	p.Info("saved to %s (mode 0600)", path)
	p.Info("Re-runs will pick it up automatically. Never commit that file.")
	return cred, nil
}

// hostOf strips any port from an endpoint string.
func hostOf(endpoint string) string {
	if h, _, err := net.SplitHostPort(endpoint); err == nil {
		return h
	}
	return endpoint
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
	p.UseExamples = g.useDefaults

	// Discover before asking, so anything vCenter already knows is reported
	// rather than typed. Best-effort: any failure falls back to asking, and
	// never aborts the run. See docs/ADR/0014.
	discovered := g.discover(cmd.Context(), cfg)

	if err := prompt.Elicit(p, cfg, discovered); err != nil {
		return nil, err
	}

	// Validate what was assembled through exactly the same path a file takes.
	// An interactively-built config that a file-based run would have rejected
	// is a bug, not a convenience.
	if err := config.Validate(cfg); err != nil {
		return nil, err
	}

	// A config built from placeholder answers stays marked for its whole life.
	// Someone re-running a saved file weeks later has no idea which flag
	// produced it, so the file has to say so itself.
	if cfg.FromPlaceholders() {
		fmt.Fprintf(os.Stderr,
			"\n  ⚠ this config was built from placeholder answers (--defaults).\n"+
				"    Results describe the example addresses, not a real environment.\n\n")
	}

	if g.saveConfig != "" {
		if err := g.writeConfig(cfg, p); err != nil {
			return nil, err
		}
		fmt.Fprintf(os.Stderr, "\n  saved answers to %s\n  re-run non-interactively:  vksinspect %s --config %s\n\n",
			g.saveConfig, cmd.Name(), g.saveConfig)
	}
	return cfg, nil
}

// discover reads what it can from vCenter to pre-fill the question flow.
//
// Returns nil on any failure. Discovery is a convenience; a tool that cannot
// run without it has made a network call load-bearing for a config-only check.
func (g *globalOpts) discover(ctx context.Context, cfg *config.Config) *prompt.Discovered {
	if cfg.Infrastructure.VCenter.FQDN == "" || g.nonInteractive {
		return nil
	}
	credSet, err := g.loadCreds()
	if err != nil {
		return nil
	}
	ref := cfg.Infrastructure.VCenter.CredentialRef
	if ref == "" {
		ref = "vcenter"
	}
	cred, ok := credSet.Get(ref)
	if !ok {
		return nil
	}
	if g.insecureTLS {
		cred.InsecureSkipVerify = true
	}

	opts := clients.DefaultOptions()
	opts.Timeout = g.timeout
	c := vcenterclient.New(cfg.Infrastructure.VCenter.FQDN, cred, opts)
	if err := c.Connect(ctx); err != nil {
		return nil
	}
	defer func() { _ = c.Close(ctx) }()

	inv, err := c.Discover(ctx)
	if err != nil {
		return nil
	}

	d := &prompt.Discovered{
		VCenterVersion: inv.Version,
		Switches:       inv.Switches,
		NSXManager:     inv.NSXManager,
		Hosts:          inv.HostCount,
	}
	// Fill the config only where the operator has not already decided. A
	// discovered value must never overwrite a declared one.
	if len(inv.Datacenters) == 1 {
		d.Datacenter = inv.Datacenters[0]
		if cfg.VSphere.Datacenter == "" {
			cfg.VSphere.Datacenter = inv.Datacenters[0]
		}
	}
	if len(inv.Clusters) == 1 {
		d.Cluster = inv.Clusters[0]
		if cfg.VSphere.Cluster == "" {
			cfg.VSphere.Cluster = inv.Clusters[0]
		}
	}
	if len(inv.Switches) == 1 && cfg.VSphere.DistributedSwitch == "" {
		cfg.VSphere.DistributedSwitch = inv.Switches[0]
	}
	return d
}

// writeConfig writes the assembled config, asking before it overwrites.
//
// Overwriting silently would let someone destroy a colleague's config with a
// mistyped filename; refusing outright made re-running the wizard needlessly
// painful. So: --force overwrites, an interactive run asks, and a
// non-interactive run without --force still refuses.
func (g *globalOpts) writeConfig(cfg *config.Config, p *prompt.Prompter) error {
	path := g.saveConfig
	if _, err := os.Stat(path); err == nil && !g.forceOverwrite {
		ok, perr := p.Confirm(fmt.Sprintf("%s already exists. Overwrite it?", path), false)
		if perr != nil || !ok {
			return fmt.Errorf("%s already exists; re-run with --force, or choose another path", path)
		}
	}
	return saveConfig(cfg, path)
}

// saveConfig writes the assembled config.
func saveConfig(cfg *config.Config, path string) error {
	blob, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("serialise config: %w", err)
	}
	header := "# Generated by vksinspect. Contains no credentials — those come from\n" +
		"# the environment or a separate credentials file. See config/credentials.example.yaml\n"
	if cfg.FromPlaceholders() {
		header = "# !! BUILT FROM PLACEHOLDER ANSWERS (--defaults) !!\n" +
			"# These addresses are illustrative examples, not a real environment.\n" +
			"# Any report produced from this file is meaningless as a readiness assessment.\n" + header
	}
	return os.WriteFile(path, append([]byte(header), blob...), 0o644)
}
