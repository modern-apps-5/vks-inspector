package flb_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/modern-apps-5/vks-inspector/internal/checks"
	"github.com/modern-apps-5/vks-inspector/internal/checks/flb"
	"github.com/modern-apps-5/vks-inspector/internal/clients/vcenter"
	"github.com/modern-apps-5/vks-inspector/internal/config"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// fakeVCenter is a minimal checks.VCenterClient stub, not vcsim: the check
// under test reads exactly one field (About().version), and vcsim's version
// string lives in a shared, mutable package-level var
// (simulator/vpx.ServiceContent) that is not safe to override per test case
// without risking cross-test races. A fake is simpler and sufficient here.
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
			Topology:   config.Topology{Networking: config.NetVDS, LoadBalancer: config.LBFLB},
		},
		Clients: checks.Clients{VCenter: vc},
		Now:     func() time.Time { return at },
		Vantage: "test",
	}
}

func TestVersionSupported(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
		want    results.Status
	}{
		{"vCenter 9.0 supports FLB", "9.0.0", results.StatusPass},
		{"vCenter 9.1 supports FLB", "9.1.0", results.StatusPass},
		{"vCenter 10 supports FLB", "10.0.0", results.StatusPass},
		{"vCenter 8.0 does not support FLB", "8.0.2", results.StatusFail},
		{"vCenter 7.0 does not support FLB", "7.0.3", results.StatusFail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := flb.VersionSupported{}
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
			// A version boundary must never be asserted as a blocker without
			// severity backing it — Meta owns severity, this just confirms the
			// check does not override it to something softer for a real failure.
			if res[0].Status == results.StatusFail && c.Meta().Severity != results.SeverityBlocker {
				t.Errorf("severity = %s, want blocker — FLB genuinely does not exist below the minimum version",
					c.Meta().Severity)
			}
		})
	}
}

func TestVersionSupportedVCenterUnreachableIsUnknown(t *testing.T) {
	t.Parallel()
	c := flb.VersionSupported{}
	res, err := c.Run(context.Background(), runCtx(&fakeVCenter{aboutErr: errors.New("boom")}))
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Status != results.StatusUnknown {
		t.Errorf("status = %s, want unknown — an unreachable vCenter is vc.api-reachable's finding, not this check's",
			res[0].Status)
	}
}

func TestVersionSupportedUnparseableVersionIsUnknown(t *testing.T) {
	t.Parallel()
	c := flb.VersionSupported{}
	res, err := c.Run(context.Background(), runCtx(&fakeVCenter{version: "not-a-version"}))
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Status != results.StatusUnknown {
		t.Errorf("status = %s, want unknown", res[0].Status)
	}
}

func TestMetaIsWellFormed(t *testing.T) {
	t.Parallel()
	m := flb.VersionSupported{}.Meta()
	if m.ID == "" {
		t.Error("empty check ID")
	}
	if len(m.RequirementIDs) == 0 {
		t.Error("no requirement IDs")
	}
	if m.Remediation == "" {
		t.Error("no remediation")
	}
	if !m.AppliesTo(config.Topology{Networking: config.NetVDS, LoadBalancer: config.LBFLB}) {
		t.Error("should apply to vds+flb")
	}
	if m.AppliesTo(config.Topology{Networking: config.NetVDS, LoadBalancer: config.LBALB}) {
		t.Error("should not apply to vds+alb")
	}
}
