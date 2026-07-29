package config

import (
	"fmt"
	"strings"
)

// Topology is the deployment shape, expressed as orthogonal axes rather than a
// single enum.
//
// Networking and load balancing are independent decisions and the requirements
// attach to whichever axis they actually depend on: the NSX overlay MTU
// requirement depends on the networking choice and is completely indifferent to
// which load balancer is in front of it. A flat enum forces every requirement
// to re-list combinations, and it strains immediately — "nsx-vpc" says nothing
// about the load balancer, which is a real question with a real answer.
//
// Not every combination is valid. See Validate.
type Topology struct {
	Networking   Networking   `yaml:"networking"   json:"networking"`
	LoadBalancer LoadBalancer `yaml:"loadBalancer" json:"loadBalancer"`
}

// Networking is the L2/L3 substrate the Supervisor and workloads sit on.
type Networking string

const (
	// NetVDS — vSphere Distributed Switch networking, no NSX.
	NetVDS Networking = "vds"
	// NetNSX — NSX networking with the traditional Tier-0/Tier-1 model.
	NetNSX Networking = "nsx"
	// NetNSXVPC — NSX VPC-based networking (VCF 9).
	// FLAGGED: lowest-confidence area of the requirements matrix.
	NetNSXVPC Networking = "nsx-vpc"
)

// LoadBalancer is what provides L4 load balancing for the Supervisor control
// plane and for LoadBalancer-type Services.
type LoadBalancer string

const (
	// LBNSX — the load balancer built into NSX.
	LBNSX LoadBalancer = "nsx-lb"
	// LBALB — NSX Advanced Load Balancer (Avi).
	LBALB LoadBalancer = "alb"
	// LBHAProxy — the HAProxy appliance.
	// FLAGGED: believed deprecated/removed in the VCF 9 generation.
	LBHAProxy LoadBalancer = "haproxy"
)

// AllNetworking and AllLoadBalancers are the canonical ordered lists. Prompts,
// help text and docs iterate these so a new value cannot be half-added.
var (
	AllNetworking    = []Networking{NetVDS, NetNSX, NetNSXVPC}
	AllLoadBalancers = []LoadBalancer{LBNSX, LBALB, LBHAProxy}
)

// validCombinations is the validity table. A combination absent from this map
// is rejected rather than assumed workable — telling someone their unsupported
// design passed preflight is the worst thing this tool could do.
//
// The value is a note shown when the combination is unusual, empty when it is
// the mainstream choice.
var validCombinations = map[Topology]string{
	{NetVDS, LBALB}:     "",
	{NetVDS, LBHAProxy}: "HAProxy is believed deprecated in the VCF 9 generation; see matrix row LB-HAP-000",
	{NetNSX, LBNSX}:     "",
	{NetNSX, LBALB}:     "",
	// FLAGGED: which load balancers VPC-based networking supports is not
	// confirmed. Both are permitted so the tool does not block a valid design,
	// and both carry a note so nobody reads acceptance as confirmation.
	{NetNSXVPC, LBNSX}: "VPC-based networking is the least-verified topology in the requirements matrix; every VPC-* row is flagged",
	{NetNSXVPC, LBALB}: "VPC-based networking is the least-verified topology in the requirements matrix; every VPC-* row is flagged",
}

// Validate reports whether the combination is one this build claims to grade.
func (t Topology) Validate() error {
	var errs []string
	if !t.Networking.Valid() {
		errs = append(errs, fmt.Sprintf("networking %q is not one of %v", t.Networking, AllNetworking))
	}
	if !t.LoadBalancer.Valid() {
		errs = append(errs, fmt.Sprintf("loadBalancer %q is not one of %v", t.LoadBalancer, AllLoadBalancers))
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	if _, ok := validCombinations[t]; !ok {
		return fmt.Errorf("networking %q with loadBalancer %q is not a supported combination (supported: %s)",
			t.Networking, t.LoadBalancer, strings.Join(supportedCombinationStrings(), ", "))
	}
	return nil
}

// Note returns the caveat for this combination, if any.
func (t Topology) Note() string { return validCombinations[t] }

// Valid reports whether the combination is supported.
func (t Topology) Valid() bool { return t.Validate() == nil }

// String is the short form used in reports, filenames and log lines.
func (t Topology) String() string {
	return string(t.Networking) + "+" + string(t.LoadBalancer)
}

// Description is the operator-facing one-liner.
func (t Topology) Description() string {
	return fmt.Sprintf("Supervisor on %s networking with %s", t.Networking.Description(), t.LoadBalancer.Description())
}

// UsesNSX reports whether there is an NSX control plane to interrogate.
func (t Topology) UsesNSX() bool { return t.Networking.UsesNSX() }

// UsesALB reports whether NSX Advanced Load Balancer is in play.
func (t Topology) UsesALB() bool { return t.LoadBalancer == LBALB }

// UsesHAProxy reports whether the HAProxy appliance is in play.
func (t Topology) UsesHAProxy() bool { return t.LoadBalancer == LBHAProxy }

// UsesVDSPortGroups reports whether the deployment consumes named vSphere
// portgroups for its workload networks, as opposed to NSX segments.
func (t Topology) UsesVDSPortGroups() bool { return t.Networking == NetVDS }

// Valid reports whether n is a known networking value.
func (n Networking) Valid() bool {
	for _, k := range AllNetworking {
		if k == n {
			return true
		}
	}
	return false
}

// UsesNSX reports whether this networking choice involves NSX at all.
func (n Networking) UsesNSX() bool { return n == NetNSX || n == NetNSXVPC }

// Description is the operator-facing name.
func (n Networking) Description() string {
	switch n {
	case NetVDS:
		return "vSphere Distributed Switch (VDS)"
	case NetNSX:
		return "NSX"
	case NetNSXVPC:
		return "NSX VPC-based (VCF 9)"
	default:
		return string(n)
	}
}

// Valid reports whether lb is a known load balancer value.
func (lb LoadBalancer) Valid() bool {
	for _, k := range AllLoadBalancers {
		if k == lb {
			return true
		}
	}
	return false
}

// Description is the operator-facing name.
func (lb LoadBalancer) Description() string {
	switch lb {
	case LBNSX:
		return "the NSX built-in load balancer"
	case LBALB:
		return "NSX Advanced Load Balancer (Avi)"
	case LBHAProxy:
		return "HAProxy (legacy)"
	default:
		return string(lb)
	}
}

// LoadBalancersFor returns the load balancers valid with a given networking
// choice. The interactive prompt uses this so it never offers a combination it
// would then reject.
func LoadBalancersFor(n Networking) []LoadBalancer {
	var out []LoadBalancer
	for _, lb := range AllLoadBalancers {
		if _, ok := validCombinations[Topology{n, lb}]; ok {
			out = append(out, lb)
		}
	}
	return out
}

// SupportedCombinations returns every valid topology, ordered.
func SupportedCombinations() []Topology {
	var out []Topology
	for _, n := range AllNetworking {
		for _, lb := range LoadBalancersFor(n) {
			out = append(out, Topology{n, lb})
		}
	}
	return out
}

func supportedCombinationStrings() []string {
	combos := SupportedCombinations()
	out := make([]string, 0, len(combos))
	for _, c := range combos {
		out = append(out, c.String())
	}
	return out
}

// ParseTopology accepts the short "networking+loadbalancer" form used by flags
// and report filenames.
func ParseTopology(s string) (Topology, error) {
	n, lb, ok := strings.Cut(s, "+")
	if !ok {
		return Topology{}, fmt.Errorf("topology %q must be in the form networking+loadBalancer, e.g. nsx+alb", s)
	}
	t := Topology{Networking(n), LoadBalancer(lb)}
	if err := t.Validate(); err != nil {
		return Topology{}, err
	}
	return t, nil
}
