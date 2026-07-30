package configval

import (
	"net/netip"

	"github.com/modern-apps-5/vks-inspector/internal/config"
	"github.com/modern-apps-5/vks-inspector/internal/netx"
)

// declaredNetworks collects every addressable range the config declares, tagged
// with where it came from.
//
// This is shared by every overlap and containment check, and it is the piece
// most likely to silently under-report: a range this function forgets is a
// collision the tool will never find. When a field is added to the config
// schema, it must be added here in the same change.
//
// Malformed entries are returned separately rather than dropped. A CIDR that
// does not parse is a finding, not a reason to check less.
func declaredNetworks(cfg *config.Config) (nets []netx.Named, bad []badEntry) {
	add := func(source, label, cidr string, routable bool) {
		if cidr == "" {
			return
		}
		p, err := netx.ParsePrefix(cidr)
		if err != nil {
			bad = append(bad, badEntry{Source: source, Value: cidr, Err: err})
			return
		}
		nets = append(nets, netx.Named{Source: source, Label: label, Prefix: p, Routable: routable})
	}

	// addParent records a network's own subnet, tagged with its group so that
	// the ranges carved out of it are not later reported as colliding with it.
	addParent := func(group, source, label, cidr string, routable bool) {
		if cidr == "" {
			return
		}
		p, err := netx.ParsePrefix(cidr)
		if err != nil {
			bad = append(bad, badEntry{Source: source, Value: cidr, Err: err})
			return
		}
		nets = append(nets, netx.Named{
			Source: source, Label: label, Prefix: p, Routable: routable,
			Group: group, IsParent: true,
		})
	}

	addNetwork := func(source string, n config.NetworkSpec) {
		addParent(source, source+".cidr", n.Name, n.CIDR, n.Routable)
		for i, r := range n.Ranges {
			rng, err := netx.ParseRange(r.Start, r.End)
			if err != nil {
				bad = append(bad, badEntry{
					Source: sprintf("%s.ranges[%d]", source, i),
					Value:  r.Start + "-" + r.End,
					Err:    err,
				})
				continue
			}
			label := r.Purpose
			if label == "" {
				label = n.Name
			}
			nets = append(nets, netx.Named{
				Source:   sprintf("%s.ranges[%d]", source, i),
				Label:    label,
				Range:    &rng,
				Routable: n.Routable,
				Group:    source,
			})
		}
	}

	addNetwork("networks.management", cfg.Networks.Management)
	for i, w := range cfg.Networks.Workload {
		addNetwork(sprintf("networks.workload[%d]", i), w)
	}
	if cfg.Networks.Frontend != nil {
		addNetwork("networks.frontend", *cfg.Networks.Frontend)
	}

	for i, c := range cfg.Kubernetes.PodCIDRs {
		add(sprintf("kubernetes.podCIDRs[%d]", i), "pod", c, false)
	}
	add("kubernetes.serviceCIDR", "service", cfg.Kubernetes.ServiceCIDR, false)
	add("kubernetes.ingressCIDR", "ingress", cfg.Kubernetes.IngressCIDR, true)
	add("kubernetes.egressCIDR", "egress", cfg.Kubernetes.EgressCIDR, true)

	if cfg.ALB != nil {
		if cfg.ALB.VIPNetwork != nil {
			addNetwork("alb.vipNetwork", *cfg.ALB.VIPNetwork)
		}
		if cfg.ALB.SEDataNetwork != nil {
			addNetwork("alb.seDataNetwork", *cfg.ALB.SEDataNetwork)
		}
	}
	if cfg.HAProxy != nil {
		add("haproxy.loadBalancerCIDR", "haproxy-vips", cfg.HAProxy.LoadBalancerCIDR, true)
	}
	if cfg.FLB != nil {
		if cfg.FLB.VIPNetwork != nil {
			addNetwork("flb.vipNetwork", *cfg.FLB.VIPNetwork)
		}
		if cfg.FLB.TransitNetwork != nil {
			addNetwork("flb.transitNetwork", *cfg.FLB.TransitNetwork)
		}
	}
	if cfg.NSX != nil && cfg.NSX.VPC != nil {
		for i, c := range cfg.NSX.VPC.PrivateIPBlocks {
			add(sprintf("nsx.vpc.privateIPBlocks[%d]", i), "vpc-private", c, false)
		}
		for i, c := range cfg.NSX.VPC.PublicIPBlocks {
			add(sprintf("nsx.vpc.publicIPBlocks[%d]", i), "vpc-public", c, true)
		}
		for i, c := range cfg.NSX.VPC.ExternalIPBlocks {
			add(sprintf("nsx.vpc.externalIPBlocks[%d]", i), "vpc-external", c, true)
		}
	}
	return nets, bad
}

// externalNetworks collects the ranges the deployment must not collide with.
func externalNetworks(cfg *config.Config) (nets []netx.Named, bad []badEntry) {
	for i, c := range cfg.Kubernetes.ExternalCIDRs {
		p, err := netx.ParsePrefix(c)
		if err != nil {
			bad = append(bad, badEntry{
				Source: sprintf("kubernetes.externalCIDRs[%d]", i), Value: c, Err: err,
			})
			continue
		}
		nets = append(nets, netx.Named{
			Source: sprintf("kubernetes.externalCIDRs[%d]", i), Label: "external", Prefix: p,
		})
	}
	return nets, bad
}

// infraAddress is a management-plane endpoint the cluster must be able to reach.
type infraAddress struct {
	Source string
	Name   string
	Addr   netip.Addr
}

// infraAddresses collects the declared IP addresses of infrastructure the
// cluster has to talk to.
//
// Only addresses declared literally in the config are returned. Endpoints given
// as FQDNs are invisible here — resolving them is a network probe, which is
// class (a), not class (c). A collision against an unresolved FQDN is therefore
// something this check cannot see, and the result says so rather than implying
// full coverage.
func infraAddresses(cfg *config.Config) []infraAddress {
	var out []infraAddress

	add := func(source, name, ip string) {
		if ip == "" {
			return
		}
		a, err := netip.ParseAddr(ip)
		if err != nil {
			return // malformed endpoint IPs are reported by their own check
		}
		out = append(out, infraAddress{Source: source, Name: name, Addr: a})
	}

	add("infrastructure.vcenter.ip", cfg.Infrastructure.VCenter.FQDN, cfg.Infrastructure.VCenter.IP)
	if cfg.Infrastructure.NSXManager != nil {
		add("infrastructure.nsxManager.ip", cfg.Infrastructure.NSXManager.FQDN, cfg.Infrastructure.NSXManager.IP)
	}
	for i, h := range cfg.Infrastructure.ESXiHosts {
		add(sprintf("infrastructure.esxiHosts[%d].ip", i), h.FQDN, h.IP)
	}
	if cfg.Infrastructure.Registry != nil {
		add("infrastructure.registry.ip", cfg.Infrastructure.Registry.FQDN, cfg.Infrastructure.Registry.IP)
	}
	for i, s := range cfg.Services.DNS.Servers {
		add(sprintf("services.dns.servers[%d]", i), "dns", s)
	}
	for i, s := range cfg.Services.NTP.Servers {
		add(sprintf("services.ntp.servers[%d]", i), "ntp", s)
	}
	if cfg.ALB != nil {
		add("alb.controller.ip", cfg.ALB.Controller.FQDN, cfg.ALB.Controller.IP)
		for i, a := range cfg.ALB.ControllerCluster {
			add(sprintf("alb.controllerCluster[%d]", i), "alb-controller", a)
		}
	}
	return out
}

// badEntry is a config value that could not be parsed.
type badEntry struct {
	Source string
	Value  string
	Err    error
}
