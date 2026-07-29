package configval_test

import (
	"context"
	"testing"
	"time"

	"github.com/modern-apps-5/vks-inspector/internal/checks"
	"github.com/modern-apps-5/vks-inspector/internal/checks/configval"
	"github.com/modern-apps-5/vks-inspector/internal/config"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

func rc(cfg *config.Config) *checks.RunContext {
	at := time.Date(2026, 3, 14, 9, 30, 0, 0, time.UTC)
	return &checks.RunContext{
		Mode:    checks.ModePreflight,
		Config:  cfg,
		Now:     func() time.Time { return at },
		Vantage: "test",
	}
}

// base is a minimal coherent config. Tests mutate a copy of it, so each case
// states only the thing it is actually testing.
func base() *config.Config {
	return &config.Config{
		APIVersion: config.APIVersion,
		Kind:       config.Kind,
		Metadata:   config.Metadata{Name: "unit"},
		Topology:   config.Topology{Networking: config.NetNSX, LoadBalancer: config.LBNSX},
		Networks: config.Networks{
			Management: config.NetworkSpec{
				Name: "management", CIDR: "192.0.2.0/24", Gateway: "192.0.2.1", Routable: true,
				Ranges: []config.IPRange{{Start: "192.0.2.30", End: "192.0.2.34", Purpose: "supervisor"}},
			},
			Workload: []config.NetworkSpec{{Name: "workload", CIDR: "100.100.0.0/16", Routable: true}},
		},
		Kubernetes: config.Kubernetes{
			PodCIDRs:    []string{"100.96.0.0/16"},
			ServiceCIDR: "100.64.0.0/18",
		},
		NSX: &config.NSX{Tier0Gateway: "T0"},
	}
}

// statusOf returns the status of the result for a given target, or the single
// summary result when the check passed.
func statusOf(t *testing.T, res []results.Result) results.Status {
	t.Helper()
	if len(res) == 0 {
		t.Fatal("check returned no results")
	}
	// Any non-pass in the set decides the verdict; the fan-out idiom means a
	// passing check returns exactly one row.
	for _, r := range res {
		if r.Status != results.StatusPass {
			return r.Status
		}
	}
	return results.StatusPass
}

func TestOverlap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*config.Config)
		want    results.Status
		wantHit int // expected number of failing rows
	}{
		{
			name:   "a coherent plan passes",
			mutate: func(*config.Config) {},
			want:   results.StatusPass,
		},
		{
			// The bug found by running the tool against its own example: a
			// network's static range sits inside its own subnet by design.
			// That containment is required, not a collision.
			name:   "a range inside its own subnet is not an overlap",
			mutate: func(c *config.Config) {},
			want:   results.StatusPass,
		},
		{
			name: "pod CIDR overlapping the workload network is a blocker",
			mutate: func(c *config.Config) {
				c.Kubernetes.PodCIDRs = []string{"100.100.0.0/20"}
			},
			want:    results.StatusFail,
			wantHit: 1,
		},
		{
			name: "containment counts as an overlap",
			mutate: func(c *config.Config) {
				c.Kubernetes.ServiceCIDR = "100.100.5.0/24"
			},
			want:    results.StatusFail,
			wantHit: 1,
		},
		{
			name: "adjacent ranges are fine",
			mutate: func(c *config.Config) {
				c.Kubernetes.PodCIDRs = []string{"100.101.0.0/16"}
			},
			want: results.StatusPass,
		},
		{
			name: "each overlapping pair is its own finding",
			mutate: func(c *config.Config) {
				// Both pod and service land inside the workload network.
				c.Kubernetes.PodCIDRs = []string{"100.100.1.0/24"}
				c.Kubernetes.ServiceCIDR = "100.100.2.0/24"
			},
			want:    results.StatusFail,
			wantHit: 2,
		},
		{
			// A malformed CIDR must be reported, not silently dropped from
			// every comparison. Otherwise the report looks cleaner than the
			// config is.
			name: "a malformed CIDR is a reported finding",
			mutate: func(c *config.Config) {
				c.Kubernetes.ServiceCIDR = "not-a-cidr"
			},
			want:    results.StatusFail,
			wantHit: 1,
		},
		{
			name: "a CIDR with host bits set is rejected rather than masked",
			mutate: func(c *config.Config) {
				c.Kubernetes.ServiceCIDR = "100.64.0.5/18"
			},
			want:    results.StatusFail,
			wantHit: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := base()
			tt.mutate(cfg)

			res, err := configval.Overlap{}.Run(context.Background(), rc(cfg))
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if got := statusOf(t, res); got != tt.want {
				t.Fatalf("status = %s, want %s (%d results)", got, tt.want, len(res))
			}
			if tt.want == results.StatusFail {
				fails := 0
				for _, r := range res {
					if r.Status == results.StatusFail {
						fails++
					}
				}
				if fails != tt.wantHit {
					t.Errorf("got %d failing rows, want %d", fails, tt.wantHit)
				}
			}
		})
	}
}

// An empty external list means the check had nothing to compare against.
// Reporting that as a pass would be the tool overstating its own coverage.
func TestExternalCollisionSkipsWhenNothingDeclared(t *testing.T) {
	t.Parallel()

	res, err := configval.ExternalCollision{}.Run(context.Background(), rc(base()))
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Status != results.StatusSkip {
		t.Errorf("status = %s, want skip when externalCIDRs is empty", res[0].Status)
	}
	if res[0].Remediation == "" {
		t.Error("the skip should tell the operator how to make the check useful")
	}
}

func TestExternalCollision(t *testing.T) {
	t.Parallel()

	cfg := base()
	// 100.96.0.0/12 spans 100.96.0.0-100.111.255.255 and so contains the
	// workload network. Note the prefix is written masked: 100.100.0.0/12 has
	// host bits set and netx.ParsePrefix rejects it rather than silently
	// masking, which is how the first draft of this test caught itself.
	cfg.Kubernetes.ExternalCIDRs = []string{"100.96.0.0/12"}

	res, err := configval.ExternalCollision{}.Run(context.Background(), rc(cfg))
	if err != nil {
		t.Fatal(err)
	}
	if statusOf(t, res) != results.StatusFail {
		t.Fatalf("expected a collision against the declared external range")
	}
	// The finding must name both sides by config path — an operator cannot act
	// on "10.0.0.0/8 collides with 10.20.0.0/16" without knowing where each came from.
	r := res[0]
	if r.Observed.Data["declared_source"] == nil || r.Observed.Data["external_source"] == nil {
		t.Error("finding does not name both config sources")
	}
}

// A pod CIDR containing the vCenter address makes vCenter unreachable from
// inside the cluster, weeks later, with a symptom that points nowhere near the
// cause. That is a distinct failure from a plain range overlap.
func TestInfraCollision(t *testing.T) {
	t.Parallel()

	cfg := base()
	cfg.Infrastructure.VCenter = config.Endpoint{FQDN: "vc.example.com", IP: "100.96.5.10"}

	res, err := configval.InfraCollision{}.Run(context.Background(), rc(cfg))
	if err != nil {
		t.Fatal(err)
	}
	if statusOf(t, res) != results.StatusFail {
		t.Fatal("expected the pod CIDR containing the vCenter address to be a finding")
	}
}

// Routable ranges containing an infrastructure address are ordinary overlaps,
// not this check's problem — otherwise every management network containing its
// own vCenter would be flagged.
func TestInfraCollisionIgnoresRoutableRanges(t *testing.T) {
	t.Parallel()

	cfg := base()
	cfg.Infrastructure.VCenter = config.Endpoint{FQDN: "vc.example.com", IP: "192.0.2.10"}

	res, err := configval.InfraCollision{}.Run(context.Background(), rc(cfg))
	if err != nil {
		t.Fatal(err)
	}
	if statusOf(t, res) != results.StatusPass {
		t.Error("a vCenter inside the routable management network is normal, not a finding")
	}
}

// The pass must disclose what it could not see. An endpoint declared only by
// FQDN has no address to compare, and a reader who does not know that will
// over-trust the result.
func TestInfraCollisionDisclosesUnresolvedEndpoints(t *testing.T) {
	t.Parallel()

	cfg := base()
	cfg.Infrastructure.VCenter = config.Endpoint{FQDN: "vc.example.com"} // no IP

	res, err := configval.InfraCollision{}.Run(context.Background(), rc(cfg))
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Observed.Data["fqdn_only_endpoints"] == nil {
		t.Error("the pass does not disclose the endpoints it could not check")
	}
}

func TestRangeContainment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		start, end string
		want       results.Status
		wantHow    string
	}{
		{"inside the subnet", "192.0.2.30", "192.0.2.34", results.StatusPass, ""},
		{"exactly the subnet", "192.0.2.0", "192.0.2.255", results.StatusPass, ""},
		{"runs past the end", "192.0.2.250", "192.0.3.10", results.StatusFail, "extends past the end of the subnet"},
		{"starts before the subnet", "192.0.1.250", "192.0.2.10", results.StatusFail, "starts before the subnet"},
		{"entirely elsewhere", "10.9.9.0", "10.9.9.10", results.StatusFail, "both ends are outside the subnet"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := base()
			cfg.Networks.Management.Ranges = []config.IPRange{{Start: tt.start, End: tt.end}}

			res, err := configval.RangeContainment{}.Run(context.Background(), rc(cfg))
			if err != nil {
				t.Fatal(err)
			}
			if got := statusOf(t, res); got != tt.want {
				t.Fatalf("status = %s, want %s", got, tt.want)
			}
			// "Not contained" leaves the operator to work out whether the
			// start, the end, or both are wrong. Say which.
			if tt.wantHow != "" && res[0].Observed.Data["how"] != tt.wantHow {
				t.Errorf("how = %v, want %q", res[0].Observed.Data["how"], tt.wantHow)
			}
		})
	}
}

// No declared ranges is not a pass. A green tick for work that was not done is
// worse than admitting it was not done.
func TestRangeContainmentSkipsWithNoRanges(t *testing.T) {
	t.Parallel()

	cfg := base()
	cfg.Networks.Management.Ranges = nil

	res, err := configval.RangeContainment{}.Run(context.Background(), rc(cfg))
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Status != results.StatusSkip {
		t.Errorf("status = %s, want skip", res[0].Status)
	}
}

// Every check in this package must be answerable in every mode and must trace
// to the requirements matrix. Enforced here so a new check cannot quietly ship
// without it.
func TestAllChecksAreWellFormed(t *testing.T) {
	t.Parallel()

	for _, c := range configval.Checks() {
		m := c.Meta()
		t.Run(m.ID, func(t *testing.T) {
			if len(m.RequirementIDs) == 0 {
				t.Error("no requirement IDs")
			}
			if len(m.Modes) != len(checks.AllModes) {
				t.Errorf("config arithmetic should work in every mode, got %v", m.Modes)
			}
			if m.Remediation == "" {
				t.Error("no remediation")
			}
			if m.EffectiveLayer() == "" {
				t.Error("no layer")
			}
			if len(m.Needs) != 0 {
				t.Errorf("class (c) checks need no capabilities, got %v", m.Needs)
			}
		})
	}
}

// Every result must carry a machine-comparable observation, or drift cannot
// diff it.
func TestResultsAreDiffable(t *testing.T) {
	t.Parallel()

	cfg := base()
	cfg.Kubernetes.ExternalCIDRs = []string{"203.0.113.0/24"}

	for _, c := range configval.Checks() {
		res, err := c.Run(context.Background(), rc(cfg))
		if err != nil {
			t.Fatalf("%s: %v", c.Meta().ID, err)
		}
		for _, r := range res {
			if len(r.Observed.Data) == 0 {
				t.Errorf("%s: Observed.Data is empty; drift has nothing to compare", r.CheckID)
			}
			if r.Observed.Summary == "" {
				t.Errorf("%s: Observed.Summary is empty; a human has nothing to read", r.CheckID)
			}
		}
	}
}
