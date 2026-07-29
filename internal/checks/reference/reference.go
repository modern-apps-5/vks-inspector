// Package reference contains the one fully-implemented check in phase 1.
//
// It exists to prove the shape end to end — Meta → registry selection → Run →
// Result → renderer → exit code → baseline serialisation — before any real
// networking logic is written. Every subsequent check should read like this
// one. If a new check cannot be written in this shape, the shape is wrong and
// that is a design conversation, not a reason to special-case the check.
//
// It is also genuinely useful: it records, in every report and every baseline,
// which topology the run was graded against and how many checks this build
// knew about. A baseline that does not record that is not interpretable a year
// later.
package reference

import (
	"context"

	"github.com/modern-apps-5/vks-inspector/internal/buildinfo"
	"github.com/modern-apps-5/vks-inspector/internal/checks"
	"github.com/modern-apps-5/vks-inspector/internal/config"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// TopologyRecognised asserts that the declared topology is one this build knows
// how to grade.
type TopologyRecognised struct {
	// KnownCheckCount is injected by the wiring in checks/all so the check can
	// record the size of the check set without importing the registry (which
	// would be an import cycle).
	KnownCheckCount int
}

var _ checks.Check = (*TopologyRecognised)(nil)

// Meta implements checks.Check.
func (c *TopologyRecognised) Meta() checks.Meta {
	return checks.Meta{
		ID:             "meta.topology-recognised",
		Title:          "Declared topology is supported by this build",
		RequirementIDs: []string{"MET-001"},
		Category:       results.CategoryMeta,
		Severity:       results.SeverityBlocker,
		// No topology restriction: this check is what tells you the topology
		// was understood, so it cannot itself be filtered by topology.
		Topologies: nil,
		// Runs in every mode. In preflight it grades declared intent; in verify
		// and snapshot it records the same fact about the run that produced the
		// artifact, which is what makes two artifacts comparable at all.
		Modes: checks.AllModes,
		Needs: nil,
		Remediation: "Set `topology:` in the config to one of: " +
			topologyList() + ". If the environment genuinely uses a shape not in that " +
			"list, this build cannot grade it — do not interpret other passes as coverage.",
	}
}

// Run implements checks.Check.
func (c *TopologyRecognised) Run(ctx context.Context, rc *checks.RunContext) ([]results.Result, error) {
	r := checks.NewResult(c.Meta(), rc, string(rc.Config.Topology))

	r.Expected = results.Value{
		Summary: "one of: " + topologyList(),
		Data: map[string]any{
			"supported_topologies": topologyStrings(),
		},
	}

	declared := rc.Config.Topology
	r.Observed = results.Value{
		Summary: string(declared) + " — " + declared.Description(),
		// Data is what drift diffs. Keep it small, stable and machine-typed:
		// a topology change or a tool upgrade that changes the check set are
		// both things a later run should surface.
		Data: map[string]any{
			"topology":          string(declared),
			"tool_version":      buildinfo.Version,
			"known_check_count": c.KnownCheckCount,
		},
	}

	if declared.Valid() {
		r.Status = results.StatusPass
	} else {
		r.Status = results.StatusFail
		r.Observed.Summary = string(declared) + " — not recognised by this build"
	}

	// Evidence is verbose-only detail. Never assertions, never secrets.
	r.Evidence = map[string]any{
		"mode":          string(rc.Mode),
		"config_name":   rc.Config.Metadata.Name,
		"config_digest": rc.Config.Digest(),
	}

	checks.Finish(rc, &r)
	return []results.Result{r}, nil
}

func topologyStrings() []string {
	out := make([]string, 0, len(config.AllTopologies))
	for _, t := range config.AllTopologies {
		out = append(out, string(t))
	}
	return out
}

func topologyList() string {
	s := ""
	for i, t := range config.AllTopologies {
		if i > 0 {
			s += ", "
		}
		s += string(t)
	}
	return s
}
