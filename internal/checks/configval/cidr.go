package configval

import (
	"context"
	"fmt"

	"github.com/modern-apps-5/vks-inspector/internal/checks"
	"github.com/modern-apps-5/vks-inspector/internal/netx"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// sprintf is a local alias so the inventory file reads without an fmt import
// on every line.
func sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }

// ---------------------------------------------------------------------------
// The fan-out idiom
//
// A check that finds nothing wrong returns ONE passing result summarising what
// it examined. A check that finds problems returns one result PER problem, each
// with a Target naming the specific collision.
//
// This is not cosmetic. Each overlap is a separate ticket with a separate fix,
// and a single result saying "3 overlaps" cannot be triaged, cannot be
// individually severity-overridden, and cannot be diffed by drift when one of
// the three is fixed. The passing case collapses to one row because "47 pairs
// were disjoint" is not 47 findings.
// ---------------------------------------------------------------------------

// Overlap asserts that no two declared ranges intersect.
//
// This is the cheapest check in the tool and catches the most expensive class
// of mistake: an address plan that could never have worked. Overlaps cannot be
// fixed after Supervisor enablement without a rebuild.
type Overlap struct{}

var _ checks.Check = (*Overlap)(nil)

// Meta implements checks.Check.
func (Overlap) Meta() checks.Meta {
	return checks.Meta{
		ID:             "cidr.overlap",
		Title:          "No two declared ranges overlap",
		RequirementIDs: []string{"COM-CID-001"},
		Category:       results.CategoryCIDR,
		Layer:          results.LayerSupervisor,
		Severity:       results.SeverityBlocker,
		Modes:          checks.AllModes,
		Remediation: "Re-plan the address space so the ranges are disjoint. Overlapping ranges route " +
			"unpredictably and cannot be corrected after Supervisor enablement without rebuilding it.",
	}
}

// Run implements checks.Check.
func (c Overlap) Run(ctx context.Context, rc *checks.RunContext) ([]results.Result, error) {
	nets, bad := declaredNetworks(rc.Config)

	var out []results.Result
	out = append(out, malformedResults(c.Meta(), rc, bad)...)

	var found []results.Result
	pairs := 0
	for i := 0; i < len(nets); i++ {
		for j := i + 1; j < len(nets); j++ {
			// A network's own static range sits inside its own subnet by
			// design. That containment is asserted by range.containment; it is
			// not a collision and must not be reported as one.
			if nets[i].SiblingOfParent(nets[j]) {
				continue
			}
			pairs++
			if !nets[i].Overlaps(nets[j]) {
				continue
			}
			r := checks.NewResult(c.Meta(), rc, nets[i].Source+" vs "+nets[j].Source)
			r.Status = results.StatusFail
			r.Expected = results.Value{
				Summary: sprintf("%s and %s are disjoint", nets[i], nets[j]),
			}
			r.Observed = results.Value{
				Summary: sprintf("%s overlaps %s", nets[i].Describe(), nets[j].Describe()),
				Data: map[string]any{
					"a":        nets[i].String(),
					"a_source": nets[i].Source,
					"b":        nets[j].String(),
					"b_source": nets[j].Source,
					"overlap":  intersectionString(nets[i], nets[j]),
				},
			}
			checks.Finish(rc, &r)
			found = append(found, r)
		}
	}

	if len(found) > 0 {
		return append(out, found...), nil
	}

	r := checks.NewResult(c.Meta(), rc, "")
	r.Status = results.StatusPass
	r.Expected = results.Text("no declared range overlaps another")
	r.Observed = results.Value{
		Summary: sprintf("%d declared ranges, %d pairs checked, no overlaps", len(nets), pairs),
		Data: map[string]any{
			"ranges_checked": len(nets),
			"pairs_checked":  pairs,
			"overlaps":       0,
		},
	}
	checks.Finish(rc, &r)
	return append(out, r), nil
}

// ExternalCollision asserts that no declared range collides with infrastructure
// that already exists.
type ExternalCollision struct{}

var _ checks.Check = (*ExternalCollision)(nil)

// Meta implements checks.Check.
func (ExternalCollision) Meta() checks.Meta {
	return checks.Meta{
		ID:             "cidr.external-collision",
		Title:          "No declared range collides with existing infrastructure",
		RequirementIDs: []string{"COM-CID-002"},
		Category:       results.CategoryCIDR,
		Layer:          results.LayerSupervisor,
		Severity:       results.SeverityBlocker,
		Modes:          checks.AllModes,
		Remediation: "Re-plan the colliding range, or extend kubernetes.externalCIDRs if the declared " +
			"list of existing networks is incomplete.",
	}
}

// Run implements checks.Check.
func (c ExternalCollision) Run(ctx context.Context, rc *checks.RunContext) ([]results.Result, error) {
	nets, badNets := declaredNetworks(rc.Config)
	ext, badExt := externalNetworks(rc.Config)

	var out []results.Result
	out = append(out, malformedResults(c.Meta(), rc, append(badNets, badExt...))...)

	// An empty external list is not a pass. It means the check had nothing to
	// compare against, and reporting that as healthy would be the tool lying
	// about its own coverage.
	if len(ext) == 0 {
		r := checks.NewResult(c.Meta(), rc, "")
		r.Status = results.StatusSkip
		r.Expected = results.Text("declared ranges do not collide with existing infrastructure")
		r.Observed = results.Value{
			Summary: "kubernetes.externalCIDRs is empty — nothing to compare against",
			Data:    map[string]any{"skip_reason": "no external CIDRs declared"},
		}
		r.Remediation = "Declare the corporate, VPN and neighbouring-cluster ranges in " +
			"kubernetes.externalCIDRs. This check can only find collisions it is told about; " +
			"an empty list means it found nothing because it looked at nothing."
		checks.Finish(rc, &r)
		return append(out, r), nil
	}

	var found []results.Result
	for _, n := range nets {
		for _, e := range ext {
			if !n.Overlaps(e) {
				continue
			}
			r := checks.NewResult(c.Meta(), rc, n.Source+" vs "+e.Source)
			r.Status = results.StatusFail
			r.Expected = results.Value{Summary: sprintf("%s is outside all declared external ranges", n)}
			r.Observed = results.Value{
				Summary: sprintf("%s collides with existing network %s", n.Describe(), e.Describe()),
				Data: map[string]any{
					"declared":        n.String(),
					"declared_source": n.Source,
					"external":        e.String(),
					"external_source": e.Source,
					"overlap":         intersectionString(n, e),
				},
			}
			checks.Finish(rc, &r)
			found = append(found, r)
		}
	}

	if len(found) > 0 {
		return append(out, found...), nil
	}

	r := checks.NewResult(c.Meta(), rc, "")
	r.Status = results.StatusPass
	r.Expected = results.Text("no declared range collides with existing infrastructure")
	r.Observed = results.Value{
		Summary: sprintf("%d declared ranges checked against %d existing networks, no collisions", len(nets), len(ext)),
		Data: map[string]any{
			"ranges_checked":   len(nets),
			"external_checked": len(ext),
			"collisions":       0,
		},
	}
	checks.Finish(rc, &r)
	return append(out, r), nil
}

// InfraCollision asserts that cluster-internal ranges do not swallow the
// address of something the cluster has to reach.
//
// This is a distinct failure from a plain overlap. A pod CIDR that contains the
// vCenter address does not produce a routing conflict anyone notices at
// enablement time — it produces a Supervisor that cannot reach vCenter from
// inside a pod, weeks later, with a symptom that points nowhere near the cause.
type InfraCollision struct{}

var _ checks.Check = (*InfraCollision)(nil)

// Meta implements checks.Check.
func (InfraCollision) Meta() checks.Meta {
	return checks.Meta{
		ID:             "cidr.infra-collision",
		Title:          "Cluster-internal ranges do not contain infrastructure addresses",
		RequirementIDs: []string{"COM-CID-003"},
		Category:       results.CategoryCIDR,
		Layer:          results.LayerSupervisor,
		Severity:       results.SeverityBlocker,
		Modes:          checks.AllModes,
		Remediation: "Re-plan the pod or service CIDR. A cluster-internal range that contains an " +
			"infrastructure address makes that endpoint permanently unreachable from inside the " +
			"cluster, and the symptom appears long after enablement.",
	}
}

// Run implements checks.Check.
func (c InfraCollision) Run(ctx context.Context, rc *checks.RunContext) ([]results.Result, error) {
	nets, bad := declaredNetworks(rc.Config)
	infra := infraAddresses(rc.Config)

	var out []results.Result
	out = append(out, malformedResults(c.Meta(), rc, bad)...)

	// Only the non-routable cluster-internal ranges matter here; a routable
	// range containing an infrastructure address is an ordinary overlap and is
	// COM-CID-001's problem.
	var internal []netx.Named
	for _, n := range nets {
		if !n.Routable {
			internal = append(internal, n)
		}
	}

	var found []results.Result
	for _, n := range internal {
		rng, ok := n.AsRange()
		if !ok {
			continue
		}
		for _, a := range infra {
			if !rng.Contains(a.Addr) {
				continue
			}
			r := checks.NewResult(c.Meta(), rc, n.Source+" contains "+a.Source)
			r.Status = results.StatusFail
			r.Expected = results.Value{Summary: sprintf("%s does not contain %s", n, a.Addr)}
			r.Observed = results.Value{
				Summary: sprintf("%s contains %s (%s), which the cluster must reach", n.Describe(), a.Addr, a.Name),
				Data: map[string]any{
					"range":          n.String(),
					"range_source":   n.Source,
					"address":        a.Addr.String(),
					"address_source": a.Source,
				},
			}
			checks.Finish(rc, &r)
			found = append(found, r)
		}
	}

	if len(found) > 0 {
		return append(out, found...), nil
	}

	r := checks.NewResult(c.Meta(), rc, "")
	r.Status = results.StatusPass
	r.Expected = results.Text("no cluster-internal range contains an infrastructure address")
	r.Observed = results.Value{
		Summary: sprintf("%d internal ranges checked against %d declared infrastructure addresses",
			len(internal), len(infra)),
		Data: map[string]any{
			"internal_ranges_checked": len(internal),
			"addresses_checked":       len(infra),
			"collisions":              0,
		},
	}
	// Say what was NOT covered. Endpoints declared only as an FQDN have no
	// address to compare, and a reader who does not know that will over-trust
	// this pass.
	if n := unresolvedEndpointCount(rc); n > 0 {
		r.Observed.Summary += sprintf("; %d endpoint(s) declared by FQDN only and not covered here", n)
		r.Observed.Data["fqdn_only_endpoints"] = n
	}
	checks.Finish(rc, &r)
	return append(out, r), nil
}

// unresolvedEndpointCount counts endpoints that declared an FQDN but no IP, and
// are therefore invisible to a config-only collision check.
func unresolvedEndpointCount(rc *checks.RunContext) int {
	cfg := rc.Config
	n := 0
	count := func(fqdn, ip string) {
		if fqdn != "" && ip == "" {
			n++
		}
	}
	count(cfg.Infrastructure.VCenter.FQDN, cfg.Infrastructure.VCenter.IP)
	if cfg.Infrastructure.NSXManager != nil {
		count(cfg.Infrastructure.NSXManager.FQDN, cfg.Infrastructure.NSXManager.IP)
	}
	for _, h := range cfg.Infrastructure.ESXiHosts {
		count(h.FQDN, h.IP)
	}
	if cfg.Infrastructure.Registry != nil {
		count(cfg.Infrastructure.Registry.FQDN, cfg.Infrastructure.Registry.IP)
	}
	if cfg.ALB != nil {
		count(cfg.ALB.Controller.FQDN, cfg.ALB.Controller.IP)
	}
	return n
}

// malformedResults turns unparseable config values into reported findings.
//
// A CIDR that does not parse must be a visible result, not a silent omission:
// otherwise a typo'd range simply disappears from every overlap comparison and
// the report looks cleaner than the config is.
func malformedResults(m checks.Meta, rc *checks.RunContext, bad []badEntry) []results.Result {
	var out []results.Result
	for _, b := range bad {
		r := checks.NewResult(m, rc, b.Source)
		r.Status = results.StatusFail
		r.Expected = results.Text("a parseable address range")
		r.Observed = results.Value{
			Summary: sprintf("%s = %q is not usable: %v", b.Source, b.Value, b.Err),
			Data: map[string]any{
				"source": b.Source,
				"value":  b.Value,
				"error":  b.Err.Error(),
			},
		}
		r.Remediation = "Correct the value. Until it parses it is excluded from every overlap " +
			"and containment comparison, so other checks are reporting less coverage than they appear to."
		checks.Finish(rc, &r)
		out = append(out, r)
	}
	return out
}

func intersectionString(a, b netx.Named) string {
	ra, ok1 := a.AsRange()
	rb, ok2 := b.AsRange()
	if !ok1 || !ok2 {
		return ""
	}
	if i, ok := ra.Intersection(rb); ok {
		return i.String()
	}
	return ""
}
