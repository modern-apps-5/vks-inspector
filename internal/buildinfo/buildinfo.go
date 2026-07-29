// Package buildinfo carries values stamped in at link time by the Makefile.
package buildinfo

import "github.com/modern-apps-5/vks-inspector/internal/results"

// Name is the binary name as reported in artifacts.
const Name = "vksinspect"

// Overridden via -ldflags -X. Defaults keep `go run` usable.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// Tool returns the identity block embedded in every report and baseline.
func Tool() results.ToolInfo {
	return results.ToolInfo{Name: Name, Version: Version, Commit: Commit}
}
