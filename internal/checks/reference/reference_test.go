package reference_test

import (
	"context"
	"testing"
	"time"

	"github.com/modern-apps-5/vks-inspector/internal/checks"
	"github.com/modern-apps-5/vks-inspector/internal/checks/reference"
	"github.com/modern-apps-5/vks-inspector/internal/config"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

func topo(n config.Networking, lb config.LoadBalancer) config.Topology {
	return config.Topology{Networking: n, LoadBalancer: lb}
}

func fixedClock() func() time.Time {
	t := time.Date(2026, 3, 14, 9, 30, 0, 0, time.UTC)
	return func() time.Time { return t }
}

func runCtx(mode checks.Mode, t config.Topology) *checks.RunContext {
	return &checks.RunContext{
		Mode: mode,
		Config: &config.Config{
			APIVersion: config.APIVersion,
			Kind:       config.Kind,
			Metadata:   config.Metadata{Name: "unit-test"},
			Topology:   t,
		},
		Now:     fixedClock(),
		Vantage: "test",
	}
}

// The check must behave identically in every mode. This is the property the
// whole architecture rests on — if a check's verdict depends on which command
// invoked it, it cannot be reused by verify, snapshot or drift.
func TestRunsIdenticallyInEveryMode(t *testing.T) {
	t.Parallel()
	c := &reference.TopologyRecognised{KnownCheckCount: 1}
	want := topo(config.NetNSX, config.LBNSX)

	for _, mode := range checks.AllModes {
		t.Run(string(mode), func(t *testing.T) {
			res, err := c.Run(context.Background(), runCtx(mode, want))
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if len(res) != 1 {
				t.Fatalf("got %d results, want 1", len(res))
			}
			r := res[0]
			if r.Status != results.StatusPass {
				t.Errorf("status = %s, want pass", r.Status)
			}
			if r.Mode != string(mode) {
				t.Errorf("result did not record its mode: got %q, want %q", r.Mode, mode)
			}
			if got := r.Observed.Data["networking"]; got != string(want.Networking) {
				t.Errorf("Observed.Data[networking] = %v, want %v", got, want.Networking)
			}
			if got := r.Observed.Data["load_balancer"]; got != string(want.LoadBalancer) {
				t.Errorf("Observed.Data[load_balancer] = %v, want %v", got, want.LoadBalancer)
			}
		})
	}
}

func TestGradesEveryTopologyCombination(t *testing.T) {
	t.Parallel()
	c := &reference.TopologyRecognised{KnownCheckCount: 1}

	tests := []struct {
		name     string
		topology config.Topology
		want     results.Status
	}{
		{"nsx with the nsx load balancer", topo(config.NetNSX, config.LBNSX), results.StatusPass},
		{"nsx with alb", topo(config.NetNSX, config.LBALB), results.StatusPass},
		{"vds with alb", topo(config.NetVDS, config.LBALB), results.StatusPass},
		{"vds with haproxy is supported but caveated", topo(config.NetVDS, config.LBHAProxy), results.StatusPass},
		{"nsx-vpc with alb is supported but caveated", topo(config.NetNSXVPC, config.LBALB), results.StatusPass},
		// The axes are independent but not freely combinable. An unsupported
		// pairing of two individually-valid values must fail: telling someone
		// their unsupported design passed preflight is the worst outcome here.
		{"vds with the nsx load balancer is not a real combination", topo(config.NetVDS, config.LBNSX), results.StatusFail},
		{"unknown networking fails rather than being ignored", topo("openshift", config.LBALB), results.StatusFail},
		{"unknown load balancer fails", topo(config.NetNSX, "f5"), results.StatusFail},
		{"empty topology fails", config.Topology{}, results.StatusFail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res, err := c.Run(context.Background(), runCtx(checks.ModePreflight, tt.topology))
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if res[0].Status != tt.want {
				t.Errorf("status = %s, want %s (observed: %s)", res[0].Status, tt.want, res[0].Observed.Summary)
			}
		})
	}
}

// A supported-but-caveated combination passes, and the caveat is carried into
// the report. Refusing to grade HAProxy or VPC would only be more honest if we
// were certain they were unsupported, and we are not.
func TestCaveatedCombinationPassesWithTheCaveatVisible(t *testing.T) {
	t.Parallel()
	c := &reference.TopologyRecognised{}

	res, err := c.Run(context.Background(), runCtx(checks.ModePreflight, topo(config.NetVDS, config.LBHAProxy)))
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Status != results.StatusPass {
		t.Fatalf("status = %s, want pass", res[0].Status)
	}
	if len(res[0].Observed.Summary) == 0 || res[0].Observed.Summary == topo(config.NetVDS, config.LBHAProxy).String() {
		t.Error("caveat was not surfaced in the observation")
	}
}

// Every result must be diffable by a later drift run. A Result whose Observed
// carries only prose is invisible to drift, so this is a structural rule, not a
// style preference.
func TestObservationIsMachineComparable(t *testing.T) {
	t.Parallel()
	c := &reference.TopologyRecognised{KnownCheckCount: 7}

	res, err := c.Run(context.Background(), runCtx(checks.ModeSnapshot, topo(config.NetVDS, config.LBALB)))
	if err != nil {
		t.Fatal(err)
	}
	r := res[0]

	if len(r.Observed.Data) == 0 {
		t.Fatal("Observed.Data is empty; drift has nothing to compare")
	}
	if r.Observed.Summary == "" {
		t.Error("Observed.Summary is empty; a human has nothing to read")
	}
	if len(r.RequirementIDs) == 0 {
		t.Error("result carries no requirement IDs; it traces to nothing in the matrix")
	}
	if r.Observed.Data["known_check_count"] != 7 {
		t.Errorf("known_check_count = %v, want 7", r.Observed.Data["known_check_count"])
	}
}

// Timestamps must come from the injected clock. A check that calls time.Now()
// directly cannot be golden-tested and will make the whole report
// nondeterministic.
func TestUsesInjectedClock(t *testing.T) {
	t.Parallel()
	c := &reference.TopologyRecognised{}
	rc := runCtx(checks.ModePreflight, topo(config.NetNSX, config.LBNSX))

	res, err := c.Run(context.Background(), rc)
	if err != nil {
		t.Fatal(err)
	}
	want := rc.Now()
	if !res[0].StartedAt.Equal(want) || !res[0].FinishedAt.Equal(want) {
		t.Errorf("timestamps did not come from the injected clock: started=%v finished=%v want=%v",
			res[0].StartedAt, res[0].FinishedAt, want)
	}
}

// Meta must be answerable without running anything — the registry filters on it
// before any check executes.
func TestMetaIsWellFormed(t *testing.T) {
	t.Parallel()
	m := (&reference.TopologyRecognised{}).Meta()

	if m.ID == "" {
		t.Error("empty check ID")
	}
	if len(m.RequirementIDs) == 0 {
		t.Error("no requirement IDs")
	}
	if len(m.Modes) != len(checks.AllModes) {
		t.Errorf("check supports %d modes, want all %d", len(m.Modes), len(checks.AllModes))
	}
	if m.EffectiveLayer() != results.LayerSupervisor {
		t.Errorf("layer = %s, want supervisor", m.EffectiveLayer())
	}
	// This check is what tells you the topology was understood, so it must not
	// itself be filterable by topology.
	for _, c := range config.SupportedCombinations() {
		if !m.AppliesTo(c) {
			t.Errorf("check should apply to every topology, but not %s", c)
		}
	}
	if m.Remediation == "" {
		t.Error("no remediation; a failing check with no remediation is a dead end for the operator")
	}
}
