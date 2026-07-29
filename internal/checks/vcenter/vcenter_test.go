package vcenter_test

import (
	"context"
	"crypto/tls"
	"testing"
	"time"

	"github.com/vmware/govmomi/simulator"

	"github.com/modern-apps-5/vks-inspector/internal/checks"
	vccheck "github.com/modern-apps-5/vks-inspector/internal/checks/vcenter"
	"github.com/modern-apps-5/vks-inspector/internal/clients"
	vcclient "github.com/modern-apps-5/vks-inspector/internal/clients/vcenter"
	"github.com/modern-apps-5/vks-inspector/internal/config"
	"github.com/modern-apps-5/vks-inspector/internal/creds"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// These run the real checks against vcsim, so they exercise the whole path:
// check → client → SOAP → response parsing → Result. That is a stronger claim
// than a fake client can make, and a weaker one than a live lab.

func simEnv(t *testing.T) (*vcclient.Client, *vcclient.Inventory) {
	t.Helper()

	model := simulator.VPX()
	model.Datacenter = 1
	model.Cluster = 1
	model.ClusterHost = 3
	model.Portgroup = 2
	if err := model.Create(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(model.Remove)

	model.Service.TLS = new(tls.Config)
	server := model.Service.NewServer()
	t.Cleanup(server.Close)

	pw, _ := server.URL.User.Password()
	c := vcclient.New(server.URL.Host, creds.Credential{
		Username:           server.URL.User.Username(),
		Password:           pw,
		InsecureSkipVerify: true,
	}, clients.DefaultOptions())

	ctx := context.Background()
	if err := c.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close(ctx) })

	inv, err := c.Discover(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return c, inv
}

func runCtx(t *testing.T, c *vcclient.Client, mutate func(*config.Config)) *checks.RunContext {
	t.Helper()
	at := time.Date(2026, 3, 14, 9, 30, 0, 0, time.UTC)

	cfg := &config.Config{
		APIVersion: config.APIVersion,
		Kind:       config.Kind,
		Metadata:   config.Metadata{Name: "sim"},
		Topology:   config.Topology{Networking: config.NetVDS, LoadBalancer: config.LBALB},
	}
	if mutate != nil {
		mutate(cfg)
	}
	return &checks.RunContext{
		Mode:    checks.ModePreflight,
		Config:  cfg,
		Clients: checks.Clients{VCenter: c},
		Now:     func() time.Time { return at },
		Vantage: "test",
	}
}

// one asserts a check returned exactly one result. A method rather than a
// function so the multi-value Run(...) call can be its sole argument.
type one struct{ t *testing.T }

func (o one) only(res []results.Result, err error) results.Result {
	o.t.Helper()
	if err != nil {
		o.t.Fatalf("Run: %v", err)
	}
	if len(res) != 1 {
		o.t.Fatalf("got %d results, want 1", len(res))
	}
	return res[0]
}

func TestAPIReachable(t *testing.T) {
	c, _ := simEnv(t)

	r := one{t}.only(vccheck.APIReachable{}.Run(context.Background(), runCtx(t, c, nil)))
	if r.Status != results.StatusPass {
		t.Fatalf("status = %s: %s", r.Status, r.Observed.Summary)
	}
	if r.Observed.Data["version"] == nil || r.Observed.Data["build"] == nil {
		t.Errorf("observation carries no version/build: %+v", r.Observed.Data)
	}
	if r.Observed.Data["api_type"] != "VirtualCenter" {
		t.Errorf("api_type = %v", r.Observed.Data["api_type"])
	}
}

func TestClusterExists(t *testing.T) {
	c, inv := simEnv(t)

	t.Run("cluster that exists", func(t *testing.T) {
		rc := runCtx(t, c, func(cfg *config.Config) {
			cfg.VSphere.Datacenter = inv.Datacenters[0]
			cfg.VSphere.Cluster = inv.Clusters[0]
		})
		r := one{t}.only(vccheck.ClusterExists{}.Run(context.Background(), rc))
		if r.Status != results.StatusPass {
			t.Fatalf("status = %s: %s", r.Status, r.Observed.Summary)
		}
		if r.Observed.Data["host_count"] != 3 {
			t.Errorf("host_count = %v, want 3", r.Observed.Data["host_count"])
		}
	})

	t.Run("missing cluster fails and names what does exist", func(t *testing.T) {
		rc := runCtx(t, c, func(cfg *config.Config) {
			cfg.VSphere.Datacenter = inv.Datacenters[0]
			cfg.VSphere.Cluster = "Cluster-That-Does-Not-Exist"
		})
		r := one{t}.only(vccheck.ClusterExists{}.Run(context.Background(), rc))
		if r.Status != results.StatusFail {
			t.Fatalf("status = %s, want fail", r.Status)
		}
		// "Not found" alone is a guessing game. Listing what is there is a fix.
		if r.Observed.Data["available_clusters"] == nil {
			t.Error("failure does not name the clusters that do exist")
		}
	})

	t.Run("no cluster declared is a skip, not a pass", func(t *testing.T) {
		r := one{t}.only(vccheck.ClusterExists{}.Run(context.Background(), runCtx(t, c, nil)))
		if r.Status != results.StatusSkip {
			t.Errorf("status = %s, want skip", r.Status)
		}
	})
}

func TestSwitchExists(t *testing.T) {
	c, inv := simEnv(t)
	if len(inv.Switches) == 0 {
		t.Skip("simulator produced no distributed switch")
	}

	rc := runCtx(t, c, func(cfg *config.Config) {
		cfg.VSphere.DistributedSwitch = inv.Switches[0]
	})
	if r := (one{t}).only(vccheck.SwitchExists{}.Run(context.Background(), rc)); r.Status != results.StatusPass {
		t.Fatalf("status = %s: %s", r.Status, r.Observed.Summary)
	}

	bad := runCtx(t, c, func(cfg *config.Config) { cfg.VSphere.DistributedSwitch = "nope" })
	if r := (one{t}).only(vccheck.SwitchExists{}.Run(context.Background(), bad)); r.Status != results.StatusFail {
		t.Errorf("missing switch: status = %s, want fail", r.Status)
	}
}

// vcsim does not populate MaxMtu, so this exercises the case that matters most:
// an unreadable MTU must be indeterminate, never a failure. Asserting a switch
// is below spec when we never read its MTU would block a deployment over
// nothing.
func TestSwitchMTUUnreadableIsUnknownNotFail(t *testing.T) {
	c, inv := simEnv(t)
	if len(inv.Switches) == 0 {
		t.Skip("simulator produced no distributed switch")
	}

	rc := runCtx(t, c, func(cfg *config.Config) {
		cfg.VSphere.DistributedSwitch = inv.Switches[0]
		cfg.Topology = config.Topology{Networking: config.NetNSX, LoadBalancer: config.LBNSX}
		cfg.NSX = &config.NSX{OverlayMTU: 1700}
	})

	r := one{t}.only(vccheck.SwitchMTU{}.Run(context.Background(), rc))
	if r.Status != results.StatusUnknown {
		t.Errorf("status = %s, want unknown when vCenter reports no MTU (observed: %s)",
			r.Status, r.Observed.Summary)
	}
	if r.Expected.Data["required_mtu"] != 1700 {
		t.Errorf("required_mtu = %v, want 1700 from the config", r.Expected.Data["required_mtu"])
	}
}

// The MTU requirement comes from the config, not a constant in the check: the
// VCF 9 figure is flagged unconfirmed in the matrix, and hardcoding it would
// launder a guess into an assertion.
func TestSwitchMTUSkipsWithNoDeclaredRequirement(t *testing.T) {
	c, inv := simEnv(t)
	if len(inv.Switches) == 0 {
		t.Skip("simulator produced no distributed switch")
	}

	rc := runCtx(t, c, func(cfg *config.Config) {
		cfg.VSphere.DistributedSwitch = inv.Switches[0]
	})
	if r := (one{t}).only(vccheck.SwitchMTU{}.Run(context.Background(), rc)); r.Status != results.StatusSkip {
		t.Errorf("status = %s, want skip with no declared MTU requirement", r.Status)
	}
}

func TestPortGroupsExist(t *testing.T) {
	c, inv := simEnv(t)
	if len(inv.PortGroups) == 0 {
		t.Skip("simulator produced no portgroups")
	}

	t.Run("declared portgroup that exists", func(t *testing.T) {
		rc := runCtx(t, c, func(cfg *config.Config) {
			cfg.Networks.Management = config.NetworkSpec{Name: "mgmt", PortGroup: inv.PortGroups[0]}
		})
		res, err := vccheck.PortGroupsExist{}.Run(context.Background(), rc)
		if err != nil {
			t.Fatal(err)
		}
		if res[0].Status != results.StatusPass {
			t.Errorf("status = %s: %s", res[0].Status, res[0].Observed.Summary)
		}
	})

	t.Run("one result per declared portgroup", func(t *testing.T) {
		rc := runCtx(t, c, func(cfg *config.Config) {
			cfg.Networks.Management = config.NetworkSpec{Name: "mgmt", PortGroup: inv.PortGroups[0]}
			cfg.Networks.Workload = []config.NetworkSpec{{Name: "wl", PortGroup: "missing-pg"}}
		})
		res, err := vccheck.PortGroupsExist{}.Run(context.Background(), rc)
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 2 {
			t.Fatalf("got %d results, want one per declared portgroup", len(res))
		}
		// Each is a separate object with a separate fix, so each gets its own row.
		var sawFail bool
		for _, r := range res {
			if r.Status == results.StatusFail {
				sawFail = true
				if r.Target != "missing-pg" {
					t.Errorf("failing row targets %q, want missing-pg", r.Target)
				}
			}
		}
		if !sawFail {
			t.Error("the missing portgroup was not reported")
		}
	})

	t.Run("no portgroups declared is a skip", func(t *testing.T) {
		res, err := vccheck.PortGroupsExist{}.Run(context.Background(), runCtx(t, c, nil))
		if err != nil {
			t.Fatal(err)
		}
		if res[0].Status != results.StatusSkip {
			t.Errorf("status = %s, want skip", res[0].Status)
		}
	})
}

// Portgroups back workload networks only under VDS networking; under NSX the
// segments are created by NSX itself, so the check must not run there.
func TestPortGroupCheckIsVDSOnly(t *testing.T) {
	t.Parallel()
	m := vccheck.PortGroupsExist{}.Meta()

	if !m.AppliesTo(config.Topology{Networking: config.NetVDS, LoadBalancer: config.LBALB}) {
		t.Error("should apply to VDS networking")
	}
	if m.AppliesTo(config.Topology{Networking: config.NetNSX, LoadBalancer: config.LBNSX}) {
		t.Error("should not apply to NSX networking")
	}
}

// Every check in this package must declare the vCenter capability, or a run
// without credentials will fail an environment it never inspected.
func TestAllChecksDeclareTheVCenterCapability(t *testing.T) {
	t.Parallel()

	for _, c := range vccheck.Checks() {
		m := c.Meta()
		t.Run(m.ID, func(t *testing.T) {
			var found bool
			for _, n := range m.Needs {
				if n == checks.CapVCenter {
					found = true
				}
			}
			if !found {
				t.Error("does not declare CapVCenter; a credential-less run would fail it")
			}
			if len(m.RequirementIDs) == 0 {
				t.Error("no requirement IDs")
			}
			if m.Remediation == "" {
				t.Error("no remediation")
			}
		})
	}
}
