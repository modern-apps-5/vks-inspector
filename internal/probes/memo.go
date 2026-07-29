package probes

import (
	"context"
	"crypto/x509"
	"net/netip"
	"sync"
)

// Memo caches probe results for the lifetime of one run.
//
// Several checks legitimately need the same observation: dns.forward and
// dns.resolver-agreement both resolve every declared name, and tls.chain and
// tls.expiry both inspect every certificate. Without caching, a run probes each
// target twice — doubling the time an operator waits and doubling the traffic
// pointed at a customer's network, which for a read-only preflight tool is
// simply rude.
//
// Caching is correct here because a run is a point-in-time observation. Two
// checks asking the same question within one run should get the same answer;
// if they did not, the report would contradict itself.
//
// Safe for concurrent use. In-flight duplicate requests wait for the first
// rather than racing, so N checks asking simultaneously still produce one probe.
type Memo struct {
	inner Prober

	mu    sync.Mutex
	calls map[string]*memoEntry
}

type memoEntry struct {
	once  sync.Once
	value any
}

// NewMemo wraps a prober with a per-run cache.
func NewMemo(inner Prober) *Memo {
	return &Memo{inner: inner, calls: map[string]*memoEntry{}}
}

var _ Prober = (*Memo)(nil)

// ProbeKinds implements Prober.
func (m *Memo) ProbeKinds() []string { return m.inner.ProbeKinds() }

// do runs fn once per key, returning the same value to every caller.
func (m *Memo) do(key string, fn func() any) any {
	m.mu.Lock()
	e, ok := m.calls[key]
	if !ok {
		e = &memoEntry{}
		m.calls[key] = e
	}
	m.mu.Unlock()

	e.once.Do(func() { e.value = fn() })
	return e.value
}

// LookupHost implements Prober.
func (m *Memo) LookupHost(ctx context.Context, name, resolver string) DNSAnswer {
	v := m.do("host|"+resolver+"|"+name, func() any {
		return m.inner.LookupHost(ctx, name, resolver)
	})
	return v.(DNSAnswer)
}

// LookupAddr implements Prober.
func (m *Memo) LookupAddr(ctx context.Context, addr netip.Addr, resolver string) PTRAnswer {
	v := m.do("ptr|"+resolver+"|"+addr.String(), func() any {
		return m.inner.LookupAddr(ctx, addr, resolver)
	})
	return v.(PTRAnswer)
}

// DialTCP implements Prober.
func (m *Memo) DialTCP(ctx context.Context, address string) PortAnswer {
	v := m.do("tcp|"+address, func() any {
		return m.inner.DialTCP(ctx, address)
	})
	return v.(PortAnswer)
}

// InspectTLS implements Prober.
func (m *Memo) InspectTLS(ctx context.Context, address, serverName string, roots *x509.CertPool) TLSAnswer {
	// Roots are not part of the key: a run uses one trust configuration
	// throughout, and including a pointer would defeat the cache entirely.
	v := m.do("tls|"+address+"|"+serverName, func() any {
		return m.inner.InspectTLS(ctx, address, serverName, roots)
	})
	return v.(TLSAnswer)
}

// QueryNTP implements Prober.
func (m *Memo) QueryNTP(ctx context.Context, server string) NTPAnswer {
	v := m.do("ntp|"+server, func() any {
		return m.inner.QueryNTP(ctx, server)
	})
	return v.(NTPAnswer)
}
