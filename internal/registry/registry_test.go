package registry_test

import (
	"context"
	"testing"

	"github.com/modern-apps-5/vks-inspector/internal/checks"
	"github.com/modern-apps-5/vks-inspector/internal/config"
	"github.com/modern-apps-5/vks-inspector/internal/registry"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

type stub struct{ meta checks.Meta }

func (s stub) Meta() checks.Meta { return s.meta }
func (s stub) Run(context.Context, *checks.RunContext) ([]results.Result, error) {
	return []results.Result{{CheckID: s.meta.ID, Status: results.StatusPass}}, nil
}

func meta(id string, mods ...func(*checks.Meta)) checks.Meta {
	m := checks.Meta{
		ID:             id,
		Title:          id,
		RequirementIDs: []string{"TEST-001"},
		Category:       results.CategoryMeta,
		Severity:       results.SeverityWarning,
		Modes:          checks.AllModes,
	}
	for _, f := range mods {
		f(&m)
	}
	return m
}

func TestSelect(t *testing.T) {
	t.Parallel()

	reg := registry.New()
	reg.Register(
		stub{meta("dns.forward", func(m *checks.Meta) { m.Category = results.CategoryDNS })},
		stub{meta("dns.reverse", func(m *checks.Meta) { m.Category = results.CategoryDNS })},
		stub{meta("nsx.tier0", func(m *checks.Meta) {
			m.Topologies = []config.Topology{config.TopologyNSX, config.TopologyNSXALB}
		})},
		stub{meta("verify.only", func(m *checks.Meta) { m.Modes = []checks.Mode{checks.ModeVerify} })},
	)

	tests := []struct {
		name     string
		selector registry.Selector
		want     []string
	}{
		{
			name:     "preflight on nsx selects everything that supports it",
			selector: registry.Selector{Mode: checks.ModePreflight, Topology: config.TopologyNSX},
			want:     []string{"dns.forward", "dns.reverse", "nsx.tier0"},
		},
		{
			name:     "topology filters out inapplicable checks",
			selector: registry.Selector{Mode: checks.ModePreflight, Topology: config.TopologyVDSALB},
			want:     []string{"dns.forward", "dns.reverse"},
		},
		{
			name:     "verify mode selects the verify-only check",
			selector: registry.Selector{Mode: checks.ModeVerify, Topology: config.TopologyNSX},
			want:     []string{"dns.forward", "dns.reverse", "nsx.tier0", "verify.only"},
		},
		{
			name:     "only by exact check id",
			selector: registry.Selector{Mode: checks.ModePreflight, Topology: config.TopologyNSX, Only: []string{"dns.forward"}},
			want:     []string{"dns.forward"},
		},
		{
			name:     "only by namespace prefix",
			selector: registry.Selector{Mode: checks.ModePreflight, Topology: config.TopologyNSX, Only: []string{"dns"}},
			want:     []string{"dns.forward", "dns.reverse"},
		},
		{
			name:     "only by category",
			selector: registry.Selector{Mode: checks.ModePreflight, Topology: config.TopologyNSX, Only: []string{"dns"}},
			want:     []string{"dns.forward", "dns.reverse"},
		},
		{
			name:     "skip by namespace",
			selector: registry.Selector{Mode: checks.ModePreflight, Topology: config.TopologyNSX, Skip: []string{"dns"}},
			want:     []string{"nsx.tier0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got []string
			for _, d := range reg.Select(tt.selector) {
				if d.Selected {
					got = append(got, d.Check.Meta().ID)
				}
			}
			if !equal(got, tt.want) {
				t.Errorf("selected %v, want %v", got, tt.want)
			}
		})
	}
}

// Every registered check must appear in the decision list, selected or not.
// The engine turns non-selections into reported skips; if Select silently
// dropped them, the report would overstate its coverage.
func TestSelectAccountsForEveryCheck(t *testing.T) {
	t.Parallel()

	reg := registry.New()
	reg.Register(
		stub{meta("a")},
		stub{meta("b", func(m *checks.Meta) { m.Topologies = []config.Topology{config.TopologyNSXVPC} })},
	)

	decisions := reg.Select(registry.Selector{Mode: checks.ModePreflight, Topology: config.TopologyNSX})
	if len(decisions) != reg.Len() {
		t.Fatalf("got %d decisions for %d checks", len(decisions), reg.Len())
	}
	for _, d := range decisions {
		if !d.Selected && d.Reason == "" {
			t.Errorf("check %s was excluded with no reason", d.Check.Meta().ID)
		}
	}
}

// Registration guards are startup panics on purpose: a shadowed or
// untraceable check produces a report that quietly omits a requirement, which
// is the failure mode this tool exists to prevent.
func TestRegistrationGuards(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		check checks.Check
	}{
		{"empty ID", stub{meta("")}},
		{"no requirement IDs", stub{meta("x", func(m *checks.Meta) { m.RequirementIDs = nil })}},
		{"no modes", stub{meta("x", func(m *checks.Meta) { m.Modes = nil })}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if recover() == nil {
					t.Errorf("expected a panic for %s", tt.name)
				}
			}()
			registry.New().Register(tt.check)
		})
	}
}

func TestDuplicateIDPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Error("expected a panic on duplicate check ID")
		}
	}()
	reg := registry.New()
	reg.Register(stub{meta("dup")})
	reg.Register(stub{meta("dup")})
}

// Ordering must be stable — report output and golden files depend on it.
func TestAllIsOrderedByID(t *testing.T) {
	t.Parallel()

	reg := registry.New()
	reg.Register(stub{meta("z.check")}, stub{meta("a.check")}, stub{meta("m.check")})

	var got []string
	for _, c := range reg.All() {
		got = append(got, c.Meta().ID)
	}
	if !equal(got, []string{"a.check", "m.check", "z.check"}) {
		t.Errorf("All() = %v, want sorted by ID", got)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
