package prompt

import (
	"fmt"
	"net/netip"
	"strconv"

	"github.com/modern-apps-5/vks-inspector/internal/config"
	"github.com/modern-apps-5/vks-inspector/internal/netx"
)

// Elicit fills in the gaps in a config by asking.
//
// It takes a partially-populated config — from flags, from a partial file, or
// empty — and returns one complete enough to run against. Anything already set
// is left alone and never re-asked; that is what makes `--config partial.yaml`
// plus a couple of prompts a sensible workflow rather than an all-or-nothing
// choice.
//
// Discovered is the set of values already learned from vCenter. Those are
// reported, not asked, and the operator can see what was filled in for them.
func Elicit(p *Prompter, cfg *config.Config, discovered *Discovered) error {
	if cfg.APIVersion == "" {
		cfg.APIVersion = config.APIVersion
	}
	if cfg.Kind == "" {
		cfg.Kind = config.Kind
	}

	if discovered != nil && discovered.Any() {
		p.Section("Discovered from vCenter")
		for _, line := range discovered.Lines() {
			p.Info("%s", line)
		}
		p.Info("")
		p.Info("Values above were read from vCenter and are not asked for below.")
	}

	if err := elicitIdentity(p, cfg); err != nil {
		return err
	}
	if err := elicitTopology(p, cfg); err != nil {
		return err
	}
	if err := elicitServices(p, cfg); err != nil {
		return err
	}
	if err := elicitNetworks(p, cfg); err != nil {
		return err
	}
	if err := elicitTopologySpecific(p, cfg); err != nil {
		return err
	}
	return elicitScale(p, cfg)
}

func elicitIdentity(p *Prompter, cfg *config.Config) error {
	if cfg.Metadata.Name != "" {
		return nil
	}
	p.Section("Environment")
	name, err := p.Ask("Name for this environment (used in reports and baselines)", "", nil)
	if err != nil {
		return err
	}
	cfg.Metadata.Name = name
	return nil
}

func elicitTopology(p *Prompter, cfg *config.Config) error {
	if cfg.Topology.Valid() {
		return nil
	}
	p.Section("Topology")

	if cfg.Topology.Networking == "" {
		choices := make([]Choice, 0, len(config.AllNetworking))
		for _, n := range config.AllNetworking {
			c := Choice{Value: string(n), Label: n.Description()}
			if n == config.NetNSXVPC {
				c.Note = "every VPC requirement in the matrix is flagged as unverified"
			}
			choices = append(choices, c)
		}
		v, err := p.Select("Networking", choices, "")
		if err != nil {
			return err
		}
		cfg.Topology.Networking = config.Networking(v)
	}

	if cfg.Topology.LoadBalancer == "" {
		// Only offer load balancers valid with the chosen networking, so the
		// prompt can never produce a combination the loader then rejects.
		valid := config.LoadBalancersFor(cfg.Topology.Networking)
		if len(valid) == 0 {
			return fmt.Errorf("no supported load balancer for networking %q", cfg.Topology.Networking)
		}
		choices := make([]Choice, 0, len(valid))
		for _, lb := range valid {
			choices = append(choices, Choice{
				Value: string(lb),
				Label: lb.Description(),
				Note:  config.Topology{Networking: cfg.Topology.Networking, LoadBalancer: lb}.Note(),
			})
		}
		v, err := p.Select("Load balancer", choices, "")
		if err != nil {
			return err
		}
		cfg.Topology.LoadBalancer = config.LoadBalancer(v)
	}

	if err := cfg.Topology.Validate(); err != nil {
		return fmt.Errorf("topology: %w", err)
	}
	return nil
}

func elicitServices(p *Prompter, cfg *config.Config) error {
	needDNS := len(cfg.Services.DNS.Servers) == 0
	needNTP := len(cfg.Services.NTP.Servers) == 0
	if !needDNS && !needNTP {
		return nil
	}
	p.Section("Infrastructure services")

	if needDNS {
		servers, err := p.AskList("DNS servers (comma separated)", nil, validAddr)
		if err != nil {
			return err
		}
		cfg.Services.DNS.Servers = servers
	}
	if needNTP {
		servers, err := p.AskList("NTP servers (comma separated)", nil, nil)
		if err != nil {
			return err
		}
		cfg.Services.NTP.Servers = servers
	}
	if cfg.Services.NTP.MaxSkewSeconds == 0 {
		// FLAGGED: this default is a field heuristic, not a sourced product
		// requirement (matrix row COM-NTP-002). It is prompted rather than
		// silently applied so the operator knows a judgement call was made.
		v, err := p.Ask("Maximum tolerated clock skew, seconds (field heuristic, not a documented product limit)",
			"30", validPositiveInt)
		if err != nil {
			return err
		}
		cfg.Services.NTP.MaxSkewSeconds, _ = strconv.Atoi(v)
	}
	return nil
}

func elicitNetworks(p *Prompter, cfg *config.Config) error {
	p.Section("Addressing")

	if cfg.Networks.Management.CIDR == "" {
		cidr, err := p.Ask("Management network CIDR (Supervisor control plane VMs)", "", validCIDR)
		if err != nil {
			return err
		}
		cfg.Networks.Management.Name = "management"
		cfg.Networks.Management.CIDR = cidr
		cfg.Networks.Management.Routable = true

		gw, err := p.Ask("Management network gateway", "", validAddr)
		if err != nil {
			return err
		}
		cfg.Networks.Management.Gateway = gw
	}

	if len(cfg.Networks.Management.Ranges) == 0 {
		// FLAGGED: matrix row SUP-MGT-001. The "five consecutive addresses"
		// rule is carried forward from the vSphere-with-Tanzu era and is not
		// confirmed for VCF 9, so the prompt states it as an unconfirmed
		// convention rather than presenting it as a requirement.
		start, err := p.Ask("Start of the Supervisor control plane address range", "", validAddr)
		if err != nil {
			return err
		}
		end, err := p.Ask("End of that range (conventionally 5 consecutive addresses — unconfirmed for VCF 9)", "", validAddr)
		if err != nil {
			return err
		}
		cfg.Networks.Management.Ranges = []config.IPRange{
			{Start: start, End: end, Purpose: "supervisor-control-plane"},
		}
	}

	if len(cfg.Networks.Workload) == 0 {
		cidr, err := p.Ask("Workload network CIDR (Supervisor and VKS cluster nodes)", "", validCIDR)
		if err != nil {
			return err
		}
		w := config.NetworkSpec{Name: "workload-primary", CIDR: cidr, Routable: true}
		if gw, err := p.AskOptional("Workload network gateway", validAddr); err == nil && gw != "" {
			w.Gateway = gw
		}
		cfg.Networks.Workload = append(cfg.Networks.Workload, w)
	}

	if len(cfg.Kubernetes.PodCIDRs) == 0 {
		v, err := p.Ask("Pod CIDR (cluster-internal)", "10.244.0.0/20", validCIDR)
		if err != nil {
			return err
		}
		cfg.Kubernetes.PodCIDRs = []string{v}
	}
	if cfg.Kubernetes.ServiceCIDR == "" {
		v, err := p.Ask("Service CIDR (cluster-internal)", "10.96.0.0/22", validCIDR)
		if err != nil {
			return err
		}
		cfg.Kubernetes.ServiceCIDR = v
	}

	if len(cfg.Kubernetes.ExternalCIDRs) == 0 {
		// Asked explicitly, with the consequence stated, because an empty list
		// silently reduces cidr.external-collision to a no-op and the operator
		// deserves to know that before the report says "no collisions".
		v, err := p.AskList(
			"Existing networks this deployment must not collide with, comma separated\n"+
				"    (leave empty to skip collision detection entirely)", nil, validCIDRList)
		if err != nil {
			return err
		}
		cfg.Kubernetes.ExternalCIDRs = v
	}
	return nil
}

func elicitTopologySpecific(p *Prompter, cfg *config.Config) error {
	t := cfg.Topology

	if t.Networking.UsesNSX() {
		if cfg.NSX == nil {
			cfg.NSX = &config.NSX{}
		}
		p.Section("NSX")
		if cfg.NSX.Tier0Gateway == "" && t.Networking == config.NetNSX {
			v, err := p.Ask("Tier-0 gateway name", "", nil)
			if err != nil {
				return err
			}
			cfg.NSX.Tier0Gateway = v
		}
		if cfg.NSX.OverlayMTU == 0 {
			// FLAGGED: matrix row COM-MTU-001. 1600 is the long-standing Geneve
			// minimum; the VCF 9 requirement is unconfirmed and 9000 is common
			// in practice. Prompted rather than hardcoded.
			v, err := p.Ask("Required overlay (Geneve) MTU on the underlay", "1700", validPositiveInt)
			if err != nil {
				return err
			}
			cfg.NSX.OverlayMTU, _ = strconv.Atoi(v)
		}
		if t.Networking == config.NetNSX {
			if cfg.Kubernetes.IngressCIDR == "" {
				v, err := p.Ask("Ingress CIDR (load balancer VIPs)", "", validCIDR)
				if err != nil {
					return err
				}
				cfg.Kubernetes.IngressCIDR = v
			}
			if cfg.Kubernetes.EgressCIDR == "" {
				v, err := p.Ask("Egress CIDR (SNAT addresses)", "", validCIDR)
				if err != nil {
					return err
				}
				cfg.Kubernetes.EgressCIDR = v
			}
		}
		if t.Networking == config.NetNSXVPC && cfg.NSX.VPC == nil {
			cfg.NSX.VPC = &config.NSXVPC{}
			p.Info("%s VPC-based networking: every requirement is flagged unverified in the matrix.", flagMark)
			p.Info("  Only reachability and address-plan checks will produce meaningful results.")
		}
	}

	if t.UsesALB() {
		if cfg.ALB == nil {
			cfg.ALB = &config.ALB{}
		}
		p.Section("NSX Advanced Load Balancer")
		if cfg.ALB.Controller.FQDN == "" {
			v, err := p.Ask("ALB controller FQDN or address", "", nil)
			if err != nil {
				return err
			}
			cfg.ALB.Controller.FQDN = v
			cfg.ALB.Controller.CredentialRef = "alb"
		}
		if cfg.ALB.VIPNetwork == nil {
			cidr, err := p.Ask("VIP network CIDR", "", validCIDR)
			if err != nil {
				return err
			}
			vip := &config.NetworkSpec{Name: "alb-vip", CIDR: cidr, Routable: true}
			start, err := p.AskOptional("VIP pool start address", validAddr)
			if err != nil {
				return err
			}
			if start != "" {
				end, err := p.Ask("VIP pool end address", "", validAddr)
				if err != nil {
					return err
				}
				vip.Ranges = []config.IPRange{{Start: start, End: end, Purpose: "load-balancer-vips"}}
			}
			cfg.ALB.VIPNetwork = vip
		}
	}

	if t.UsesHAProxy() {
		if cfg.HAProxy == nil {
			cfg.HAProxy = &config.HAProxy{}
		}
		p.Section("HAProxy")
		p.Info("%s HAProxy is believed deprecated in the VCF 9 generation (matrix row LB-HAP-000).", flagMark)
		if cfg.HAProxy.Appliance.FQDN == "" {
			v, err := p.Ask("HAProxy appliance FQDN or address", "", nil)
			if err != nil {
				return err
			}
			cfg.HAProxy.Appliance.FQDN = v
			cfg.HAProxy.Appliance.CredentialRef = "haproxy"
		}
		if cfg.HAProxy.LoadBalancerCIDR == "" {
			v, err := p.Ask("Load balancer VIP CIDR", "", validCIDR)
			if err != nil {
				return err
			}
			cfg.HAProxy.LoadBalancerCIDR = v
		}
	}
	return nil
}

func elicitScale(p *Prompter, cfg *config.Config) error {
	if cfg.Scale.SupervisorControlPlaneNodes != 0 {
		return nil
	}
	p.Section("Expected scale")
	p.Info("Used by pool-sizing checks. Answer as best you can — a sizing check")
	p.Info("with no declared scale reports as skipped, never as passed.")

	cfg.Scale.SupervisorControlPlaneNodes = 3
	for _, q := range []struct {
		question string
		def      string
		set      func(int)
	}{
		{"Expected number of vSphere namespaces", "20", func(v int) { cfg.Scale.ExpectedNamespaces = v }},
		{"Expected number of VKS workload clusters", "5", func(v int) { cfg.Scale.ExpectedWorkloadClusters = v }},
		{"Expected nodes per workload cluster", "6", func(v int) { cfg.Scale.ExpectedNodesPerCluster = v }},
		{"Expected LoadBalancer-type Services", "30", func(v int) { cfg.Scale.ExpectedLoadBalancerServices = v }},
		{"Growth headroom percent", "30", func(v int) { cfg.Scale.GrowthHeadroomPercent = v }},
	} {
		v, err := p.Ask(q.question, q.def, validPositiveInt)
		if err != nil {
			return err
		}
		n, _ := strconv.Atoi(v)
		q.set(n)
	}
	return nil
}

// Discovered holds values read from vCenter so they can be reported rather than
// asked for.
type Discovered struct {
	VCenterVersion string
	Datacenter     string
	Cluster        string
	Switches       []string
	NSXManager     string
	Hosts          int
}

// Any reports whether anything was discovered.
func (d *Discovered) Any() bool {
	return d != nil && (d.VCenterVersion != "" || d.Datacenter != "" || d.Cluster != "" ||
		len(d.Switches) > 0 || d.NSXManager != "" || d.Hosts > 0)
}

// Lines renders the discovery summary.
func (d *Discovered) Lines() []string {
	var out []string
	if d.VCenterVersion != "" {
		out = append(out, "vCenter        "+d.VCenterVersion)
	}
	if d.Datacenter != "" {
		out = append(out, "datacenter     "+d.Datacenter)
	}
	if d.Cluster != "" {
		out = append(out, fmt.Sprintf("cluster        %s (%d hosts)", d.Cluster, d.Hosts))
	}
	for i, s := range d.Switches {
		label := "switches"
		if i > 0 {
			label = ""
		}
		out = append(out, fmt.Sprintf("%-14s %s", label, s))
	}
	if d.NSXManager != "" {
		out = append(out, "NSX Manager    "+d.NSXManager)
	}
	return out
}

func validCIDR(s string) error {
	_, err := netx.ParsePrefix(s)
	return err
}

func validCIDRList(s string) error { return validCIDR(s) }

func validAddr(s string) error {
	if _, err := netip.ParseAddr(s); err != nil {
		return fmt.Errorf("%q is not a valid IP address", s)
	}
	return nil
}

func validPositiveInt(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return fmt.Errorf("%q is not a positive number", s)
	}
	return nil
}
