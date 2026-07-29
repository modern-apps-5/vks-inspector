package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/modern-apps-5/vks-inspector/internal/buildinfo"
	"github.com/modern-apps-5/vks-inspector/internal/config"
	"github.com/modern-apps-5/vks-inspector/internal/creds"
	"github.com/modern-apps-5/vks-inspector/internal/renderers"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// globalOpts are the flags shared by every mode. They are global precisely
// because every mode needs them: the moment one of these becomes a
// `check`-only flag, some internal package has started assuming preflight.
type globalOpts struct {
	configPath  string
	credsPath   string
	format      string
	output      string
	verbose     bool
	showSkipped bool
	noColour    bool
	invasive    bool
	only        []string
	skip        []string
	timeout     time.Duration
}

func (g *globalOpts) bind(cmd *cobra.Command) {
	f := cmd.PersistentFlags()
	f.StringVarP(&g.configPath, "config", "c", "", "path to the environment config YAML (required for all modes)")
	f.StringVar(&g.credsPath, "credentials", "", "path to a credentials YAML; env vars "+creds.EnvPrefix+"* override it")
	f.StringVarP(&g.format, "format", "f", "terminal", "output format: "+fmt.Sprint(renderers.Formats()))
	f.StringVarP(&g.output, "output", "o", "-", "write output to a file instead of stdout")
	f.BoolVarP(&g.verbose, "verbose", "v", false, "include evidence detail in human output")
	f.BoolVar(&g.showSkipped, "show-skipped", false, "show skipped checks in human output (always present in JSON)")
	f.BoolVar(&g.noColour, "no-color", false, "disable ANSI colour")
	f.BoolVar(&g.invasive, "invasive", false,
		"permit probes that may disturb the network (e.g. path-MTU discovery). Off by default; every invasive check is documented as such in docs/REQUIREMENTS-MATRIX.md")
	f.StringSliceVar(&g.only, "only", nil, "run only these check IDs, namespaces or categories")
	f.StringSliceVar(&g.skip, "skip", nil, "skip these check IDs, namespaces or categories")
	f.DurationVar(&g.timeout, "timeout", 60*time.Second, "per-check timeout")
}

// loadConfig is shared by every mode that needs one.
func (g *globalOpts) loadConfig() (*config.Config, error) {
	if g.configPath == "" {
		return nil, fmt.Errorf("--config is required; see config/example.yaml")
	}
	return config.Load(g.configPath)
}

// loadCreds never fails on absence. Missing credentials are a capability
// question, answered as skipped checks, not a startup error.
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
		Short: "Establish ground truth about VKS networking",
		Long: `vksinspect validates the networking underlying a VMware vSphere Kubernetes
Service (VKS) environment against a declared, topology-aware config.

It is read-only by default and makes no outbound internet calls. Every finding
carries the requirement it traces to, what was expected, what was observed, and
what to do about it.

Exit codes are contractual:
  0  all checks passed
  1  one or more blocker-severity checks failed
  2  warnings or indeterminate results only
  3  tool error — the run says nothing about the environment`,
		SilenceUsage:  true,
		SilenceErrors: false,
		Version:       fmt.Sprintf("%s (commit %s, built %s)", buildinfo.Version, buildinfo.Commit, buildinfo.Date),
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

// exitWith renders a report and terminates with the contractual exit code.
// Every mode that produces a report funnels through here so the exit-code
// contract has exactly one implementation.
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
// instead of recording a spurious pass.
func notImplemented(cmd *cobra.Command, phase, detail string) error {
	fmt.Fprintf(os.Stderr, "vksinspect %s: not implemented (planned for %s)\n", cmd.Name(), phase)
	if detail != "" {
		fmt.Fprintf(os.Stderr, "\n%s\n", detail)
	}
	os.Exit(results.ExitToolError)
	return nil
}
