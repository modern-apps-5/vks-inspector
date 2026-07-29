// Package registry holds the set of known checks and decides which of them are
// eligible for a given run.
//
// Selection is a first-class concern rather than a loop in main: "why did this
// check not run" is the question operators ask most, and the answer has to be
// reportable. Every exclusion decision here produces a reason string that ends
// up in the report as a skip, not a silent absence.
package registry

import (
	"fmt"
	"sort"
	"strings"

	"github.com/modern-apps-5/vks-inspector/internal/checks"
	"github.com/modern-apps-5/vks-inspector/internal/config"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// Registry is a collection of checks keyed by ID.
type Registry struct {
	byID map[string]checks.Check
}

// New returns an empty registry.
func New() *Registry { return &Registry{byID: map[string]checks.Check{}} }

// Register adds a check. Duplicate IDs are a programming error and panic at
// startup rather than silently shadowing — a shadowed check would produce a
// report that quietly omits a requirement.
func (r *Registry) Register(cs ...checks.Check) {
	for _, c := range cs {
		id := c.Meta().ID
		if id == "" {
			panic("registry: check with empty ID")
		}
		if _, dup := r.byID[id]; dup {
			panic(fmt.Sprintf("registry: duplicate check ID %q", id))
		}
		if len(c.Meta().RequirementIDs) == 0 {
			panic(fmt.Sprintf("registry: check %q has no requirement IDs; every check must trace to docs/REQUIREMENTS-MATRIX.md", id))
		}
		if len(c.Meta().Modes) == 0 {
			panic(fmt.Sprintf("registry: check %q declares no modes", id))
		}
		r.byID[id] = c
	}
}

// All returns every registered check, ordered by ID. Deterministic ordering
// matters: report output and golden files depend on it.
func (r *Registry) All() []checks.Check {
	out := make([]checks.Check, 0, len(r.byID))
	for _, c := range r.byID {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Meta().ID < out[j].Meta().ID })
	return out
}

// Get returns a check by ID.
func (r *Registry) Get(id string) (checks.Check, bool) {
	c, ok := r.byID[id]
	return c, ok
}

// Len returns the number of registered checks.
func (r *Registry) Len() int { return len(r.byID) }

// Selector describes which checks a run wants.
type Selector struct {
	Mode     checks.Mode
	Topology config.Topology
	// Layer restricts to Supervisor-enablement or VKS-cluster prerequisites.
	// Empty or LayerBoth runs everything.
	Layer results.Layer
	// Only, when non-empty, restricts to these check IDs or categories.
	Only []string
	// Skip removes these check IDs or categories.
	Skip []string
}

// Decision is the outcome of selecting one check.
type Decision struct {
	Check    checks.Check
	Selected bool
	// Reason explains a non-selection in operator language. Empty when selected.
	Reason string
}

// Select returns a decision per registered check, in ID order. Callers report
// the non-selected ones as skips rather than dropping them, so a report always
// accounts for every check the build knows about.
func (r *Registry) Select(s Selector) []Decision {
	only := normaliseSet(s.Only)
	skip := normaliseSet(s.Skip)

	out := make([]Decision, 0, len(r.byID))
	for _, c := range r.All() {
		m := c.Meta()
		d := Decision{Check: c, Selected: true}

		want := s.Layer
		if want == "" {
			want = results.LayerBoth
		}

		switch {
		case !m.SupportsMode(s.Mode):
			d.Selected, d.Reason = false, fmt.Sprintf("check does not support %s mode", s.Mode)
		case !m.AppliesTo(s.Topology):
			d.Selected, d.Reason = false, fmt.Sprintf("applies to %s, this environment is %s",
				m.Applies.Describe(), s.Topology)
		case !m.EffectiveLayer().Includes(want):
			d.Selected, d.Reason = false, fmt.Sprintf("%s-layer check, run asked for %s", m.EffectiveLayer(), want)
		case len(only) > 0 && !matches(m, only):
			d.Selected, d.Reason = false, "excluded by --only"
		case len(skip) > 0 && matches(m, skip):
			d.Selected, d.Reason = false, "excluded by --skip or policy.skip"
		}
		out = append(out, d)
	}
	return out
}

func matches(m checks.Meta, set map[string]bool) bool {
	if set[strings.ToLower(m.ID)] {
		return true
	}
	if set[strings.ToLower(string(m.Category))] {
		return true
	}
	// Prefix match on the dotted namespace, so `--only dns` selects
	// `dns.forward-reverse` and friends.
	if i := strings.Index(m.ID, "."); i > 0 && set[strings.ToLower(m.ID[:i])] {
		return true
	}
	return false
}

func normaliseSet(in []string) map[string]bool {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]bool, len(in))
	for _, s := range in {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" {
			out[s] = true
		}
	}
	return out
}
