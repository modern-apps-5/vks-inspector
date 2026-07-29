package config

import "fmt"

// Topology is the deployment shape the environment is meant to be. It is the
// primary axis of the requirements matrix: almost every requirement applies to
// some topologies and not others, and a check that cannot say which topologies
// it applies to will be run in the wrong environment and produce a false
// failure.
type Topology string

const (
	// TopologyNSX — Supervisor on NSX networking. NSX provides both the
	// networking and the L4 load balancing.
	TopologyNSX Topology = "nsx"
	// TopologyNSXALB — Supervisor on NSX networking with NSX Advanced Load
	// Balancer (Avi) providing load balancing instead of the NSX built-in LB.
	// Present in the existing README's option list; not named in the phase-1
	// brief. Kept because it is a real supported shape.
	TopologyNSXALB Topology = "nsx-alb"
	// TopologyVDSALB — Supervisor on vSphere (VDS) networking with NSX ALB.
	TopologyVDSALB Topology = "vds-alb"
	// TopologyVDSHAProxy — Supervisor on vSphere (VDS) networking with the
	// HAProxy appliance as load balancer.
	// FLAGGED: believed deprecated/removed in the VCF 9 generation. See
	// docs/REQUIREMENTS-MATRIX.md row group LB-HAP.
	TopologyVDSHAProxy Topology = "vds-haproxy"
	// TopologyNSXVPC — VPC-based NSX networking (VCF 9). Lowest-confidence
	// topology in the matrix; every requirement under it is flagged.
	TopologyNSXVPC Topology = "nsx-vpc"
)

// AllTopologies is the canonical ordered list. Renderers and docs iterate this
// so a new topology cannot be half-added.
var AllTopologies = []Topology{
	TopologyNSX,
	TopologyNSXALB,
	TopologyVDSALB,
	TopologyVDSHAProxy,
	TopologyNSXVPC,
}

// Description is used by `explain` and by help text.
func (t Topology) Description() string {
	switch t {
	case TopologyNSX:
		return "Supervisor on NSX networking (NSX-provided L4 load balancing)"
	case TopologyNSXALB:
		return "Supervisor on NSX networking with NSX Advanced Load Balancer"
	case TopologyVDSALB:
		return "Supervisor on vSphere (VDS) networking with NSX Advanced Load Balancer"
	case TopologyVDSHAProxy:
		return "Supervisor on vSphere (VDS) networking with HAProxy (legacy)"
	case TopologyNSXVPC:
		return "Supervisor on NSX VPC-based networking (VCF 9)"
	default:
		return "unknown topology"
	}
}

// UsesNSX reports whether the topology has an NSX control plane to interrogate.
func (t Topology) UsesNSX() bool {
	return t == TopologyNSX || t == TopologyNSXALB || t == TopologyNSXVPC
}

// UsesALB reports whether NSX Advanced Load Balancer is in play.
func (t Topology) UsesALB() bool {
	return t == TopologyNSXALB || t == TopologyVDSALB
}

// UsesHAProxy reports whether the HAProxy appliance is in play.
func (t Topology) UsesHAProxy() bool { return t == TopologyVDSHAProxy }

// Valid reports whether t is a known topology.
func (t Topology) Valid() bool {
	for _, k := range AllTopologies {
		if k == t {
			return true
		}
	}
	return false
}

// ParseTopology validates a topology string from config or a CLI flag.
func ParseTopology(s string) (Topology, error) {
	t := Topology(s)
	if !t.Valid() {
		return "", fmt.Errorf("unknown topology %q (known: %v)", s, AllTopologies)
	}
	return t, nil
}
