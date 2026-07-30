// Package vcenter holds checks requiring vCenter API credentials.
// An API check — see docs/check-types.md.
//
// Every check here declares checks.CapVCenter, so a run without credentials
// reports them as skips rather than failing an environment it never inspected.
// "We could not log in" is a statement about the tool's access, not about the
// customer's network.
//
// All calls are read-only. See docs/ADR/0007.
package vcenter

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modern-apps-5/vks-inspector/internal/checks"
	vc "github.com/modern-apps-5/vks-inspector/internal/clients/vcenter"
	"github.com/modern-apps-5/vks-inspector/internal/config"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// Checks returns the checks in this package.
//
// TODO — blocked on flagged matrix rows, which need a number or a fact this
// project cannot supply from memory:
//
//	vc.version-supported  COM-VER-001 ⚑  interoperability matrix must be data
//	vc.host-ntp           COM-NTP-003 ⚑  confirm VCF 9 enforces it at enablement
//	vc.host-time-skew     COM-NTP-004 ⚑  no documented tolerance known
//	vc.portgroup-security VDS-PG-004  ⚑  requirements differ per load balancer
func Checks() []checks.Check {
	return []checks.Check{
		APIReachable{},
		ClusterExists{},
		SwitchExists{},
		SwitchMTU{},
		PortGroupsExist{},
	}
}

// ---------------------------------------------------------------------------

// APIReachable asserts the vCenter API answers and the credentials authenticate.
//
// The cheapest proof that anything else in this package can work, and the check
// whose failure explains every other vCenter skip in the report.
type APIReachable struct{}

var _ checks.Check = (*APIReachable)(nil)

// Meta implements checks.Check.
func (APIReachable) Meta() checks.Meta {
	return checks.Meta{
		ID:             "vc.api-reachable",
		Title:          "vCenter API answers and credentials authenticate",
		RequirementIDs: []string{"COM-API-001"},
		Category:       results.CategoryReachability,
		Layer:          results.LayerSupervisor,
		Severity:       results.SeverityBlocker,
		Modes:          checks.AllModes,
		Needs:          []checks.Capability{checks.CapVCenter},
		Remediation: "Confirm the endpoint, the credentials, and that the account can read the " +
			"inventory. Note this verifies THIS tool's access only — the account used for the " +
			"actual deployment needs more privileges, which is a separate question (matrix COM-API-001).",
	}
}

// Run implements checks.Check.
func (c APIReachable) Run(ctx context.Context, rc *checks.RunContext) ([]results.Result, error) {
	r := checks.NewResult(c.Meta(), rc, rc.Clients.VCenter.Endpoint())
	r.Expected = results.Value{
		Summary: "the endpoint is a vCenter and the credentials authenticate",
		Data:    map[string]any{"api_type": "VirtualCenter"},
	}

	about, err := rc.Clients.VCenter.About(ctx)
	if err != nil {
		// The API being unreachable is a finding about the environment, not a
		// tool fault — the operator gave us an address that does not answer.
		r.Status = results.StatusFail
		r.Observed = results.Value{
			Summary: "could not query the API: " + err.Error(),
			Data:    map[string]any{"error": err.Error()},
		}
		checks.Finish(rc, &r)
		return []results.Result{r}, nil
	}

	isVC, _ := rc.Clients.VCenter.IsVCenter(ctx)
	r.Observed = results.Value{
		Summary: fmt.Sprintf("%v %v build %v", about["name"], about["version"], about["build"]),
		Data: map[string]any{
			"version":     about["version"],
			"build":       about["build"],
			"api_version": about["api_version"],
			"api_type":    about["api_type"],
		},
	}

	if !isVC {
		// Pointing the tool at an ESXi host instead of vCenter is a common
		// mistake whose downstream failures are baffling if not caught here.
		r.Status = results.StatusFail
		r.Observed.Summary += " — this is not a vCenter"
		r.Remediation = "The endpoint answered but reports api_type " +
			fmt.Sprint(about["api_type"]) + ", not VirtualCenter. This looks like a bare ESXi host. " +
			"Point --vcenter at the vCenter Server instead."
	} else {
		r.Status = results.StatusPass
	}

	checks.Finish(rc, &r)
	return []results.Result{r}, nil
}

// ---------------------------------------------------------------------------

// ClusterExists asserts the declared datacenter and cluster are real.
type ClusterExists struct{}

var _ checks.Check = (*ClusterExists)(nil)

// Meta implements checks.Check.
func (ClusterExists) Meta() checks.Meta {
	return checks.Meta{
		ID:             "vc.cluster-exists",
		Title:          "Declared datacenter and cluster exist",
		RequirementIDs: []string{"INV-VC-001"},
		Category:       results.CategoryInventory,
		Layer:          results.LayerSupervisor,
		Severity:       results.SeverityBlocker,
		Modes:          checks.AllModes,
		Needs:          []checks.Capability{checks.CapVCenter},
		Remediation: "Correct vsphere.datacenter / vsphere.cluster in the config, or create the " +
			"cluster. A typo here makes every other inventory check inspect the wrong object.",
	}
}

// Run implements checks.Check.
func (c ClusterExists) Run(ctx context.Context, rc *checks.RunContext) ([]results.Result, error) {
	target := rc.Config.VSphere.Cluster
	r := checks.NewResult(c.Meta(), rc, target)
	r.Expected = results.Value{
		Summary: fmt.Sprintf("cluster %q exists in datacenter %q",
			rc.Config.VSphere.Cluster, rc.Config.VSphere.Datacenter),
		Data: map[string]any{
			"datacenter": rc.Config.VSphere.Datacenter,
			"cluster":    rc.Config.VSphere.Cluster,
		},
	}

	if target == "" {
		r.Status = results.StatusSkip
		r.Observed = results.Value{
			Summary: "no cluster declared in the config",
			Data:    map[string]any{"skip_reason": "vsphere.cluster is empty"},
		}
		checks.Finish(rc, &r)
		return []results.Result{r}, nil
	}

	info, err := rc.Clients.VCenter.Cluster(ctx, rc.Config.VSphere.Datacenter, target)
	if err != nil {
		r.Status, r.Observed = classify(err, "cluster "+target)
		// Name what does exist. "Cluster not found" plus the list of clusters
		// that do is a fix; "cluster not found" alone is a guessing game.
		if inv, derr := rc.Clients.VCenter.Discover(ctx); derr == nil {
			r.Observed.Summary += fmt.Sprintf(" (this vCenter has: %s)", joinOrNone(inv.Clusters))
			r.Observed.Data["available_clusters"] = inv.Clusters
			r.Observed.Data["available_datacenters"] = inv.Datacenters
		}
		checks.Finish(rc, &r)
		return []results.Result{r}, nil
	}

	r.Status = results.StatusPass
	r.Observed = results.Value{
		Summary: fmt.Sprintf("%s exists with %d host(s)", info.Name, info.HostCount),
		Data: map[string]any{
			"cluster":     info.Name,
			"datacenter":  info.Datacenter,
			"host_count":  info.HostCount,
			"hosts":       info.Hosts,
			"drs_enabled": info.DRSEnabled,
			"ha_enabled":  info.HAEnabled,
		},
	}
	checks.Finish(rc, &r)
	return []results.Result{r}, nil
}

// ---------------------------------------------------------------------------

// SwitchExists asserts the declared distributed switch is real.
type SwitchExists struct{}

var _ checks.Check = (*SwitchExists)(nil)

// Meta implements checks.Check.
func (SwitchExists) Meta() checks.Meta {
	return checks.Meta{
		ID:             "vc.vds-exists",
		Title:          "Declared distributed switch exists",
		RequirementIDs: []string{"INV-VC-002"},
		Category:       results.CategoryInventory,
		Layer:          results.LayerSupervisor,
		Severity:       results.SeverityBlocker,
		Modes:          checks.AllModes,
		Needs:          []checks.Capability{checks.CapVCenter},
		Remediation:    "Correct vsphere.distributedSwitch in the config, or create the switch.",
	}
}

// Run implements checks.Check.
func (c SwitchExists) Run(ctx context.Context, rc *checks.RunContext) ([]results.Result, error) {
	name := rc.Config.VSphere.DistributedSwitch
	r := checks.NewResult(c.Meta(), rc, name)
	r.Expected = results.Value{
		Summary: fmt.Sprintf("distributed switch %q exists", name),
		Data:    map[string]any{"switch": name},
	}

	if name == "" {
		r.Status = results.StatusSkip
		r.Observed = results.Value{
			Summary: "no distributed switch declared in the config",
			Data:    map[string]any{"skip_reason": "vsphere.distributedSwitch is empty"},
		}
		checks.Finish(rc, &r)
		return []results.Result{r}, nil
	}

	info, err := rc.Clients.VCenter.DistributedSwitch(ctx, name)
	if err != nil {
		r.Status, r.Observed = classify(err, "distributed switch "+name)
		if inv, derr := rc.Clients.VCenter.Discover(ctx); derr == nil {
			r.Observed.Summary += fmt.Sprintf(" (this vCenter has: %s)", joinOrNone(inv.Switches))
			r.Observed.Data["available_switches"] = inv.Switches
		}
		checks.Finish(rc, &r)
		return []results.Result{r}, nil
	}

	r.Status = results.StatusPass
	r.Observed = results.Value{
		Summary: fmt.Sprintf("%s exists, version %s, %d host(s) attached",
			info.Name, info.Version, info.HostCount),
		Data: map[string]any{
			"switch":     info.Name,
			"version":    info.Version,
			"host_count": info.HostCount,
		},
	}
	checks.Finish(rc, &r)
	return []results.Result{r}, nil
}

// ---------------------------------------------------------------------------

// SwitchMTU asserts the VDS carries at least the MTU the topology needs.
type SwitchMTU struct{}

var _ checks.Check = (*SwitchMTU)(nil)

// Meta implements checks.Check.
func (SwitchMTU) Meta() checks.Meta {
	return checks.Meta{
		ID:             "vc.vds-mtu",
		Title:          "Distributed switch MTU meets the requirement",
		RequirementIDs: []string{"COM-MTU-003"},
		Category:       results.CategoryMTU,
		Layer:          results.LayerSupervisor,
		Severity:       results.SeverityBlocker,
		Modes:          checks.AllModes,
		Needs:          []checks.Capability{checks.CapVCenter},
		Remediation: "Raise the VDS MTU. Note this is a fabric-wide change: the physical underlay " +
			"must already carry it, and the NSX uplink profile has to match. Raising one and not " +
			"the others produces intermittent, size-dependent failures that look like application bugs.",
	}
}

// Run implements checks.Check.
func (c SwitchMTU) Run(ctx context.Context, rc *checks.RunContext) ([]results.Result, error) {
	name := rc.Config.VSphere.DistributedSwitch
	r := checks.NewResult(c.Meta(), rc, name)

	required := requiredMTU(rc.Config)
	r.Expected = results.Value{
		Summary: fmt.Sprintf("MTU >= %d", required),
		Data:    map[string]any{"required_mtu": required},
	}

	if name == "" || required == 0 {
		r.Status = results.StatusSkip
		reason := "no distributed switch declared"
		if required == 0 {
			reason = "no MTU requirement declared for this topology"
		}
		r.Observed = results.Value{Summary: reason, Data: map[string]any{"skip_reason": reason}}
		checks.Finish(rc, &r)
		return []results.Result{r}, nil
	}

	info, err := rc.Clients.VCenter.DistributedSwitch(ctx, name)
	if err != nil {
		r.Status, r.Observed = classify(err, "distributed switch "+name)
		checks.Finish(rc, &r)
		return []results.Result{r}, nil
	}

	r.Observed = results.Value{
		Data: map[string]any{"switch": info.Name, "mtu": info.MTU},
	}
	switch {
	case info.MTU < 0:
		// The property was not populated. We did not measure anything, so we do
		// not get to assert a failure.
		r.Status = results.StatusUnknown
		r.Observed.Summary = "vCenter did not report an MTU for this switch"
		r.Observed.Data["mtu"] = nil
	case info.MTU < required:
		r.Status = results.StatusFail
		r.Observed.Summary = fmt.Sprintf("%s is set to MTU %d, below the required %d",
			info.Name, info.MTU, required)
	default:
		r.Status = results.StatusPass
		r.Observed.Summary = fmt.Sprintf("%s is set to MTU %d", info.Name, info.MTU)
	}

	checks.Finish(rc, &r)
	return []results.Result{r}, nil
}

// requiredMTU returns the MTU the declared topology needs on the VDS.
//
// Under NSX the overlay minimum governs, and it comes from the config rather
// than a constant in here: the VCF 9 figure is flagged unconfirmed in the matrix
// (COM-MTU-001) and hardcoding it would launder a guess into an assertion.
func requiredMTU(cfg *config.Config) int {
	if cfg.Topology.Networking.UsesNSX() && cfg.NSX != nil && cfg.NSX.OverlayMTU > 0 {
		return cfg.NSX.OverlayMTU
	}
	// Otherwise the highest MTU any declared segment expects to carry.
	max := 0
	consider := func(n config.NetworkSpec) {
		if n.MTU > max {
			max = n.MTU
		}
	}
	consider(cfg.Networks.Management)
	for _, w := range cfg.Networks.Workload {
		consider(w)
	}
	if cfg.Networks.Frontend != nil {
		consider(*cfg.Networks.Frontend)
	}
	return max
}

// ---------------------------------------------------------------------------

// PortGroupsExist asserts every declared portgroup is real and on the declared
// VLAN.
type PortGroupsExist struct{}

var _ checks.Check = (*PortGroupsExist)(nil)

// Meta implements checks.Check.
func (PortGroupsExist) Meta() checks.Meta {
	return checks.Meta{
		ID:             "vc.portgroup-exists",
		Title:          "Declared portgroups exist with the declared VLAN",
		RequirementIDs: []string{"VDS-PG-001", "VDS-PG-002", "VDS-PG-003"},
		Category:       results.CategoryInventory,
		Layer:          results.LayerSupervisor,
		Severity:       results.SeverityBlocker,
		Modes:          checks.AllModes,
		Needs:          []checks.Capability{checks.CapVCenter},
		// Portgroups back the workload networks only under VDS networking;
		// under NSX the segments are created by NSX itself.
		Applies:     checks.Applicability{Networking: []config.Networking{config.NetVDS}},
		Remediation: "Create the portgroup on the declared switch, or correct the name and VLAN in the config.",
	}
}

// Run implements checks.Check.
func (c PortGroupsExist) Run(ctx context.Context, rc *checks.RunContext) ([]results.Result, error) {
	type declared struct {
		source string
		name   string
		vlan   int
	}
	var want []declared

	add := func(source string, n config.NetworkSpec) {
		if n.PortGroup != "" {
			want = append(want, declared{source: source, name: n.PortGroup, vlan: n.VLAN})
		}
	}
	add("networks.management", rc.Config.Networks.Management)
	for i, w := range rc.Config.Networks.Workload {
		add(fmt.Sprintf("networks.workload[%d]", i), w)
	}
	if rc.Config.Networks.Frontend != nil {
		add("networks.frontend", *rc.Config.Networks.Frontend)
	}

	if len(want) == 0 {
		r := checks.NewResult(c.Meta(), rc, "")
		r.Status = results.StatusSkip
		r.Expected = results.Text("declared portgroups exist")
		r.Observed = results.Value{
			Summary: "no portgroups declared in the config",
			Data:    map[string]any{"skip_reason": "no networks declare a portGroup"},
		}
		checks.Finish(rc, &r)
		return []results.Result{r}, nil
	}

	// Fan out: one result per declared portgroup, because each is a separate
	// object with a separate fix.
	var out []results.Result
	for _, d := range want {
		r := checks.NewResult(c.Meta(), rc, d.name)
		r.Expected = results.Value{
			Summary: fmt.Sprintf("portgroup %q exists", d.name),
			Data:    map[string]any{"portgroup": d.name, "declared_vlan": d.vlan, "source": d.source},
		}

		info, err := rc.Clients.VCenter.PortGroup(ctx, d.name)
		if err != nil {
			r.Status, r.Observed = classify(err, "portgroup "+d.name)
			if inv, derr := rc.Clients.VCenter.Discover(ctx); derr == nil {
				r.Observed.Data["available_portgroups"] = inv.PortGroups
			}
			checks.Finish(rc, &r)
			out = append(out, r)
			continue
		}

		r.Observed = results.Value{
			Data: map[string]any{
				"portgroup": info.Name,
				"switch":    info.Switch,
				"vlan":      info.VLAN,
				"vlan_kind": info.VLANKind,
			},
		}

		switch {
		case d.vlan != 0 && info.VLANKind == "access" && info.VLAN != d.vlan:
			r.Status = results.StatusFail
			r.Observed.Summary = fmt.Sprintf("%s exists but carries VLAN %d, not the declared %d",
				info.Name, info.VLAN, d.vlan)
			r.Remediation = "Reconcile the portgroup VLAN with the config. One of the two is wrong, " +
				"and a Supervisor on the wrong VLAN fails in ways that point at routing."
		case d.vlan != 0 && info.VLANKind != "access":
			// A trunk is not a mismatch, but it is not the access VLAN the
			// config declared either. We cannot assert it is wrong.
			r.Status = results.StatusUnknown
			r.Observed.Summary = fmt.Sprintf("%s exists but is a %s, so the declared VLAN %d cannot be confirmed",
				info.Name, info.VLANKind, d.vlan)
		default:
			r.Status = results.StatusPass
			r.Observed.Summary = fmt.Sprintf("%s exists on %s (%s VLAN %d)",
				info.Name, orNone(info.Switch), info.VLANKind, info.VLAN)
		}
		checks.Finish(rc, &r)
		out = append(out, r)
	}
	return out, nil
}

// ---------------------------------------------------------------------------

// classify turns a client error into a status and observation.
//
// The distinction that matters: an object the environment does not have is a
// FAIL (a finding the operator can act on), while a failure to look is UNKNOWN
// (we did not observe anything and must not assert we did).
func classify(err error, what string) (results.Status, results.Value) {
	var nf *vc.NotFoundError
	if errors.As(err, &nf) {
		return results.StatusFail, results.Value{
			Summary: what + " does not exist in this vCenter",
			Data:    map[string]any{"found": false, "error": err.Error()},
		}
	}
	return results.StatusUnknown, results.Value{
		Summary: "could not determine whether " + what + " exists: " + err.Error(),
		Data:    map[string]any{"error": err.Error()},
	}
}

func joinOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}

func orNone(s string) string {
	if s == "" {
		return "an unknown switch"
	}
	return s
}
