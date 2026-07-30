package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/modern-apps-5/vks-inspector/internal/buildinfo"
	"github.com/modern-apps-5/vks-inspector/internal/creds"
	"github.com/modern-apps-5/vks-inspector/internal/renderers"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// globalOpts are the flags shared by every mode. They are shared precisely
// because every mode needs them: the moment one becomes a `check`-only flag,
// some package has started assuming preflight is the only caller.
type globalOpts struct {
	configPath string
	credsPath  string
	// vcenter is the entry point. Everything else the tool can discover from
	// it, it should — see docs/ADR/0011-vcenter-first-discovery.md.
	vcenter        string
	topology       string
	layer          results.Layer
	layerFlag      string
	nonInteractive bool
	useDefaults    bool
	saveConfig     string
	forceOverwrite bool
	insecureTLS    bool
	relogin        bool
	format         string
	output         string
	verbose        bool
	showSkipped    bool
	noColour       bool
	invasive       bool
	only           []string
	skip           []string
	timeout        time.Duration
	probeTimeout   time.Duration
}

func (g *globalOpts) bind(cmd *cobra.Command) {
	f := cmd.PersistentFlags()
	f.StringVar(&g.vcenter, "vcenter", "", "vCenter FQDN or address — the starting point; everything else is found from it")
	f.StringVarP(&g.configPath, "config", "c", "", "path to a saved environment config YAML; anything missing from it is prompted for")
	f.StringVar(&g.topology, "topology", "", "topology as networking+loadBalancer, e.g. nsx+alb (skips those two prompts)")
	f.StringVar(&g.layerFlag, "layer", "both", "which prerequisites to check: supervisor | vks | both")
	f.BoolVar(&g.nonInteractive, "non-interactive", false,
		"never ask questions; anything missing is an error that names the config field it belongs in")
	f.BoolVar(&g.useDefaults, "defaults", false,
		"take each prompt's example answer when you press Enter. FOR TRYING OUT THE CLI ONLY — "+
			"the answers describe no real environment, but the checks still run and may report PASS")
	f.StringVar(&g.saveConfig, "save-config", "", "write the config it assembled to this path, so later runs need no prompting")
	f.BoolVar(&g.forceOverwrite, "force", false, "allow --save-config to overwrite an existing file")
	f.BoolVar(&g.relogin, "relogin", false,
		"ignore any stored credentials and ask again — use this when a saved password is wrong")
	f.BoolVar(&g.insecureTLS, "insecure-skip-tls-verify", false,
		"do not verify management-plane TLS certificates. Needed for the self-signed certs common "+
			"in labs. Certificate checks for those endpoints then skip with a reason, because a "+
			"connection that skipped verification cannot prove the certificate chain is good")
	f.StringVar(&g.credsPath, "credentials", "", "path to a credentials YAML; env vars "+creds.EnvPrefix+"* override it")
	f.StringVarP(&g.format, "format", "f", "terminal", "output format: "+fmt.Sprint(renderers.Formats()))
	f.StringVarP(&g.output, "output", "o", "-", "write output to a file instead of stdout")
	f.BoolVarP(&g.verbose, "verbose", "v", false, "show the supporting detail behind each result")
	f.BoolVar(&g.showSkipped, "show-skipped", false, "show skipped checks in human output (always present in JSON)")
	f.BoolVar(&g.noColour, "no-color", false, "disable ANSI colour")
	f.BoolVar(&g.invasive, "invasive", false,
		"allow probes that may disturb the network (path-MTU discovery, for one). Off by default; every one of these is marked in docs/REQUIREMENTS-MATRIX.md")
	f.StringSliceVar(&g.only, "only", nil, "run only these check IDs, namespaces or categories")
	f.StringSliceVar(&g.skip, "skip", nil, "skip these check IDs, namespaces or categories")
	f.DurationVar(&g.timeout, "timeout", 60*time.Second,
		"time limit for a whole check, which may cover many targets")
	f.DurationVar(&g.probeTimeout, "probe-timeout", 5*time.Second,
		"time limit for one DNS lookup, TCP connect or NTP query")
}

// resolveLayer validates --layer. Done once, up front, so a bad value fails
// before the operator answers twenty questions.
func (g *globalOpts) resolveLayer() error {
	l := results.Layer(g.layerFlag)
	if g.layerFlag == "" {
		l = results.LayerBoth
	}
	if !l.Valid() {
		return fmt.Errorf("--layer %q must be one of %v", g.layerFlag, results.AllLayers)
	}
	g.layer = l
	return nil
}

// loadCreds never fails when there are none. Missing credentials are a question
// about access, answered as skipped checks, not a startup error.
func (g *globalOpts) loadCreds() (*creds.Set, error) {
	return creds.Load(g.credsPath)
}

func (g *globalOpts) renderer() (renderers.Renderer, error) {
	colour := !g.noColour && os.Getenv("NO_COLOR") == "" && isTTY(os.Stdout)
	return renderers.New(g.format, renderers.Options{
		Colour:      colour,
		Verbose:     g.verbose,
		ShowSkipped: g.showSkipped,
	})
}

// writer returns the output stream and a closer.
func (g *globalOpts) writer() (io.Writer, func() error, error) {
	if g.output == "" || g.output == "-" {
		return os.Stdout, func() error { return nil }, nil
	}
	f, err := os.Create(g.output)
	if err != nil {
		return nil, nil, fmt.Errorf("open output: %w", err)
	}
	return f, f.Close, nil
}

func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func newRootCmd() *cobra.Command {
	g := &globalOpts{}

	root := &cobra.Command{
		Use:   "vksinspect",
		Short: "Check whether VKS networking is ready",
		Long: `vksinspect checks the networking under a VMware vSphere Kubernetes Service
(VKS) environment against a config describing what you intend to deploy.

It is read-only by default and never calls out to the internet. Every finding
names the requirement it comes from, what was expected, what was actually seen,
and what to change.

Exit codes are fixed — pipelines rely on them:
  0  all checks passed
  1  one or more blocker-severity checks failed
  2  only warnings, or checks that could not tell
  3  tool error — the run says nothing about the environment`,
		SilenceUsage:  true,
		SilenceErrors: false,
		PersistentPreRunE: func(*cobra.Command, []string) error {
			return g.resolveLayer()
		},
		Version: fmt.Sprintf("%s (commit %s, built %s)", buildinfo.Version, buildinfo.Commit, buildinfo.Date),
	}
	g.bind(root)

	root.AddCommand(
		newCheckCmd(g),
		newVerifyCmd(g),
		newSnapshotCmd(g),
		newDriftCmd(g),
		newExplainCmd(g),
		newServeCmd(g),
	)
	return root
}

// exitWith renders a report and exits with the right code. Every mode that
// produces a report goes through here, so the exit codes have exactly one
// implementation.
func exitWith(g *globalOpts, rep *results.Report) error {
	r, err := g.renderer()
	if err != nil {
		return err
	}
	w, closeFn, err := g.writer()
	if err != nil {
		return err
	}
	if err := r.Render(w, rep); err != nil {
		_ = closeFn()
		return fmt.Errorf("render: %w", err)
	}
	if err := closeFn(); err != nil {
		return fmt.Errorf("close output: %w", err)
	}
	os.Exit(results.ExitCode(rep.Results))
	return nil
}

// notImplemented is the standard stub response. It exits 3 (tool error) rather
// than 0, so a pipeline that calls a phase-2 command by mistake fails loudly
// instead of recording a pass that never happened.
func notImplemented(cmd *cobra.Command, phase, detail string) error {
	fmt.Fprintf(os.Stderr, "vksinspect %s: not implemented (planned for %s)\n", cmd.Name(), phase)
	if detail != "" {
		fmt.Fprintf(os.Stderr, "\n%s\n", detail)
	}
	os.Exit(results.ExitToolError)
	return nil
}
