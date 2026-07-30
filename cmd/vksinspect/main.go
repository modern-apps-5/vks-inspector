// Command vksinspect checks whether the networking under a VMware vSphere
// Kubernetes Service (VKS) environment is ready.
//
// Phase 1 does preflight only (`check`). The other subcommands are stubbed so
// that no package can build in an assumption that preflight is the only mode —
// see docs/ADR/0002-mode-parametric-checks.md.
package main

import (
	"fmt"
	"os"

	"github.com/modern-apps-5/vks-inspector/internal/results"
)

func main() {
	// Exit codes are fixed and belong to results.ExitCode. main's only job is
	// to make sure every path out of the program goes through one.
	if err := newRootCmd().Execute(); err != nil {
		// Cobra has already printed the error for usage problems; anything
		// reaching here is a tool error, never an environment verdict.
		fmt.Fprintln(os.Stderr, "vksinspect:", err)
		os.Exit(results.ExitToolError)
	}
}
