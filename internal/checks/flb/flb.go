// Package flb holds checks for vSphere Supervisor with Foundation Load
// Balancer (VCF 9.1+). Taxonomy class (b) — see docs/CHECK-TAXONOMY.md — but
// unlike alb and nsx, FLB has no separate controller API to authenticate
// against: it is deployed and configured through vCenter itself, so its
// checks lean on CapVCenter rather than a dedicated capability or client.
package flb

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/modern-apps-5/vks-inspector/internal/checks"
	"github.com/modern-apps-5/vks-inspector/internal/config"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// minVCenterMajor is the lowest vCenter major version FLB exists on at all —
// the fetched Broadcom requirements page states vCenter 9.0 as the minimum
// for a Supervisor with Foundation Load Balancer (matrix row LB-FLB-000).
//
// This is deliberately a named constant rather than external data. ADR-0008
// bars hardcoding version-*interoperability* data (COM-VER-001: which patch
// builds interoperate, a grid that shifts every release) — but this is a hard
// existence boundary, not a compatibility grid: FLB did not exist before
// vCenter 9.0. Update it if that boundary is confirmed to have moved.
const minVCenterMajor = 9

// Checks returns the checks in this package.
//
// TODO(phase-3): the rest are blocked on flagged matrix rows:
//
//	flb.vm-healthy            LB-FLB-001  FLB VM(s) exist, powered on, correctly placed
//	flb.topology-networks     LB-FLB-002  declared arm mode has its required networks
//	flb.one-nic-simplified    LB-FLB-003  one-arm-one-nic only used with a Simplified Supervisor
//	flb.ha-prerequisites      LB-FLB-004  DRS/HA cluster prerequisites for active-passive mode
//	flb.vip-pool-free         LB-FLB-005  declared VIP range is not already allocated (LOW confidence)
func Checks() []checks.Check {
	return []checks.Check{
		VersionSupported{},
	}
}

// VersionSupported asserts the vCenter version is new enough for FLB to exist
// at all. Unlike everything else in this package, this one is not blocked on
// a flagged row: LB-FLB-000 is unflagged, HIGH confidence, sourced from a
// fetched product doc.
type VersionSupported struct{}

var _ checks.Check = (*VersionSupported)(nil)

// Meta implements checks.Check.
func (VersionSupported) Meta() checks.Meta {
	return checks.Meta{
		ID:             "flb.version-supported",
		Title:          "vCenter version supports Foundation Load Balancer",
		RequirementIDs: []string{"LB-FLB-000"},
		Category:       results.CategoryInventory,
		Layer:          results.LayerSupervisor,
		Severity:       results.SeverityBlocker,
		Applies:        checks.Applicability{LoadBalancers: []config.LoadBalancer{config.LBFLB}},
		Modes:          checks.AllModes,
		Needs:          []checks.Capability{checks.CapVCenter},
		Remediation: fmt.Sprintf(
			"Foundation Load Balancer does not exist before vCenter %d.0. Upgrade vCenter, or choose "+
				"NSX Advanced Load Balancer or the NSX built-in load balancer instead.", minVCenterMajor),
	}
}

// Run implements checks.Check.
func (c VersionSupported) Run(ctx context.Context, rc *checks.RunContext) ([]results.Result, error) {
	r := checks.NewResult(c.Meta(), rc, rc.Clients.VCenter.Endpoint())
	r.Expected = results.Value{
		Summary: fmt.Sprintf("vCenter %d.0 or later", minVCenterMajor),
		Data:    map[string]any{"min_major_version": minVCenterMajor},
	}

	about, err := rc.Clients.VCenter.About(ctx)
	if err != nil {
		// Unreachable/unauthenticated is vc.api-reachable's finding, not this
		// check's — we cannot assert a version we could not read.
		r.Status = results.StatusUnknown
		r.Observed = results.Value{
			Summary: "could not read the vCenter version: " + err.Error(),
			Data:    map[string]any{"error": err.Error()},
		}
		checks.Finish(rc, &r)
		return []results.Result{r}, nil
	}

	version, _ := about["version"].(string)
	major, ok := majorVersion(version)
	if !ok {
		r.Status = results.StatusUnknown
		r.Observed = results.Value{
			Summary: fmt.Sprintf("vCenter reported an unparseable version %q", version),
			Data:    map[string]any{"version": version},
		}
		checks.Finish(rc, &r)
		return []results.Result{r}, nil
	}

	r.Observed = results.Value{Data: map[string]any{"version": version, "major_version": major}}
	if major < minVCenterMajor {
		r.Status = results.StatusFail
		r.Observed.Summary = fmt.Sprintf(
			"vCenter is %s — Foundation Load Balancer requires %d.0 or later", version, minVCenterMajor)
	} else {
		r.Status = results.StatusPass
		r.Observed.Summary = fmt.Sprintf("vCenter is %s", version)
	}
	checks.Finish(rc, &r)
	return []results.Result{r}, nil
}

// majorVersion extracts the leading integer from a dotted version string
// ("9.0.0" -> 9, true). vCenter's About().Version is always dotted like this.
func majorVersion(v string) (int, bool) {
	head, _, _ := strings.Cut(v, ".")
	n, err := strconv.Atoi(head)
	if err != nil {
		return 0, false
	}
	return n, true
}
