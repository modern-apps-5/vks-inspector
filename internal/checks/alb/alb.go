// Package alb will hold checks requiring NSX Advanced Load Balancer (Avi)
// controller credentials. An API check — see docs/check-types.md.
//
// HAProxy checks live here too rather than in a package of their own:
// HAProxy is a single small surface, it is being phased out starting with the
// vCenter 9.x generation, and giving it its own package implies more future
// than it has.
package alb

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/modern-apps-5/vks-inspector/internal/checks"
	"github.com/modern-apps-5/vks-inspector/internal/config"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// deprecatedOnVCenterMajor is the vCenter major version starting at which
// HAProxy is being phased out as a Supervisor load balancer (matrix row
// LB-HAP-000). It is fully supported on vCenter 8.x and earlier.
//
// Named constant rather than external data, same reasoning as
// internal/checks/flb's minVCenterMajor: this is a support-lifecycle boundary
// for one specific product, not the kind of shifting patch-level
// interoperability grid ADR-0008 requires to be data (COM-VER-001).
const deprecatedOnVCenterMajor = 9

// Checks returns the checks in this package.
//
// TODO(phase-3): the rest are blocked on flagged matrix rows:
//
//	alb.controller-reachable  LB-ALB-001  controller answers on 443 and authenticates
//	alb.version-supported     LB-ALB-002  controller version is compatible (FLAGGED)
//	alb.cluster-healthy       LB-ALB-003  all controller cluster nodes up, quorum present
//	alb.license-tier          LB-ALB-004  license tier supports the required features (FLAGGED)
//	alb.cloud-configured      LB-ALB-005  declared cloud exists and is in a healthy state
//	alb.se-group              LB-ALB-006  Service Engine group exists and has capacity
//	alb.vip-network           LB-VIP-003  VIP network exists with an allocatable static pool
//	alb.vip-pool-free         LB-VIP-004  declared VIP range is not already allocated
//	alb.data-network-routing  LB-VIP-005  SE data network can reach the workload network
//	hap.dataplane-reachable   LB-HAP-001  HAProxy Data Plane API answers (legacy topology)
//	hap.vip-range             LB-HAP-002  HAProxy-owned VIP range matches the declared one
func Checks() []checks.Check {
	return []checks.Check{
		HAProxyVersionSupported{},
	}
}

// HAProxyVersionSupported reports HAProxy's support status for the vCenter
// version in play. Unlike the rest of this package, LB-HAP-000 is no longer
// flagged: the 8.x/9.x boundary is operator-confirmed, not reconstructed.
//
// This never blocks — HAProxy still works on vCenter 9.x, it is only being
// phased out — so it carries warning severity rather than blocker.
type HAProxyVersionSupported struct{}

var _ checks.Check = (*HAProxyVersionSupported)(nil)

// Meta implements checks.Check.
func (HAProxyVersionSupported) Meta() checks.Meta {
	return checks.Meta{
		ID:             "hap.version-supported",
		Title:          "HAProxy support status for this vCenter version",
		RequirementIDs: []string{"LB-HAP-000"},
		Category:       results.CategoryInventory,
		Layer:          results.LayerSupervisor,
		Severity:       results.SeverityWarning,
		Applies:        checks.Applicability{LoadBalancers: []config.LoadBalancer{config.LBHAProxy}},
		Modes:          checks.AllModes,
		Needs:          []checks.Capability{checks.CapVCenter},
		Remediation: fmt.Sprintf(
			"HAProxy is being phased out starting with the vCenter %d.x generation. It is fully "+
				"supported on vCenter 8.x. Plan a migration to NSX Advanced Load Balancer or "+
				"Foundation Load Balancer before upgrading vCenter.", deprecatedOnVCenterMajor),
	}
}

// Run implements checks.Check.
func (c HAProxyVersionSupported) Run(ctx context.Context, rc *checks.RunContext) ([]results.Result, error) {
	r := checks.NewResult(c.Meta(), rc, rc.Clients.VCenter.Endpoint())
	r.Expected = results.Value{
		Summary: fmt.Sprintf("vCenter earlier than %d.0, where HAProxy is fully supported", deprecatedOnVCenterMajor),
		Data:    map[string]any{"deprecated_from_major_version": deprecatedOnVCenterMajor},
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
	if major >= deprecatedOnVCenterMajor {
		// A warning, not a blocker: it still works, it just should not be a new
		// design.
		r.Status = results.StatusFail
		r.Observed.Summary = fmt.Sprintf(
			"vCenter is %s — HAProxy is being phased out starting in the %d.x generation",
			version, deprecatedOnVCenterMajor)
	} else {
		r.Status = results.StatusPass
		r.Observed.Summary = fmt.Sprintf("vCenter is %s — HAProxy is fully supported here", version)
	}
	checks.Finish(rc, &r)
	return []results.Result{r}, nil
}

// majorVersion extracts the leading integer from a dotted version string
// ("8.0.2" -> 8, true). vCenter's About().Version is always dotted like this.
func majorVersion(v string) (int, bool) {
	head, _, _ := strings.Cut(v, ".")
	n, err := strconv.Atoi(head)
	if err != nil {
		return 0, false
	}
	return n, true
}
