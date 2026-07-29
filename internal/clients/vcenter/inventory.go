package vcenter

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

// Inventory is what a single discovery pass learns.
//
// Everything here is read from vCenter rather than asked of the operator — see
// docs/ADR/0014. Retyping a cluster name is both tedious and a source of error:
// a typo produces a check that confidently inspects the wrong object.
type Inventory struct {
	Version     string   `json:"version"`
	Build       string   `json:"build"`
	Datacenters []string `json:"datacenters"`
	Clusters    []string `json:"clusters"`
	Switches    []string `json:"distributed_switches"`
	PortGroups  []string `json:"portgroups,omitempty"`
	HostCount   int      `json:"host_count"`
	// NSXManager is the NSX Manager registered with this vCenter, if any. Its
	// presence is also a signal about the topology the environment actually
	// has, independent of what the operator declared.
	NSXManager string `json:"nsx_manager,omitempty"`
}

// Discover reads the inventory in one pass.
//
// Best-effort by design: a failure in any one area yields an empty field rather
// than an error, because discovery runs before the prompt flow and must degrade
// to asking rather than aborting the run.
func (c *Client) Discover(ctx context.Context) (*Inventory, error) {
	gc, err := c.connected(ctx)
	if err != nil {
		return nil, err
	}
	f, err := c.find(ctx)
	if err != nil {
		return nil, err
	}

	inv := &Inventory{
		Version: gc.Client.ServiceContent.About.Version,
		Build:   gc.Client.ServiceContent.About.Build,
	}

	// Container views walk recursively from the root folder. The finder's path
	// globs are relative to a datacenter that has not been chosen yet — this is
	// discovery, so there is nothing to be relative to.
	if names, err := c.names(ctx, "Datacenter"); err == nil {
		inv.Datacenters = names
	}
	if names, err := c.names(ctx, "ClusterComputeResource"); err == nil {
		inv.Clusters = names
	}
	if names, err := c.names(ctx, "DistributedVirtualSwitch"); err == nil {
		inv.Switches = names
	}
	if names, err := c.names(ctx, "DistributedVirtualPortgroup"); err == nil {
		inv.PortGroups = names
	}
	if names, err := c.names(ctx, "HostSystem"); err == nil {
		inv.HostCount = len(names)
	}
	_ = f
	inv.NSXManager = c.registeredNSXManager(ctx)

	return inv, nil
}

// entity is a managed object reduced to what lookup needs.
type entity struct {
	Ref  types.ManagedObjectReference
	Name string
}

// entities lists every managed object of a given type, recursively from the
// root folder, with its name resolved.
//
// The view is always destroyed: leaving container views behind on a customer's
// vCenter is the same discourtesy as leaving sessions behind.
//
// Portgroups need special handling. Their ManagedEntity "name" property is not
// always populated — vcsim leaves it empty and only sets config.name — and an
// object with an empty name silently matches an empty lookup string, which is
// how a test can pass while proving nothing. Read both and prefer whichever is
// set.
func (c *Client) entities(ctx context.Context, kind string) ([]entity, error) {
	gc, err := c.connected(ctx)
	if err != nil {
		return nil, err
	}
	m := view.NewManager(gc.Client)
	v, err := m.CreateContainerView(ctx, gc.Client.ServiceContent.RootFolder, []string{kind}, true)
	if err != nil {
		return nil, err
	}
	defer func() { _ = v.Destroy(ctx) }()

	var out []entity

	if kind == "DistributedVirtualPortgroup" {
		var pgs []mo.DistributedVirtualPortgroup
		if err := v.Retrieve(ctx, []string{kind}, []string{"name", "config.name"}, &pgs); err != nil {
			return nil, err
		}
		for i := range pgs {
			name := pgs[i].Name
			if name == "" {
				name = pgs[i].Config.Name
			}
			if name == "" {
				continue // unnameable object: never offer it as a match
			}
			out = append(out, entity{Ref: pgs[i].Reference(), Name: name})
		}
	} else {
		var objs []mo.ManagedEntity
		if err := v.Retrieve(ctx, []string{kind}, []string{"name"}, &objs); err != nil {
			return nil, err
		}
		for i := range objs {
			if objs[i].Name == "" {
				continue
			}
			out = append(out, entity{Ref: objs[i].Reference(), Name: objs[i].Name})
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// names lists object names of a given type, sorted, so reports and baselines
// stay comparable.
func (c *Client) names(ctx context.Context, kind string) ([]string, error) {
	ents, err := c.entities(ctx, kind)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ents))
	for _, e := range ents {
		out = append(out, e.Name)
	}
	return out, nil
}

// registeredNSXManager returns the NSX Manager registered as a vCenter
// extension, if one is. Empty string means "none found", not "error" — the
// caller treats absence as a fact about the environment, not a failure.
func (c *Client) registeredNSXManager(ctx context.Context) string {
	gc, err := c.connected(ctx)
	if err != nil {
		return ""
	}
	em := object.NewExtensionManager(gc.Client)
	exts, err := em.List(ctx)
	if err != nil {
		return ""
	}
	for _, e := range exts {
		if !strings.Contains(strings.ToLower(e.Key), "nsx") {
			continue
		}
		for _, s := range e.Server {
			if s.Url != "" {
				return s.Url
			}
		}
		if e.Description != nil {
			if l := e.Description.GetDescription(); l != nil && l.Label != "" {
				return l.Label
			}
		}
	}
	return ""
}

// ClusterInfo describes a compute cluster.
type ClusterInfo struct {
	Name       string   `json:"name"`
	Datacenter string   `json:"datacenter"`
	Hosts      []string `json:"hosts"`
	HostCount  int      `json:"host_count"`
	DRSEnabled bool     `json:"drs_enabled"`
	HAEnabled  bool     `json:"ha_enabled"`
}

// Cluster looks up a cluster by name, optionally within a named datacenter.
//
// Returns a not-found error the caller can distinguish, because "the cluster
// does not exist" is a finding about the environment while "we could not reach
// vCenter" is a statement about the tool's access, and they must not be
// reported the same way.
func (c *Client) Cluster(ctx context.Context, datacenter, name string) (*ClusterInfo, error) {
	f, err := c.find(ctx)
	if err != nil {
		return nil, err
	}
	if datacenter != "" {
		dc, err := f.Datacenter(ctx, datacenter)
		if err != nil {
			return nil, notFound("datacenter", datacenter, err)
		}
		f.SetDatacenter(dc)
	}

	cl, err := f.ClusterComputeResource(ctx, name)
	if err != nil {
		return nil, notFound("cluster", name, err)
	}

	var mcl mo.ClusterComputeResource
	pc := property.DefaultCollector(cl.Client())
	if err := pc.RetrieveOne(ctx, cl.Reference(),
		[]string{"name", "host", "configurationEx"}, &mcl); err != nil {
		return nil, fmt.Errorf("read cluster %s: %w", name, err)
	}

	info := &ClusterInfo{Name: mcl.Name, Datacenter: datacenter, HostCount: len(mcl.Host)}
	if cfg, ok := mcl.ConfigurationEx.(*types.ClusterConfigInfoEx); ok && cfg != nil {
		if cfg.DrsConfig.Enabled != nil {
			info.DRSEnabled = *cfg.DrsConfig.Enabled
		}
		if cfg.DasConfig.Enabled != nil {
			info.HAEnabled = *cfg.DasConfig.Enabled
		}
	}
	for _, ref := range mcl.Host {
		var h mo.HostSystem
		if err := pc.RetrieveOne(ctx, ref, []string{"name"}, &h); err == nil {
			info.Hosts = append(info.Hosts, h.Name)
		}
	}
	sort.Strings(info.Hosts)
	return info, nil
}

// SwitchInfo describes a distributed virtual switch.
type SwitchInfo struct {
	Name      string   `json:"name"`
	MTU       int      `json:"mtu"`
	Version   string   `json:"version"`
	HostCount int      `json:"host_count"`
	Hosts     []string `json:"hosts,omitempty"`
}

// DistributedSwitch looks up a VDS by name.
//
// Looked up by container view rather than the finder: finder paths are relative
// to a datacenter, and a caller who only knows a switch name should not have to
// know which datacenter holds it.
func (c *Client) DistributedSwitch(ctx context.Context, name string) (*SwitchInfo, error) {
	gc, err := c.connected(ctx)
	if err != nil {
		return nil, err
	}
	ref, err := c.refByName(ctx, "DistributedVirtualSwitch", name)
	if err != nil {
		return nil, notFound("distributed switch", name, err)
	}

	// A switch may be reported as the VMware subclass or as the base type
	// depending on the server, and unmarshalling into the wrong one panics
	// inside the property collector. Dispatch on what the reference says it is.
	pc := property.DefaultCollector(gc.Client)
	var (
		dvsName string
		dvsCfg  types.BaseDVSConfigInfo
	)
	switch ref.Type {
	case "VmwareDistributedVirtualSwitch":
		var m1 mo.VmwareDistributedVirtualSwitch
		if err := pc.RetrieveOne(ctx, *ref, []string{"name", "config"}, &m1); err != nil {
			return nil, fmt.Errorf("read distributed switch %s: %w", name, err)
		}
		dvsName, dvsCfg = m1.Name, m1.Config
	default:
		var m2 mo.DistributedVirtualSwitch
		if err := pc.RetrieveOne(ctx, *ref, []string{"name", "config"}, &m2); err != nil {
			return nil, fmt.Errorf("read distributed switch %s: %w", name, err)
		}
		dvsName, dvsCfg = m2.Name, m2.Config
	}

	info := &SwitchInfo{Name: dvsName, MTU: -1}
	if dvsCfg != nil {
		if base := dvsCfg.GetDVSConfigInfo(); base != nil {
			info.Version = base.ProductInfo.Version
			info.HostCount = len(base.Host)
			// MaxMtu is on the VMware-specific config, not the base interface.
			// MTU stays -1 when it cannot be read, so a check reports "could
			// not determine" rather than asserting a switch is set to 0.
			// Guard on > 0: a switch cannot have an MTU of zero, so zero means
			// the property was not populated. Reporting it as a real value
			// would make an MTU check fail an environment it never measured.
			if vcfg, ok := dvsCfg.(*types.VMwareDVSConfigInfo); ok && vcfg != nil && vcfg.MaxMtu > 0 {
				info.MTU = int(vcfg.MaxMtu)
			}
		}
	}
	return info, nil
}

// PortGroupInfo describes a distributed portgroup.
type PortGroupInfo struct {
	Name   string `json:"name"`
	Switch string `json:"switch,omitempty"`
	// VLAN is the access VLAN ID. VLANKind distinguishes an access VLAN from a
	// trunk or a private VLAN, because "vlan: 0" means something different in
	// each and a check that ignores the distinction will misreport a trunk.
	VLAN      int    `json:"vlan"`
	VLANKind  string `json:"vlan_kind"`
	VLANRange string `json:"vlan_range,omitempty"`
	PortCount int    `json:"port_count"`
	Uplink    bool   `json:"uplink"`
}

// PortGroup looks up a distributed portgroup by name.
func (c *Client) PortGroup(ctx context.Context, name string) (*PortGroupInfo, error) {
	gc, err := c.connected(ctx)
	if err != nil {
		return nil, err
	}
	ref, err := c.refByName(ctx, "DistributedVirtualPortgroup", name)
	if err != nil {
		return nil, notFound("portgroup", name, err)
	}

	var mpg mo.DistributedVirtualPortgroup
	pc := property.DefaultCollector(gc.Client)
	if err := pc.RetrieveOne(ctx, *ref, []string{"name", "config"}, &mpg); err != nil {
		return nil, fmt.Errorf("read portgroup %s: %w", name, err)
	}

	info := &PortGroupInfo{Name: mpg.Name, VLAN: -1, VLANKind: "unknown"}
	info.PortCount = int(mpg.Config.NumPorts)
	if mpg.Config.Uplink != nil {
		info.Uplink = *mpg.Config.Uplink
	}
	if mpg.Config.DistributedVirtualSwitch != nil {
		var sw mo.ManagedEntity
		if err := pc.RetrieveOne(ctx, *mpg.Config.DistributedVirtualSwitch, []string{"name"}, &sw); err == nil {
			info.Switch = sw.Name
		}
	}

	if setting, ok := mpg.Config.DefaultPortConfig.(*types.VMwareDVSPortSetting); ok && setting != nil {
		switch v := setting.Vlan.(type) {
		case *types.VmwareDistributedVirtualSwitchVlanIdSpec:
			info.VLAN, info.VLANKind = int(v.VlanId), "access"
		case *types.VmwareDistributedVirtualSwitchTrunkVlanSpec:
			info.VLANKind = "trunk"
			var parts []string
			for _, r := range v.VlanId {
				parts = append(parts, fmt.Sprintf("%d-%d", r.Start, r.End))
			}
			info.VLANRange = strings.Join(parts, ",")
		case *types.VmwareDistributedVirtualSwitchPvlanSpec:
			info.VLAN, info.VLANKind = int(v.PvlanId), "private"
		}
	}
	return info, nil
}

// refByName finds a managed object of a given type by name, anywhere in the
// inventory.
//
// An empty name never matches: an object whose name could not be read must not
// be silently returned for a blank lookup.
func (c *Client) refByName(ctx context.Context, kind, name string) (*types.ManagedObjectReference, error) {
	if name == "" {
		return nil, fmt.Errorf("no %s name given", kind)
	}
	ents, err := c.entities(ctx, kind)
	if err != nil {
		return nil, err
	}
	for i := range ents {
		if ents[i].Name == name {
			return &ents[i].Ref, nil
		}
	}
	return nil, fmt.Errorf("no %s named %q", kind, name)
}

// NotFoundError marks an object the environment does not have, as opposed to a
// failure to look.
type NotFoundError struct {
	Kind string
	Name string
	err  error
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s %q not found in vCenter", e.Kind, e.Name)
}
func (e *NotFoundError) Unwrap() error { return e.err }

func notFound(kind, name string, err error) error {
	return &NotFoundError{Kind: kind, Name: name, err: err}
}
