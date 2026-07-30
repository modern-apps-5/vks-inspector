package vcenter_test

import (
	"context"
	"crypto/tls"
	"errors"
	"strings"
	"testing"

	"github.com/vmware/govmomi/simulator"

	"github.com/modern-apps-5/vks-inspector/internal/clients"
	"github.com/modern-apps-5/vks-inspector/internal/clients/vcenter"
	"github.com/modern-apps-5/vks-inspector/internal/creds"
)

// These tests run against vcsim, govmomi's vSphere API simulator.
//
// That matters for what they do and do not prove. vcsim speaks the real SOAP
// API and returns real managed-object shapes, so these tests genuinely verify
// request construction, property paths and response parsing — the things a
// hand-written fixture cannot, because a hand-written fixture only tests the
// author's belief about the API, which is exactly the belief most likely to be
// wrong.
//
// What they do NOT prove: that a real vCenter behaves identically, that VCF 9
// returns these shapes, or anything about authentication against a real
// identity source. vcsim is a model, not the thing. Tier-3 integration tests
// against a live lab remain necessary and remain unwritten.
// See docs/unit-test-coverage.md.

// simVCenter starts a simulated vCenter and returns a connected client.
func simVCenter(t *testing.T) (*vcenter.Client, *simulator.Model) {
	t.Helper()

	model := simulator.VPX()
	model.Datacenter = 1
	model.Cluster = 1
	model.ClusterHost = 3
	model.Portgroup = 2
	if err := model.Create(); err != nil {
		t.Fatalf("create simulator model: %v", err)
	}
	t.Cleanup(model.Remove)

	// Serve HTTPS with a self-signed cert. A real vCenter is TLS, and testing
	// against plain HTTP would leave the entire transport and trust path
	// unexercised — including endpoint normalisation, which forces https.
	model.Service.TLS = new(tls.Config)
	server := model.Service.NewServer()
	t.Cleanup(server.Close)

	pw, _ := server.URL.User.Password()
	c := vcenter.New(server.URL.Host, creds.Credential{
		Username:           server.URL.User.Username(),
		Password:           pw,
		InsecureSkipVerify: true,
	}, clients.DefaultOptions())

	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	return c, model
}

func TestConnectAndAbout(t *testing.T) {
	c, _ := simVCenter(t)
	ctx := context.Background()

	about, err := c.About(ctx)
	if err != nil {
		t.Fatalf("About: %v", err)
	}
	for _, key := range []string{"name", "version", "build", "api_type"} {
		if about[key] == nil || about[key] == "" {
			t.Errorf("About() missing %q: %+v", key, about)
		}
	}

	// Pointing this tool at a bare ESXi host instead of vCenter is a common
	// mistake that produces confusing downstream failures if not caught early.
	isVC, err := c.IsVCenter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !isVC {
		t.Errorf("simulated VPX should report as a vCenter, got api_type=%v", about["api_type"])
	}
}

// A tool that leaves sessions accumulating on a customer's vCenter is doing
// something it was not asked to do.
func TestCloseEndsTheSession(t *testing.T) {
	c, _ := simVCenter(t)
	ctx := context.Background()

	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Close must be idempotent — the CLI defers it and may also call it on an
	// error path.
	if err := c.Close(ctx); err != nil {
		t.Errorf("second Close should be a no-op, got: %v", err)
	}
}

// A connection failure must never carry the password. govmomi embeds the
// request URL in some transport errors, and that URL carries userinfo.
//
// Note what this does NOT test: vcsim accepts any credentials, so it cannot
// exercise authentication rejection at all. An earlier version of this test
// asserted that bad credentials were refused and passed only because the
// transport was misconfigured — it was green for the wrong reason. Real
// authentication behaviour needs a live lab (tier 3).
func TestConnectionErrorsDoNotLeakCredentials(t *testing.T) {
	model := simulator.VPX()
	if err := model.Create(); err != nil {
		t.Fatal(err)
	}
	defer model.Remove()
	model.Service.TLS = new(tls.Config)
	server := model.Service.NewServer()
	endpoint := server.URL.Host
	server.Close() // nothing is listening now, so Connect must fail

	c := vcenter.New(endpoint, creds.Credential{
		Username:           "readonly@vsphere.local",
		Password:           "hunter2",
		InsecureSkipVerify: true,
	}, clients.DefaultOptions())

	err := c.Connect(context.Background())
	if err == nil {
		t.Fatal("expected the connection to fail against a closed server")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("password leaked into the error: %v", err)
	}
	if strings.Contains(err.Error(), "readonly@vsphere.local") {
		t.Errorf("username leaked into the error: %v", err)
	}
}

// Connecting with no credentials at all is a usage error, not an environment
// finding, and must be refused before any network call.
func TestConnectRequiresCredentials(t *testing.T) {
	c := vcenter.New("vcenter.example.com", creds.Credential{}, clients.DefaultOptions())
	if err := c.Connect(context.Background()); err == nil {
		t.Error("expected an error with no credentials")
	}
}

func TestDiscover(t *testing.T) {
	c, _ := simVCenter(t)

	inv, err := c.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if inv.Version == "" {
		t.Error("no version discovered")
	}
	if len(inv.Datacenters) != 1 {
		t.Errorf("datacenters = %v, want 1", inv.Datacenters)
	}
	if len(inv.Clusters) != 1 {
		t.Errorf("clusters = %v, want 1", inv.Clusters)
	}
	if inv.HostCount == 0 {
		t.Error("no hosts discovered")
	}
	if len(inv.Switches) == 0 {
		t.Error("no distributed switches discovered")
	}
}

func TestCluster(t *testing.T) {
	c, _ := simVCenter(t)
	ctx := context.Background()

	inv, err := c.Discover(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Datacenters) == 0 || len(inv.Clusters) == 0 {
		t.Fatalf("discovery found nothing to look up: %+v", inv)
	}

	got, err := c.Cluster(ctx, inv.Datacenters[0], inv.Clusters[0])
	if err != nil {
		t.Fatalf("Cluster: %v", err)
	}
	if got.Name != inv.Clusters[0] {
		t.Errorf("name = %q, want %q", got.Name, inv.Clusters[0])
	}
	if got.HostCount != 3 {
		t.Errorf("host count = %d, want 3", got.HostCount)
	}
	if len(got.Hosts) != got.HostCount {
		t.Errorf("listed %d hosts but counted %d", len(got.Hosts), got.HostCount)
	}
}

// "The cluster does not exist" is a finding about the environment. "We could
// not reach vCenter" is a statement about the tool's access. A caller must be
// able to tell them apart, because they are different results with different
// remediations.
func TestMissingObjectsAreDistinguishable(t *testing.T) {
	c, _ := simVCenter(t)
	ctx := context.Background()

	var nf *vcenter.NotFoundError

	if _, err := c.Cluster(ctx, "", "no-such-cluster"); !errors.As(err, &nf) {
		t.Errorf("missing cluster: got %v, want NotFoundError", err)
	} else if !strings.Contains(nf.Error(), "no-such-cluster") {
		t.Errorf("error should name the object: %v", nf)
	}

	if _, err := c.DistributedSwitch(ctx, "no-such-switch"); !errors.As(err, &nf) {
		t.Errorf("missing switch: got %v, want NotFoundError", err)
	}
	if _, err := c.PortGroup(ctx, "no-such-portgroup"); !errors.As(err, &nf) {
		t.Errorf("missing portgroup: got %v, want NotFoundError", err)
	}
}

func TestDistributedSwitch(t *testing.T) {
	c, _ := simVCenter(t)
	ctx := context.Background()

	inv, err := c.Discover(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Switches) == 0 {
		t.Skip("simulator produced no distributed switch")
	}

	sw, err := c.DistributedSwitch(ctx, inv.Switches[0])
	if err != nil {
		t.Fatalf("DistributedSwitch: %v", err)
	}
	if sw.Name != inv.Switches[0] {
		t.Errorf("name = %q, want %q", sw.Name, inv.Switches[0])
	}
	// A switch cannot have an MTU of zero. Zero means the property was not
	// populated, and reporting it as a real value would make an MTU check fail
	// an environment it never measured. The client normalises that to -1.
	// vcsim does not populate MaxMtu, so -1 is the expected value here.
	if sw.MTU == 0 {
		t.Error("MTU of 0 is indistinguishable from unread; want a real value or -1")
	}
}

// A portgroup carrying a trunk means something different from one carrying
// access VLAN 0, and a check that ignores the distinction will misreport a
// trunk as an untagged network.
func TestPortGroupReportsVLANKind(t *testing.T) {
	c, _ := simVCenter(t)

	pg, err := findAnyPortGroup(t, c)
	if err != nil {
		t.Skip("simulator produced no distributed portgroup")
	}
	if pg.VLANKind == "" {
		t.Error("VLANKind not set")
	}
	if pg.VLANKind == "unknown" && pg.VLAN != -1 {
		t.Errorf("VLAN %d reported with unknown kind", pg.VLAN)
	}
}

// findAnyPortGroup asks discovery what exists rather than guessing at the
// simulator's naming. A test that hardcodes names is testing the simulator.
func findAnyPortGroup(t *testing.T, c *vcenter.Client) (*vcenter.PortGroupInfo, error) {
	t.Helper()
	ctx := context.Background()

	inv, err := c.Discover(ctx)
	if err != nil {
		return nil, err
	}
	if len(inv.PortGroups) == 0 {
		return nil, errors.New("no portgroup in the inventory")
	}
	return c.PortGroup(ctx, inv.PortGroups[0])
}

func TestClusterHostTime(t *testing.T) {
	c, _ := simVCenter(t)
	ctx := context.Background()

	inv, err := c.Discover(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Datacenters) == 0 || len(inv.Clusters) == 0 {
		t.Fatalf("discovery found nothing to look up: %+v", inv)
	}

	hosts, err := c.ClusterHostTime(ctx, inv.Datacenters[0], inv.Clusters[0])
	if err != nil {
		t.Fatalf("ClusterHostTime: %v", err)
	}
	if len(hosts) != 3 {
		t.Fatalf("got %d hosts, want 3", len(hosts))
	}
	for _, h := range hosts {
		if h.Host == "" {
			t.Error("host with no name")
		}
	}
	// Sorted output keeps reports and baselines byte-comparable.
	for i := 1; i < len(hosts); i++ {
		if hosts[i-1].Host > hosts[i].Host {
			t.Errorf("hosts not sorted: %v", hosts)
			break
		}
	}
}

// An operator types a bare FQDN, a host:port, or pastes a full URL. All three
// have to work; making someone remember "/sdk" is friction with nothing to show
// for it.
func TestEndpointFormsAllConnect(t *testing.T) {
	model := simulator.VPX()
	if err := model.Create(); err != nil {
		t.Fatal(err)
	}
	defer model.Remove()
	model.Service.TLS = new(tls.Config)
	server := model.Service.NewServer()
	defer server.Close()

	pw, _ := server.URL.User.Password()
	cred := creds.Credential{
		Username:           server.URL.User.Username(),
		Password:           pw,
		InsecureSkipVerify: true,
	}

	for _, endpoint := range []string{
		server.URL.Host,
		"https://" + server.URL.Host,
		"https://" + server.URL.Host + "/sdk",
	} {
		t.Run(endpoint, func(t *testing.T) {
			c := vcenter.New(endpoint, cred, clients.DefaultOptions())
			if err := c.Connect(context.Background()); err != nil {
				t.Errorf("connect to %q: %v", endpoint, err)
			}
			_ = c.Close(context.Background())
		})
	}
}
