package network

import (
	"fmt"
	"net/netip"

	"github.com/modern-apps-5/vks-inspector/internal/config"
)

// endpoint is one thing the tool probes, with where in the config it came from.
type endpoint struct {
	Source string
	// Name is the declared FQDN, empty when only an address was given.
	Name string
	// IP is the declared address, empty when only a name was given.
	IP string
	// Port is the management port; 443 unless the config says otherwise.
	Port int
	// Required marks an endpoint the deployment cannot work without, as opposed
	// to one declared for completeness.
	Required bool
}

// Address returns "host:port", preferring the name so the probe exercises the
// same resolution path the deployment will.
func (e endpoint) Address() string {
	host := e.Name
	if host == "" {
		host = e.IP
	}
	port := e.Port
	if port == 0 {
		port = 443
	}
	return fmt.Sprintf("%s:%d", host, port)
}

// Host returns the name if there is one, else the address.
func (e endpoint) Host() string {
	if e.Name != "" {
		return e.Name
	}
	return e.IP
}

// managementEndpoints collects everything with a management plane worth
// probing.
//
// A missing entry here is a path the tool will never test, so adding a field to
// the config schema means adding it here in the same change.
func managementEndpoints(cfg *config.Config) []endpoint {
	var out []endpoint

	add := func(source string, ep config.Endpoint, required bool) {
		if ep.FQDN == "" && ep.IP == "" {
			return
		}
		out = append(out, endpoint{
			Source: source, Name: ep.FQDN, IP: ep.IP, Port: ep.Port, Required: required,
		})
	}

	add("infrastructure.vcenter", cfg.Infrastructure.VCenter, true)
	if cfg.Infrastructure.NSXManager != nil && cfg.Topology.Networking.UsesNSX() {
		add("infrastructure.nsxManager", *cfg.Infrastructure.NSXManager, true)
	}
	for i, h := range cfg.Infrastructure.ESXiHosts {
		add(fmt.Sprintf("infrastructure.esxiHosts[%d]", i), h, false)
	}
	if cfg.Infrastructure.Registry != nil {
		add("infrastructure.registry", *cfg.Infrastructure.Registry, false)
	}
	if cfg.ALB != nil && cfg.Topology.UsesALB() {
		add("alb.controller", cfg.ALB.Controller, true)
	}
	if cfg.HAProxy != nil && cfg.Topology.UsesHAProxy() {
		ep := cfg.HAProxy.Appliance
		if ep.Port == 0 {
			ep.Port = cfg.HAProxy.DataPlanePort
		}
		add("haproxy.appliance", ep, true)
	}
	for i, e := range cfg.Infrastructure.Extra {
		add(fmt.Sprintf("infrastructure.extra[%d]", i), e, false)
	}
	return out
}

// resolvableNames returns every name that must resolve, with its source.
type namedTarget struct {
	Source string
	Name   string
	// ExpectIP is the address the config says the name should resolve to.
	// Empty means "any answer will do".
	ExpectIP string
}

func resolvableNames(cfg *config.Config) []namedTarget {
	var out []namedTarget
	seen := map[string]bool{}

	add := func(source, name, expect string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, namedTarget{Source: source, Name: name, ExpectIP: expect})
	}

	for _, e := range managementEndpoints(cfg) {
		add(e.Source, e.Name, e.IP)
	}
	for i, n := range cfg.Services.DNS.AdditionalNames {
		add(fmt.Sprintf("services.dns.additionalNames[%d]", i), n, "")
	}
	return out
}

// reverseTargets returns the addresses whose PTR records matter, with the name
// each should resolve back to.
func reverseTargets(cfg *config.Config) []namedTarget {
	var out []namedTarget
	for _, e := range managementEndpoints(cfg) {
		if e.IP == "" || e.Name == "" {
			continue // nothing to compare
		}
		if _, err := netip.ParseAddr(e.IP); err != nil {
			continue
		}
		out = append(out, namedTarget{Source: e.Source, Name: e.Name, ExpectIP: e.IP})
	}
	return out
}

// resolvers returns the DNS servers to query, or a single empty string meaning
// "the system resolver" when none are declared.
func resolvers(cfg *config.Config) []string {
	if len(cfg.Services.DNS.Servers) == 0 {
		return []string{""}
	}
	return cfg.Services.DNS.Servers
}
