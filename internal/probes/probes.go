// Package probes is the network-probe surface: the only place in the tool that
// is allowed to open a socket.
//
// It exists as a seam, not an abstraction for its own sake. Checks take this
// interface rather than calling net.Dial directly so the whole class-(a) check
// suite is unit-testable with a fake, on a laptop, in CI, with no lab and no
// network. See docs/unit-test-coverage.md.
//
// Every probe is read-only against the target and honours context cancellation.
// Anything that could plausibly disturb a production network belongs behind
// checks.CapInvasive and must say so in its doc comment.
package probes

import (
	"context"
	"crypto/x509"
	"net/netip"
	"time"
)

// PortState is the outcome of a TCP connection attempt.
//
// Tri-state, never boolean. "Connection refused" and "no answer" are different
// findings with different remediations: refused proves reachability with nothing
// listening, while silence proves nothing at all. Collapsing them into a bool is
// how a firewall gets reported as a dead service.
type PortState string

const (
	// PortOpen — the connection was accepted.
	PortOpen PortState = "open"
	// PortRefused — an RST came back. The host is reachable; nothing is
	// listening on that port.
	PortRefused PortState = "refused"
	// PortFiltered — silence. Almost always a firewall. This must map to
	// StatusUnknown, never StatusFail: we did not observe a failure.
	PortFiltered PortState = "filtered"
	// PortError — the probe could not be attempted (unresolvable name, bad
	// address). A tool-side problem, not an observation.
	PortError PortState = "error"
)

// DNSAnswer is one resolver's response for one name.
type DNSAnswer struct {
	Resolver string
	Name     string
	Addrs    []netip.Addr
	RTT      time.Duration
	// Err is set when the resolver did not answer usefully. NXDOMAIN and a
	// timeout are both errors here but mean different things, so Timeout
	// distinguishes them: a timeout is indeterminate, NXDOMAIN is a finding.
	Err     error
	Timeout bool
}

// PTRAnswer is a reverse lookup result.
type PTRAnswer struct {
	Resolver string
	Addr     netip.Addr
	Names    []string
	Err      error
	Timeout  bool
}

// PortAnswer is a TCP connection attempt result.
type PortAnswer struct {
	Address string
	State   PortState
	RTT     time.Duration
	Err     error
}

// TLSAnswer describes a TLS endpoint's certificate chain.
//
// Verified is reported separately from the chain itself: an endpoint whose
// chain does not validate is a finding, and one we chose not to verify is not
// evidence of anything. A check must not conflate them.
type TLSAnswer struct {
	Address string
	// Chain is leaf-first.
	Chain []*x509.Certificate
	// Verified reports whether the chain validated against the system or
	// supplied trust store.
	Verified bool
	// VerifyErr explains a verification failure.
	VerifyErr error
	// Err is set when the handshake itself could not complete.
	Err error
}

// Leaf returns the end-entity certificate, or nil.
func (a TLSAnswer) Leaf() *x509.Certificate {
	if len(a.Chain) == 0 {
		return nil
	}
	return a.Chain[0]
}

// NTPAnswer is an SNTP query result.
type NTPAnswer struct {
	Server string
	// Offset is the local clock's error relative to the server: positive means
	// the local clock is ahead.
	Offset time.Duration
	RTT    time.Duration
	// Stratum 0 means "kiss of death" or unsynchronised; 16 means the server
	// itself is not synchronised. Either makes the offset meaningless.
	Stratum uint8
	Err     error
}

// Usable reports whether the answer can be used to judge clock skew.
func (a NTPAnswer) Usable() bool {
	return a.Err == nil && a.Stratum > 0 && a.Stratum < 16
}

// Prober is the surface a check may use. Implemented by System for real work
// and by Fake in tests.
type Prober interface {
	// ProbeKinds lists what this implementation can do, so the report can say
	// what a run was capable of.
	ProbeKinds() []string

	// LookupHost resolves a name. An empty resolver means the system resolver.
	LookupHost(ctx context.Context, name, resolver string) DNSAnswer
	// LookupAddr resolves an address to names.
	LookupAddr(ctx context.Context, addr netip.Addr, resolver string) PTRAnswer
	// DialTCP attempts a TCP connection and reports a tri-state result.
	DialTCP(ctx context.Context, address string) PortAnswer
	// InspectTLS completes a TLS handshake and returns the presented chain.
	// Never bypasses verification silently — Verified says what happened.
	InspectTLS(ctx context.Context, address, serverName string, roots *x509.CertPool) TLSAnswer
	// QueryNTP sends a real SNTP request. Not a ping: a host can answer ICMP
	// perfectly and serve no time at all.
	QueryNTP(ctx context.Context, server string) NTPAnswer
}
