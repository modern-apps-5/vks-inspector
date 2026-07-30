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
		Long: `Explain a check or requirement: what it checks, which requirement rows it
comes from, which topologies it applies to, and what to do when it fails.

With no argument, lists everything this build knows about.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reg := all.Registry()

			if len(args) == 0 {
				fmt.Fprintf(os.Stdout, "Supported topologies (networking + load balancer):\n")
				for _, t := range config.SupportedCombinations() {
					line := fmt.Sprintf("  %-16s %s", t.String(), t.Description())
					if note := t.Note(); note != "" {
						line += "\n                   ⚑ " + note
					}
					fmt.Fprintln(os.Stdout, line)
				}
				fmt.Fprintf(os.Stdout, "\nChecks in this build (%d):\n", reg.Len())
				for _, c := range reg.All() {
					m := c.Meta()
					fmt.Fprintf(os.Stdout, "  %-28s %-8s %s\n", m.ID, m.Severity, m.Title)
				}
				fmt.Fprintf(os.Stdout,
					"\nMost requirements have no check yet. The full list is\ndocs/REQUIREMENTS-MATRIX.md.\n")
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
				fmt.Fprintf(os.Stdout, "  layer         %s\n", m.EffectiveLayer())
				fmt.Fprintf(os.Stdout, "  modes         %v\n", m.Modes)
				fmt.Fprintf(os.Stdout, "  applies to    %s\n", m.Applies.Describe())
				if m.Invasive {
					fmt.Fprintf(os.Stdout, "  invasive      yes — requires --invasive\n")
				}
				if m.Remediation != "" {
					fmt.Fprintf(os.Stdout, "\n  how to fix it\n    %s\n", m.Remediation)
				}
				fmt.Fprintln(os.Stdout)
			}

			if !found {
				// TODO(phase-2): fall back to the requirements matrix. That
				// needs docs/REQUIREMENTS-MATRIX.md in a machine-readable form
				// (a YAML sidecar, embedded with go:embed) rather than the
				// markdown table it is today. Deliberately not done yet: the
				// matrix rows are still being confirmed against product docs
				// and a lab, and shipping unconfirmed content inside the binary
				// would make it look more settled than it is.
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
