// Package probes is the network-probe surface: the only place in the tool that
// is allowed to open a socket.
//
// It exists as a seam, not an abstraction for its own sake. Checks take a probe
// interface rather than calling net.Dial directly so that the whole class-(a)
// check suite is unit-testable with a fake, on a laptop, in CI, with no lab and
// no network. See docs/test-coverage.md.
//
// Every probe must be read-only against the target and must honour context
// cancellation. Anything that could plausibly disturb a production network
// belongs behind checks.CapInvasive and must say so in its doc comment.
package probes

// System is the real, socket-opening implementation.
//
// TODO(phase-2): implement. The surface will grow to roughly:
//
//	LookupHost(ctx, name, resolver) ([]netip.Addr, error)      — forward DNS
//	LookupAddr(ctx, addr, resolver) ([]string, error)          — reverse DNS
//	DialTCP(ctx, addr) (open|refused|filtered, rtt, error)     — port state, tri-state on purpose
//	NTPQuery(ctx, server) (offset, stratum, error)             — real SNTP, not a ping
//	TLSInspect(ctx, addr) (chain, notAfter, thumbprint, error) — cert chain, no verification bypass
//	Ping(ctx, addr) (rtt, error)                               — ICMP echo, may need raw socket
//	PathMTU(ctx, addr, hint) (mtu, error)                      — INVASIVE, DF-flagged probes
//	ARPScan(ctx, cidr) ([]netip.Addr, error)                   — duplicate-IP detection, local segment only
//
// Two notes that must survive to implementation:
//
//  1. DialTCP returns a tri-state, not a bool. "Connection refused" and "no
//     answer" are different findings: refused proves reachability with nothing
//     listening; a silent drop proves nothing and must map to StatusUnknown.
//     Collapsing them into a bool is how a firewall gets misreported as a dead
//     service.
//
//  2. Ping and ARPScan may require raw sockets, meaning root or CAP_NET_RAW.
//     The tool must degrade to a reported skip with a clear reason when it
//     cannot get them, never a silent pass and never a hard failure. A field
//     engineer running unprivileged on a customer laptop is the normal case,
//     not the exception.
type System struct{}

// ProbeKinds implements checks.Probes.
func (System) ProbeKinds() []string {
	// Populated as probes land. An empty list is honest: this build can perform
	// no network probes yet.
	return nil
}

// Fake is a probe implementation for tests. It performs no I/O.
type Fake struct {
	Kinds []string
}

// ProbeKinds implements checks.Probes.
func (f Fake) ProbeKinds() []string { return f.Kinds }
