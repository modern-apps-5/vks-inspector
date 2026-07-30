// Package all wires every check package into a registry.
//
// Registration is explicit rather than via init() side effects. An init()-based
// registry means "which checks exist" depends on which packages happen to be
// imported, which is exactly the kind of thing that silently drops a check from
// a release build. If a check is not listed here, it does not exist.
package all

import (
	"github.com/modern-apps-5/vks-inspector/internal/checks"
	"github.com/modern-apps-5/vks-inspector/internal/checks/alb"
	"github.com/modern-apps-5/vks-inspector/internal/checks/configval"
	"github.com/modern-apps-5/vks-inspector/internal/checks/flb"
	"github.com/modern-apps-5/vks-inspector/internal/checks/network"
	"github.com/modern-apps-5/vks-inspector/internal/checks/nsx"
	"github.com/modern-apps-5/vks-inspector/internal/checks/reference"
	"github.com/modern-apps-5/vks-inspector/internal/checks/vcenter"
	"github.com/modern-apps-5/vks-inspector/internal/registry"
)

// Registry builds the registry for a run.
func Registry() *registry.Registry {
	r := registry.New()

	// Order of the groups below is documentation only; the registry sorts by ID.
	var set []checks.Check
	set = append(set, configval.Checks()...) // class (c) — pure config arithmetic
	set = append(set, network.Checks()...)   // class (a) — network-only
	set = append(set, vcenter.Checks()...)   // class (b) — credentialed
	set = append(set, nsx.Checks()...)       // class (b) — credentialed
	set = append(set, alb.Checks()...)       // class (b) — credentialed
	set = append(set, flb.Checks()...)       // class (b) — vCenter-credentialed, no dedicated controller

	// The reference check is told how many checks the build knows about, which
	// is only knowable here. +1 for itself.
	set = append(set, &reference.TopologyRecognised{KnownCheckCount: len(set) + 1})

	r.Register(set...)
	return r
}
