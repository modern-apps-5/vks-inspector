package probes

import (
	"context"
	"crypto/x509"
	"fmt"
	"net/netip"
)

// Fake is a scripted probe implementation for tests. It performs no I/O.
//
// This seam is why the entire class-(a) check suite is testable on a laptop
// with no network and no lab. A check that reaches for net.Dial directly
// bypasses it and becomes untestable — that is why probing is confined to this
// package. See docs/CHECK-TAXONOMY.md.
//
// Anything not scripted returns a deliberately unhelpful answer rather than a
// zero value, so a test that forgets to script a call fails loudly instead of
// quietly asserting against an empty result.
type Fake struct {
	Kinds []string

	// Hosts maps "resolver|name" — or just "name" for any resolver — to an
	// answer.
	Hosts map[string]DNSAnswer
	// PTRs maps "resolver|addr", or just "addr".
	PTRs map[string]PTRAnswer
	// Ports maps "host:port".
	Ports map[string]PortAnswer
	// TLS maps "host:port".
	TLS map[string]TLSAnswer
	// NTP maps server address.
	NTP map[string]NTPAnswer
}

var _ Prober = (*Fake)(nil)

// ProbeKinds implements Prober.
func (f *Fake) ProbeKinds() []string {
	if f.Kinds != nil {
		return f.Kinds
	}
	return []string{"dns-forward", "dns-reverse", "tcp", "tls", "ntp"}
}

// LookupHost implements Prober.
func (f *Fake) LookupHost(_ context.Context, name, resolver string) DNSAnswer {
	if a, ok := f.Hosts[resolver+"|"+name]; ok {
		a.Resolver, a.Name = resolverLabel(resolver), name
		return a
	}
	if a, ok := f.Hosts[name]; ok {
		a.Resolver, a.Name = resolverLabel(resolver), name
		return a
	}
	return DNSAnswer{
		Resolver: resolverLabel(resolver), Name: name,
		Err: fmt.Errorf("fake: no scripted answer for %q via %q", name, resolverLabel(resolver)),
	}
}

// LookupAddr implements Prober.
func (f *Fake) LookupAddr(_ context.Context, addr netip.Addr, resolver string) PTRAnswer {
	if a, ok := f.PTRs[resolver+"|"+addr.String()]; ok {
		a.Resolver, a.Addr = resolverLabel(resolver), addr
		return a
	}
	if a, ok := f.PTRs[addr.String()]; ok {
		a.Resolver, a.Addr = resolverLabel(resolver), addr
		return a
	}
	return PTRAnswer{
		Resolver: resolverLabel(resolver), Addr: addr,
		Err: fmt.Errorf("fake: no scripted PTR for %s", addr),
	}
}

// DialTCP implements Prober.
func (f *Fake) DialTCP(_ context.Context, address string) PortAnswer {
	if a, ok := f.Ports[address]; ok {
		a.Address = address
		return a
	}
	return PortAnswer{Address: address, State: PortError,
		Err: fmt.Errorf("fake: no scripted result for %s", address)}
}

// InspectTLS implements Prober.
func (f *Fake) InspectTLS(_ context.Context, address, _ string, _ *x509.CertPool) TLSAnswer {
	if a, ok := f.TLS[address]; ok {
		a.Address = address
		return a
	}
	return TLSAnswer{Address: address, Err: fmt.Errorf("fake: no scripted TLS for %s", address)}
}

// QueryNTP implements Prober.
func (f *Fake) QueryNTP(_ context.Context, server string) NTPAnswer {
	if a, ok := f.NTP[server]; ok {
		a.Server = server
		return a
	}
	return NTPAnswer{Server: server, Err: fmt.Errorf("fake: no scripted NTP for %s", server)}
}
