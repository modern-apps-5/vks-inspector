package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/modern-apps-5/vks-inspector/internal/checks/all"
	"github.com/modern-apps-5/vks-inspector/internal/config"
)

// newExplainCmd prints why a requirement exists and how to satisfy it.
//
// Partially implemented in phase 1: it can already explain any registered check
// and list the topologies, because that information lives in Meta and would
// otherwise rot. What it cannot yet do is explain a requirement that has no
// check behind it — that needs the requirements matrix in machine-readable
// form, which is a deliberate phase-2 decision (see the TODO below).
func newExplainCmd(g *globalOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "explain [check-id | requirement-id | topology]",
		Short: "Explain why a requirement exists and how to satisfy it",
		Long: `Explain a check or requirement: what it asserts, which requirement rows it
traces to, which topologies it applies to, and what to do when it fails.

With no argument, lists everything this build knows about.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reg := all.Registry()

			if len(args) == 0 {
				fmt.Fprintf(os.Stdout, "Topologies:\n")
				for _, t := range config.AllTopologies {
					fmt.Fprintf(os.Stdout, "  %-12s %s\n", t, t.Description())
				}
				fmt.Fprintf(os.Stdout, "\nChecks in this build (%d):\n", reg.Len())
				for _, c := range reg.All() {
					m := c.Meta()
					fmt.Fprintf(os.Stdout, "  %-28s %-8s %s\n", m.ID, m.Severity, m.Title)
				}
				fmt.Fprintf(os.Stdout,
					"\nMost requirements have no check yet. The authoritative list is\ndocs/REQUIREMENTS-MATRIX.md.\n")
				return nil
			}

			query := strings.ToLower(args[0])
			var found bool
			for _, c := range reg.All() {
				m := c.Meta()
				if strings.ToLower(m.ID) != query && !containsFold(m.RequirementIDs, query) {
					continue
				}
				found = true
				fmt.Fprintf(os.Stdout, "%s\n  %s\n\n", m.ID, m.Title)
				fmt.Fprintf(os.Stdout, "  category      %s\n", m.Category)
				fmt.Fprintf(os.Stdout, "  severity      %s\n", m.Severity)
				fmt.Fprintf(os.Stdout, "  requirements  %s\n", strings.Join(m.RequirementIDs, ", "))
				fmt.Fprintf(os.Stdout, "  modes         %v\n", m.Modes)
				if len(m.Topologies) == 0 {
					fmt.Fprintf(os.Stdout, "  topologies    all\n")
				} else {
					fmt.Fprintf(os.Stdout, "  topologies    %v\n", m.Topologies)
				}
				if m.Invasive {
					fmt.Fprintf(os.Stdout, "  invasive      yes — requires --invasive\n")
				}
				if m.Remediation != "" {
					fmt.Fprintf(os.Stdout, "\n  remediation\n    %s\n", m.Remediation)
				}
				fmt.Fprintln(os.Stdout)
			}

			if !found {
				// TODO(phase-2): fall back to the requirements matrix. That
				// needs docs/REQUIREMENTS-MATRIX.md in a machine-readable form
				// (a YAML sidecar, embedded with go:embed) rather than the
				// markdown table it is today. Deliberately not done yet: the
				// matrix rows are still being confirmed against product docs
				// and a lab, and embedding unverified content would give it an
				// authority it has not earned.
				return fmt.Errorf("nothing known about %q in this build; see docs/REQUIREMENTS-MATRIX.md", args[0])
			}
			return nil
		},
	}
	return cmd
}

func containsFold(haystack []string, needle string) bool {
	for _, h := range haystack {
		if strings.EqualFold(h, needle) {
			return true
		}
	}
	return false
}
