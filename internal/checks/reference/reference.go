// Package reference contains the reference implementation of a check.
//
// It exists to prove the shape end to end — Meta → registry selection → Run →
// Result → renderer → exit code → baseline serialisation. Every subsequent
// check should read like this one. If a new check cannot be written in this
// shape, the shape is wrong and that is a design conversation, not a reason to
// special-case the check.
//
// It is also genuinely useful: it records, in every report and every baseline,
// which topology the run was graded against and how many checks this build knew
// about. A baseline that does not record that is not interpretable a year later.
package reference

import (
	"context"
	"strings"

	"github.com/modern-apps-5/vks-inspector/internal/buildinfo"
	"github.com/modern-apps-5/vks-inspector/internal/checks"
	"github.com/modern-apps-5/vks-inspector/internal/config"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// TopologyRecognised asserts that the declared topology is a combination this
// build knows how to grade.
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
		Title:          "Declared topology is a supported combination",
		RequirementIDs: []string{"MET-001"},
		Category:       results.CategoryMeta,
		// Supervisor-layer: the topology decision is made at Supervisor
		// enablement time and everything else follows from it.
		Layer:    results.LayerSupervisor,
		Severity: results.SeverityBlocker,
		// No applicability restriction: this check is what tells you the
		// topology was understood, so it cannot itself be filtered by topology.
		Applies: checks.Applicability{},
		// Runs in every mode. In preflight it grades declared intent; in verify
		// and snapshot it records the same fact about the run that produced the
		// artifact, which is what makes two artifacts comparable at all.
		Modes: checks.AllModes,
		Needs: nil,
		Remediation: "Set `topology.networking` and `topology.loadBalancer` to a supported combination: " +
			supportedList() + ". If the environment genuinely uses a shape not in that list, this build " +
			"cannot grade it — do not interpret other passes as coverage.",
	}
}

// Run implements checks.Check.
func (c *TopologyRecognised) Run(ctx context.Context, rc *checks.RunContext) ([]results.Result, error) {
	declared := rc.Config.Topology
	r := checks.NewResult(c.Meta(), rc, declared.String())

	r.Expected = results.Value{
		Summary: "one of: " + supportedList(),
		Data: map[string]any{
			"supported_combinations": supportedStrings(),
		},
	}

	r.Observed = results.Value{
		Summary: declared.String() + " — " + declared.Description(),
		// Data is what drift diffs. Keep it small, stable and machine-typed: a
		// topology change or a tool upgrade that changes the check set are both
		// things a later run should surface.
		Data: map[string]any{
			"networking":        string(declared.Networking),
			"load_balancer":     string(declared.LoadBalancer),
			"tool_version":      buildinfo.Version,
			"known_check_count": c.KnownCheckCount,
		},
	}

	if err := declared.Validate(); err != nil {
		r.Status = results.StatusFail
		r.Observed.Summary = declared.String() + " — " + err.Error()
	} else {
		r.Status = results.StatusPass
		// A supported-but-caveated combination still passes. The caveat belongs
		// in the report, not in the verdict: refusing to grade HAProxy or VPC
		// would be more honest only if we were certain they were unsupported,
		// and we are not.
		if note := declared.Note(); note != "" {
			r.Observed.Summary += " (" + note + ")"
		}
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

func supportedStrings() []string {
	combos := config.SupportedCombinations()
	out := make([]string, 0, len(combos))
	for _, t := range combos {
		out = append(out, t.String())
	}
	return out
}

func supportedList() string { return strings.Join(supportedStrings(), ", ") }
