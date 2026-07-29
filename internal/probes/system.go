package probes

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"syscall"
	"time"
)

// System is the real, socket-opening implementation.
type System struct {
	// Timeout bounds a single probe. Zero uses a sensible default.
	Timeout time.Duration
}

var _ Prober = System{}

func (s System) timeout() time.Duration {
	if s.Timeout > 0 {
		return s.Timeout
	}
	return 5 * time.Second
}

// ProbeKinds implements Prober.
func (System) ProbeKinds() []string {
	return []string{"dns-forward", "dns-reverse", "tcp", "tls", "ntp"}
}

// resolverFor builds a resolver that queries a specific server, or the system
// resolver when addr is empty.
//
// Querying each declared resolver individually is the whole point: "DNS works"
// from the operator's laptop says nothing about whether the resolver the
// Supervisor will actually use can answer.
func (s System) resolverFor(addr string) *net.Resolver {
	if addr == "" {
		return net.DefaultResolver
	}
	server := addr
	if _, _, err := net.SplitHostPort(server); err != nil {
		server = net.JoinHostPort(server, "53")
	}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: s.timeout()}
			return d.DialContext(ctx, network, server)
		},
	}
}

// LookupHost implements Prober.
func (s System) LookupHost(ctx context.Context, name, resolver string) DNSAnswer {
	ans := DNSAnswer{Resolver: resolverLabel(resolver), Name: name}

	ctx, cancel := context.WithTimeout(ctx, s.timeout())
	defer cancel()

	start := time.Now()
	addrs, err := s.resolverFor(resolver).LookupNetIP(ctx, "ip", name)
	ans.RTT = time.Since(start)
	if err != nil {
		ans.Err = err
		ans.Timeout = isTimeout(err)
		return ans
	}
	for _, a := range addrs {
		ans.Addrs = append(ans.Addrs, a.Unmap())
	}
	return ans
}

// LookupAddr implements Prober.
func (s System) LookupAddr(ctx context.Context, addr netip.Addr, resolver string) PTRAnswer {
	ans := PTRAnswer{Resolver: resolverLabel(resolver), Addr: addr}

	ctx, cancel := context.WithTimeout(ctx, s.timeout())
	defer cancel()

	names, err := s.resolverFor(resolver).LookupAddr(ctx, addr.String())
	if err != nil {
		ans.Err = err
		ans.Timeout = isTimeout(err)
		return ans
	}
	for _, n := range names {
		ans.Names = append(ans.Names, strings.TrimSuffix(n, "."))
	}
	return ans
}

// DialTCP implements Prober.
//
// The tri-state classification is the load-bearing part. A refused connection
// and a silent drop look similar in a naive implementation and mean opposite
// things to the person reading the report.
func (s System) DialTCP(ctx context.Context, address string) PortAnswer {
	ans := PortAnswer{Address: address}

	ctx, cancel := context.WithTimeout(ctx, s.timeout())
	defer cancel()

	d := net.Dialer{}
	start := time.Now()
	conn, err := d.DialContext(ctx, "tcp", address)
	ans.RTT = time.Since(start)

	if err == nil {
		_ = conn.Close()
		ans.State = PortOpen
		return ans
	}
	ans.Err = err

	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		// The host is there and answered. Nothing is listening.
		ans.State = PortRefused
	case isTimeout(err) || errors.Is(err, context.DeadlineExceeded):
		// Silence. Almost always a firewall. We did not observe a failure of
		// the service, so this must not be reported as one.
		ans.State = PortFiltered
	case errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.ENETUNREACH):
		ans.State = PortFiltered
	default:
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) {
			// Could not even resolve the target: not an observation about the
			// port at all.
			ans.State = PortError
			return ans
		}
		ans.State = PortFiltered
	}
	return ans
}

// InspectTLS implements Prober.
//
// The handshake always completes with verification disabled so the chain can be
// inspected even when it does not validate — then verification is performed
// explicitly and reported separately. Reporting "could not connect" for an
// expired certificate would hide the actual problem.
func (s System) InspectTLS(ctx context.Context, address, serverName string, roots *x509.CertPool) TLSAnswer {
	ans := TLSAnswer{Address: address}

	ctx, cancel := context.WithTimeout(ctx, s.timeout())
	defer cancel()

	if serverName == "" {
		if h, _, err := net.SplitHostPort(address); err == nil {
			serverName = h
		} else {
			serverName = address
		}
	}

	d := tls.Dialer{
		NetDialer: &net.Dialer{},
		Config: &tls.Config{
			ServerName:         serverName,
			InsecureSkipVerify: true, //nolint:gosec // verified explicitly below
			MinVersion:         tls.VersionTLS12,
		},
	}
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		ans.Err = err
		return ans
	}
	defer func() { _ = conn.Close() }()

	tc, ok := conn.(*tls.Conn)
	if !ok {
		ans.Err = fmt.Errorf("not a TLS connection")
		return ans
	}
	ans.Chain = tc.ConnectionState().PeerCertificates
	if len(ans.Chain) == 0 {
		ans.Err = fmt.Errorf("peer presented no certificate")
		return ans
	}

	intermediates := x509.NewCertPool()
	for _, c := range ans.Chain[1:] {
		intermediates.AddCert(c)
	}
	_, verr := ans.Chain[0].Verify(x509.VerifyOptions{
		DNSName:       serverName,
		Roots:         roots, // nil means the system trust store
		Intermediates: intermediates,
	})
	ans.Verified = verr == nil
	ans.VerifyErr = verr
	return ans
}

// ntpEpochOffset converts between the NTP epoch (1900) and the Unix epoch.
const ntpEpochOffset = 2208988800

// QueryNTP implements Prober.
//
// A real SNTP request (RFC 4330 mode 3), not an ICMP echo and not a TCP connect.
// The previous generation of this tool's test list specified "ping/curl NTP",
// which tests neither the protocol nor the service: a host can answer ping
// perfectly and serve no time at all.
func (s System) QueryNTP(ctx context.Context, server string) NTPAnswer {
	ans := NTPAnswer{Server: server}

	addr := server
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, "123")
	}

	d := net.Dialer{Timeout: s.timeout()}
	conn, err := d.DialContext(ctx, "udp", addr)
	if err != nil {
		ans.Err = err
		return ans
	}
	defer func() { _ = conn.Close() }()

	deadline := time.Now().Add(s.timeout())
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = conn.SetDeadline(deadline)

	// LI=0, VN=4, Mode=3 (client).
	req := make([]byte, 48)
	req[0] = 0x23

	sent := time.Now()
	binary.BigEndian.PutUint32(req[40:], uint32(sent.Unix()+ntpEpochOffset))

	if _, err := conn.Write(req); err != nil {
		ans.Err = err
		return ans
	}

	resp := make([]byte, 48)
	if _, err := conn.Read(resp); err != nil {
		ans.Err = err
		return ans
	}
	received := time.Now()
	ans.RTT = received.Sub(sent)
	ans.Stratum = resp[1]

	transmit := ntpTime(resp[40:48])
	if transmit.IsZero() {
		ans.Err = fmt.Errorf("server returned no transmit timestamp")
		return ans
	}
	// Halve the round trip to approximate one-way delay, the standard SNTP
	// simplification. Good to well within any tolerance this tool checks.
	ans.Offset = transmit.Sub(received.Add(-ans.RTT / 2))
	return ans
}

func ntpTime(b []byte) time.Time {
	secs := binary.BigEndian.Uint32(b[0:4])
	frac := binary.BigEndian.Uint32(b[4:8])
	if secs == 0 && frac == 0 {
		return time.Time{}
	}
	nsec := (int64(frac) * 1e9) >> 32
	return time.Unix(int64(secs)-ntpEpochOffset, nsec)
}

func isTimeout(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) {
		return ne.Timeout()
	}
	return errors.Is(err, context.DeadlineExceeded)
}

func resolverLabel(r string) string {
	if r == "" {
		return "system"
	}
	return r
}
