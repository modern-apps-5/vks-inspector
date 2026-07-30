package alb_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/modern-apps-5/vks-inspector/internal/checks"
	"github.com/modern-apps-5/vks-inspector/internal/checks/alb"
	"github.com/modern-apps-5/vks-inspector/internal/clients/vcenter"
	"github.com/modern-apps-5/vks-inspector/internal/config"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// fakeVCenter is a minimal checks.VCenterClient stub — see the identical
// comment in internal/checks/flb/flb_test.go for why this is a fake rather
// than vcsim.
type fakeVCenter struct {
	version  string
	aboutErr error
}

var _ checks.VCenterClient = (*fakeVCenter)(nil)

func (f *fakeVCenter) Endpoint() string { return "vcenter.example.com" }

func (f *fakeVCenter) About(context.Context) (map[string]any, error) {
	if f.aboutErr != nil {
		return nil, f.aboutErr
	}
	return map[string]any{"version": f.version}, nil
}

func (f *fakeVCenter) IsVCenter(context.Context) (bool, error) { return true, nil }
func (f *fakeVCenter) Discover(context.Context) (*vcenter.Inventory, error) {
	return nil, nil
}
func (f *fakeVCenter) Cluster(context.Context, string, string) (*vcenter.ClusterInfo, error) {
	return nil, nil
}
func (f *fakeVCenter) DistributedSwitch(context.Context, string) (*vcenter.SwitchInfo, error) {
	return nil, nil
}
func (f *fakeVCenter) PortGroup(context.Context, string) (*vcenter.PortGroupInfo, error) {
	return nil, nil
}
func (f *fakeVCenter) ClusterHostTime(context.Context, string, string) ([]vcenter.HostTimeInfo, error) {
	return nil, nil
}

func runCtx(vc checks.VCenterClient) *checks.RunContext {
	at := time.Date(2026, 3, 14, 9, 30, 0, 0, time.UTC)
	return &checks.RunContext{
		Mode: checks.ModePreflight,
		Config: &config.Config{
			APIVersion: config.APIVersion,
			Kind:       config.Kind,
			Metadata:   config.Metadata{Name: "unit-test"},
			Topology:   config.Topology{Networking: config.NetVDS, LoadBalancer: config.LBHAProxy},
		},
		Clients: checks.Clients{VCenter: vc},
		Now:     func() time.Time { return at },
		Vantage: "test",
	}
}

func TestHAProxyVersionSupported(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
		want    results.Status
	}{
		{"vCenter 8.0 fully supports HAProxy", "8.0.2", results.StatusPass},
		{"vCenter 7.0 fully supports HAProxy", "7.0.3", results.StatusPass},
		{"vCenter 9.0 is a warning, not a block", "9.0.0", results.StatusFail},
		{"vCenter 9.1 is a warning, not a block", "9.1.0", results.StatusFail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := alb.HAProxyVersionSupported{}
			res, err := c.Run(context.Background(), runCtx(&fakeVCenter{version: tt.version}))
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if len(res) != 1 {
				t.Fatalf("got %d results, want 1", len(res))
			}
			if res[0].Status != tt.want {
				t.Errorf("status = %s, want %s (observed: %s)", res[0].Status, tt.want, res[0].Observed.Summary)
			}
		})
	}
}

// The deprecation must never escalate to a blocker: HAProxy still works on
// vCenter 9.x, it is only being phased out. Confirms this is a property of
// Meta, which policy overrides can still change, not something the check
// hardcodes at the result level.
func TestHAProxyVersionSupportedNeverBlocks(t *testing.T) {
	t.Parallel()
	m := alb.HAProxyVersionSupported{}.Meta()
	if m.Severity != results.SeverityWarning {
		t.Errorf("severity = %s, want warning — a deprecation notice must not fail deployment by itself", m.Severity)
	}
}

func TestHAProxyVersionSupportedVCenterUnreachableIsUnknown(t *testing.T) {
	t.Parallel()
	c := alb.HAProxyVersionSupported{}
	res, err := c.Run(context.Background(), runCtx(&fakeVCenter{aboutErr: errors.New("boom")}))
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Status != results.StatusUnknown {
		t.Errorf("status = %s, want unknown", res[0].Status)
	}
}

func TestHAProxyVersionSupportedMetaIsWellFormed(t *testing.T) {
	t.Parallel()
	m := alb.HAProxyVersionSupported{}.Meta()
	if m.ID == "" {
		t.Error("empty check ID")
	}
	if len(m.RequirementIDs) == 0 {
		t.Error("no requirement IDs")
	}
	if m.Remediation == "" {
		t.Error("no remediation")
	}
	if !m.AppliesTo(config.Topology{Networking: config.NetVDS, LoadBalancer: config.LBHAProxy}) {
		t.Error("should apply to vds+haproxy")
	}
	if m.AppliesTo(config.Topology{Networking: config.NetVDS, LoadBalancer: config.LBFLB}) {
		t.Error("should not apply to vds+flb")
	}
}
