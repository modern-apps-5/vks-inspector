package network_test

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/modern-apps-5/vks-inspector/internal/checks"
	"github.com/modern-apps-5/vks-inspector/internal/checks/network"
	"github.com/modern-apps-5/vks-inspector/internal/config"
	"github.com/modern-apps-5/vks-inspector/internal/probes"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// Every test here runs with probes.Fake. No socket is opened, no lab is needed,
// and the whole class-(a) suite runs in CI on a laptop. That is the entire
// reason probing is confined to internal/probes behind an interface.

var now = time.Date(2026, 3, 14, 9, 30, 0, 0, time.UTC)

func rc(f *probes.Fake, mutate func(*config.Config)) *checks.RunContext {
	cfg := &config.Config{
		APIVersion: config.APIVersion,
		Kind:       config.Kind,
		Metadata:   config.Metadata{Name: "net-test"},
		Topology:   config.Topology{Networking: config.NetNSX, LoadBalancer: config.LBNSX},
		Infrastructure: config.Infrastructure{
			VCenter: config.Endpoint{FQDN: "vc.example.com", IP: "192.0.2.10", Port: 443},
		},
		Services: config.Services{
			DNS: config.DNSBlock{Servers: []string{"192.0.2.53"}},
		},
	}
	if mutate != nil {
		mutate(cfg)
	}
	return &checks.RunContext{
		Mode:    checks.ModePreflight,
		Config:  cfg,
		Probes:  f,
		Now:     func() time.Time { return now },
		Vantage: "jumphost",
	}
}

func addrs(ss ...string) []netip.Addr {
	var out []netip.Addr
	for _, s := range ss {
		out = append(out, netip.MustParseAddr(s))
	}
	return out
}

// verdict reduces a result set to the first non-pass status. A method rather
// than a function so the multi-value Run(...) call can be its sole argument.
type verdict struct{ t *testing.T }

func (v verdict) of(res []results.Result, err error) results.Status {
	v.t.Helper()
	if err != nil {
		v.t.Fatalf("Run: %v", err)
	}
	if len(res) == 0 {
		v.t.Fatal("check returned no results")
	}
	for _, r := range res {
		if r.Status != results.StatusPass {
			return r.Status
		}
	}
	return results.StatusPass
}

// ---------------------------------------------------------------------------

func TestForwardDNS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		answer probes.DNSAnswer
		want   results.Status
	}{
		{
			name:   "resolves to the declared address",
			answer: probes.DNSAnswer{Addrs: addrs("192.0.2.10")},
			want:   results.StatusPass,
		},
		{
			// Worse than not resolving: the deployment proceeds and talks to
			// something else entirely.
			name:   "resolves to the wrong address",
			answer: probes.DNSAnswer{Addrs: addrs("198.51.100.1")},
			want:   results.StatusFail,
		},
		{
			name:   "NXDOMAIN is a finding",
			answer: probes.DNSAnswer{Err: &net.DNSError{Err: "no such host", IsNotFound: true}},
			want:   results.StatusFail,
		},
		{
			// The load-bearing distinction: a resolver that did not answer has
			// not told us the record is missing.
			name:   "a timeout is indeterminate, not a failure",
			answer: probes.DNSAnswer{Err: errors.New("i/o timeout"), Timeout: true},
			want:   results.StatusUnknown,
		},
		{
			name:   "an empty answer is a finding",
			answer: probes.DNSAnswer{},
			want:   results.StatusFail,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := &probes.Fake{Hosts: map[string]probes.DNSAnswer{"vc.example.com": tt.answer}}
			got := verdict{t}.of(network.Forward{}.Run(context.Background(), rc(f, nil)))
			if got != tt.want {
				t.Errorf("status = %s, want %s", got, tt.want)
			}
		})
	}
}

// Each declared resolver is queried separately. "DNS works" from the operator's
// laptop says nothing about the resolver the Supervisor will use.
func TestForwardDNSQueriesEveryResolver(t *testing.T) {
	t.Parallel()

	f := &probes.Fake{Hosts: map[string]probes.DNSAnswer{
		"192.0.2.53|vc.example.com": {Addrs: addrs("192.0.2.10")},
		"192.0.2.54|vc.example.com": {Err: &net.DNSError{Err: "no such host", IsNotFound: true}},
	}}
	ctx := rc(f, func(cfg *config.Config) {
		cfg.Services.DNS.Servers = []string{"192.0.2.53", "192.0.2.54"}
	})

	res, err := network.Forward{}.Run(context.Background(), ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Status != results.StatusFail {
		t.Fatalf("expected exactly one failure for the second resolver, got %d results", len(res))
	}
	if res[0].Target != "vc.example.com via 192.0.2.54" {
		t.Errorf("target = %q; the failing resolver must be named", res[0].Target)
	}
}

func TestReverseDNS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		answer probes.PTRAnswer
		want   results.Status
	}{
		{"PTR agrees", probes.PTRAnswer{Names: []string{"vc.example.com"}}, results.StatusPass},
		{"PTR is case-insensitive", probes.PTRAnswer{Names: []string{"VC.Example.COM"}}, results.StatusPass},
		{"PTR points elsewhere", probes.PTRAnswer{Names: []string{"other.example.com"}}, results.StatusFail},
		{"no PTR", probes.PTRAnswer{}, results.StatusFail},
		{"timeout is indeterminate", probes.PTRAnswer{Err: errors.New("timeout"), Timeout: true}, results.StatusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := &probes.Fake{PTRs: map[string]probes.PTRAnswer{"192.0.2.10": tt.answer}}
			got := verdict{t}.of(network.Reverse{}.Run(context.Background(), rc(f, nil)))
			if got != tt.want {
				t.Errorf("status = %s, want %s", got, tt.want)
			}
		})
	}
}

// Matrix row COM-DNS-002 is flagged on whether PTR is a hard requirement for
// VCF 9. Until that is confirmed the site decides, via config, rather than this
// tool guessing — and the escalation is recorded so nobody wonders why a
// warning became a blocker.
func TestReverseSeverityFollowsTheConfig(t *testing.T) {
	t.Parallel()

	f := &probes.Fake{PTRs: map[string]probes.PTRAnswer{"192.0.2.10": {}}}

	res, err := network.Reverse{}.Run(context.Background(), rc(f, nil))
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Severity != results.SeverityWarning {
		t.Errorf("default severity = %s, want warning", res[0].Severity)
	}

	res, err = network.Reverse{}.Run(context.Background(), rc(f, func(cfg *config.Config) {
		cfg.Services.DNS.RequireReverse = true
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Severity != results.SeverityBlocker {
		t.Errorf("with requireReverse: severity = %s, want blocker", res[0].Severity)
	}
	if res[0].Evidence["escalated_by"] == nil {
		t.Error("escalation not recorded; nobody can tell why the severity changed")
	}
}

func TestResolverAgreement(t *testing.T) {
	t.Parallel()

	t.Run("disagreement is reported", func(t *testing.T) {
		f := &probes.Fake{Hosts: map[string]probes.DNSAnswer{
			"192.0.2.53|vc.example.com": {Addrs: addrs("192.0.2.10")},
			"192.0.2.54|vc.example.com": {Addrs: addrs("198.51.100.1")},
		}}
		ctx := rc(f, func(cfg *config.Config) {
			cfg.Services.DNS.Servers = []string{"192.0.2.53", "192.0.2.54"}
		})
		if got := (verdict{t}).of(network.ResolverAgreement{}.Run(context.Background(), ctx)); got != results.StatusFail {
			t.Errorf("status = %s, want fail", got)
		}
	})

	t.Run("a single resolver is a skip, not a pass", func(t *testing.T) {
		f := &probes.Fake{Hosts: map[string]probes.DNSAnswer{"vc.example.com": {Addrs: addrs("192.0.2.10")}}}
		res, err := network.ResolverAgreement{}.Run(context.Background(), rc(f, nil))
		if err != nil {
			t.Fatal(err)
		}
		if res[0].Status != results.StatusSkip {
			t.Errorf("status = %s, want skip with one resolver", res[0].Status)
		}
	})
}

// ---------------------------------------------------------------------------

// The tri-state mapping is the most consequential logic in this package.
// Getting `filtered` wrong blocks deployments over a firewall rule the tool
// cannot see.
func TestPortStateMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state probes.PortState
		want  results.Status
	}{
		{"open passes", probes.PortOpen, results.StatusPass},
		{"refused is a finding — reachable, nothing listening", probes.PortRefused, results.StatusFail},
		{"filtered is indeterminate — a firewall is not a dead service", probes.PortFiltered, results.StatusUnknown},
		{"a probe error is indeterminate", probes.PortError, results.StatusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := &probes.Fake{Ports: map[string]probes.PortAnswer{
				"vc.example.com:443": {State: tt.state},
			}}
			if got := (verdict{t}).of(network.PortOpen{}.Run(context.Background(), rc(f, nil))); got != tt.want {
				t.Errorf("%s -> %s, want %s", tt.state, got, tt.want)
			}
		})
	}
}

// A filtered port must never produce exit 1. We did not observe a failure.
func TestFilteredPortDoesNotProduceABlockerExit(t *testing.T) {
	t.Parallel()

	f := &probes.Fake{Ports: map[string]probes.PortAnswer{
		"vc.example.com:443": {State: probes.PortFiltered},
	}}
	res, err := network.PortOpen{}.Run(context.Background(), rc(f, nil))
	if err != nil {
		t.Fatal(err)
	}
	if code := results.ExitCode(res); code != results.ExitWarning {
		t.Errorf("exit code = %d, want %d — an unobserved failure must not be asserted",
			code, results.ExitWarning)
	}
}

// The vantage point is part of the claim, so it has to survive into the result.
func TestReachabilityRecordsTheVantage(t *testing.T) {
	t.Parallel()

	f := &probes.Fake{Ports: map[string]probes.PortAnswer{
		"vc.example.com:443": {State: probes.PortOpen},
	}}
	res, err := network.PortOpen{}.Run(context.Background(), rc(f, nil))
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Observed.Data["vantage"] != "jumphost" {
		t.Errorf("vantage not recorded: %+v", res[0].Observed.Data)
	}
}

// ---------------------------------------------------------------------------

func TestNTPReachable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		answer probes.NTPAnswer
		want   results.Status
	}{
		{"synchronised source passes", probes.NTPAnswer{Stratum: 3, Offset: time.Second}, results.StatusPass},
		{
			// A check that only asked "did it answer" would pass this. A
			// stratum-16 server's time is worthless.
			name:   "an unsynchronised source is a finding",
			answer: probes.NTPAnswer{Stratum: 16},
			want:   results.StatusFail,
		},
		{"stratum 0 is a finding", probes.NTPAnswer{Stratum: 0}, results.StatusFail},
		{
			// UDP gives no refusal, so silence cannot distinguish a blocked
			// path from a dead service.
			name:   "no answer is indeterminate",
			answer: probes.NTPAnswer{Err: errors.New("i/o timeout")},
			want:   results.StatusUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := &probes.Fake{NTP: map[string]probes.NTPAnswer{"192.0.2.123": tt.answer}}
			ctx := rc(f, func(cfg *config.Config) {
				cfg.Services.NTP = config.NTPBlock{Servers: []string{"192.0.2.123"}}
			})
			if got := (verdict{t}).of(network.NTPReachable{}.Run(context.Background(), ctx)); got != tt.want {
				t.Errorf("status = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestNTPSkew(t *testing.T) {
	t.Parallel()

	within := &probes.Fake{NTP: map[string]probes.NTPAnswer{
		"192.0.2.123": {Stratum: 3, Offset: 5 * time.Second},
	}}
	beyond := &probes.Fake{NTP: map[string]probes.NTPAnswer{
		"192.0.2.123": {Stratum: 3, Offset: 47 * time.Second},
	}}
	negative := &probes.Fake{NTP: map[string]probes.NTPAnswer{
		"192.0.2.123": {Stratum: 3, Offset: -47 * time.Second},
	}}

	withTolerance := func(f *probes.Fake, secs int) *checks.RunContext {
		return rc(f, func(cfg *config.Config) {
			cfg.Services.NTP = config.NTPBlock{Servers: []string{"192.0.2.123"}, MaxSkewSeconds: secs}
		})
	}

	if got := (verdict{t}).of(network.NTPSkew{}.Run(context.Background(), withTolerance(within, 30))); got != results.StatusPass {
		t.Errorf("within tolerance: %s", got)
	}
	if got := (verdict{t}).of(network.NTPSkew{}.Run(context.Background(), withTolerance(beyond, 30))); got != results.StatusFail {
		t.Errorf("beyond tolerance: %s", got)
	}
	// Drift in either direction is drift.
	if got := (verdict{t}).of(network.NTPSkew{}.Run(context.Background(), withTolerance(negative, 30))); got != results.StatusFail {
		t.Errorf("negative offset beyond tolerance: %s", got)
	}
}

// The tolerance is the one the site declared. Matrix row COM-NTP-002 is flagged
// because no documented product threshold is known, so with nothing declared
// there is no assertion to make — and passing would claim the clock is fine
// against a threshold nobody set.
func TestNTPSkewSkipsWithNoDeclaredTolerance(t *testing.T) {
	t.Parallel()

	f := &probes.Fake{NTP: map[string]probes.NTPAnswer{
		"192.0.2.123": {Stratum: 3, Offset: time.Hour},
	}}
	ctx := rc(f, func(cfg *config.Config) {
		cfg.Services.NTP = config.NTPBlock{Servers: []string{"192.0.2.123"}}
	})
	res, err := network.NTPSkew{}.Run(context.Background(), ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Status != results.StatusSkip {
		t.Errorf("status = %s, want skip with no declared tolerance", res[0].Status)
	}
}

// ---------------------------------------------------------------------------

// Every check in this package must declare CapNetwork and trace to the matrix.
func TestAllChecksAreWellFormed(t *testing.T) {
	t.Parallel()

	for _, c := range network.Checks() {
		m := c.Meta()
		t.Run(m.ID, func(t *testing.T) {
			var net bool
			for _, n := range m.Needs {
				if n == checks.CapNetwork {
					net = true
				}
			}
			if !net {
				t.Error("does not declare CapNetwork")
			}
			if len(m.RequirementIDs) == 0 {
				t.Error("no requirement IDs")
			}
			if m.Remediation == "" {
				t.Error("no remediation")
			}
			if len(m.Modes) != len(checks.AllModes) {
				t.Errorf("supports %d modes, want all %d", len(m.Modes), len(checks.AllModes))
			}
		})
	}
}

// A check with nothing to examine must skip, never pass. This is the property
// most easily lost when a check is extended.
func TestEveryCheckSkipsOnAnEmptyConfig(t *testing.T) {
	t.Parallel()

	empty := &checks.RunContext{
		Mode: checks.ModePreflight,
		Config: &config.Config{
			APIVersion: config.APIVersion, Kind: config.Kind,
			Metadata: config.Metadata{Name: "empty"},
			Topology: config.Topology{Networking: config.NetNSX, LoadBalancer: config.LBNSX},
		},
		Probes:  &probes.Fake{},
		Now:     func() time.Time { return now },
		Vantage: "test",
	}

	for _, c := range network.Checks() {
		res, err := c.Run(context.Background(), empty)
		if err != nil {
			t.Errorf("%s: %v", c.Meta().ID, err)
			continue
		}
		for _, r := range res {
			if r.Status == results.StatusPass {
				t.Errorf("%s passed with nothing declared: %s", r.CheckID, r.Observed.Summary)
			}
		}
	}
}

// Every result must be diffable, or drift has nothing to compare.
func TestResultsAreDiffable(t *testing.T) {
	t.Parallel()

	f := &probes.Fake{
		Hosts: map[string]probes.DNSAnswer{"vc.example.com": {Addrs: addrs("192.0.2.10")}},
		PTRs:  map[string]probes.PTRAnswer{"192.0.2.10": {Names: []string{"vc.example.com"}}},
		Ports: map[string]probes.PortAnswer{"vc.example.com:443": {State: probes.PortOpen}},
		NTP:   map[string]probes.NTPAnswer{"192.0.2.123": {Stratum: 3}},
	}
	ctx := rc(f, func(cfg *config.Config) {
		cfg.Services.NTP = config.NTPBlock{Servers: []string{"192.0.2.123"}, MaxSkewSeconds: 30}
	})

	for _, c := range network.Checks() {
		res, err := c.Run(context.Background(), ctx)
		if err != nil {
			t.Fatalf("%s: %v", c.Meta().ID, err)
		}
		for _, r := range res {
			if len(r.Observed.Data) == 0 {
				t.Errorf("%s: Observed.Data is empty; drift has nothing to compare", r.CheckID)
			}
			if r.Observed.Summary == "" {
				t.Errorf("%s: Observed.Summary is empty", r.CheckID)
			}
		}
	}
}
