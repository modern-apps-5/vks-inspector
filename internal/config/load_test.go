package config_test

import (
	"strings"
	"testing"

	"github.com/modern-apps-5/vks-inspector/internal/config"
)

// The shipped example must always be loadable. It is the first thing a user
// runs and the de facto documentation for the schema; an example that no longer
// parses is worse than no example.
func TestExampleConfigLoads(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load("../../config/example.yaml")
	if err != nil {
		t.Fatalf("config/example.yaml does not load: %v", err)
	}
	if cfg.Topology.Networking != config.NetNSX || cfg.Topology.LoadBalancer != config.LBNSX {
		t.Errorf("topology = %s", cfg.Topology)
	}
	if cfg.Digest() == "" {
		t.Error("empty config digest")
	}
}

// The digest must depend on content, not on formatting. Drift uses it to tell
// "the declared intent changed" from "the environment changed".
func TestDigestIgnoresFormatting(t *testing.T) {
	t.Parallel()

	a := mustParse(t, minimalConfig)
	b := mustParse(t, strings.Replace(minimalConfig,
		"  name: digest-test\n",
		"\n  # a comment that changes nothing\n  name: digest-test\n", 1))

	if a.Digest() != b.Digest() {
		t.Error("digest changed when only comments and whitespace changed")
	}

	c := mustParse(t, strings.Replace(minimalConfig, "digest-test", "digest-test-2", 1))
	if a.Digest() == c.Digest() {
		t.Error("digest did not change when content changed")
	}
}

func TestStructuralValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name:    "valid minimal config",
			yaml:    minimalConfig,
			wantErr: "",
		},
		{
			name:    "unknown apiVersion is rejected rather than best-effort parsed",
			yaml:    strings.Replace(minimalConfig, config.APIVersion, "vksinspect/v99", 1),
			wantErr: "apiVersion",
		},
		{
			name:    "unknown networking value is rejected",
			yaml:    strings.Replace(minimalConfig, "networking: nsx", "networking: openshift", 1),
			wantErr: "networking \"openshift\" is not one of",
		},
		{
			// The axes are independent but not freely combinable. An
			// unsupported pairing must be refused, not assumed workable:
			// telling someone their unsupported design passed preflight is
			// the worst thing this tool could do.
			name:    "valid axes in an unsupported combination are rejected",
			yaml:    strings.Replace(minimalConfig, "networking: nsx", "networking: vds", 1),
			wantErr: "not a supported combination",
		},
		{
			name:    "nsx networking without an nsx block is incoherent",
			yaml:    strings.Replace(minimalConfig, "nsx:\n  tier0Gateway: T0\n", "", 1),
			wantErr: "requires an `nsx:` block",
		},
		{
			name: "vds+flb topology without an flb block is incoherent",
			yaml: strings.Replace(
				strings.Replace(minimalConfig, "networking: nsx", "networking: vds", 1),
				"loadBalancer: nsx-lb", "loadBalancer: flb", 1),
			wantErr: "requires an `flb:` block",
		},
		{
			name:    "missing metadata.name",
			yaml:    strings.Replace(minimalConfig, "  name: digest-test\n", "", 1),
			wantErr: "metadata.name",
		},
		{
			// A typo'd key must not be silently ignored — a check asserting
			// against an empty string is worse than a parse error.
			name:    "typo'd key is rejected, not ignored",
			yaml:    strings.Replace(minimalConfig, "serviceCIDR:", "serviceCidr:", 1),
			wantErr: "field serviceCidr not found",
		},
		{
			// Credentials must never live in this document.
			name: "credential-shaped key is refused",
			yaml: strings.Replace(minimalConfig,
				"    fqdn: vcenter.example.com\n",
				"    fqdn: vcenter.example.com\n    password: hunter2\n", 1),
			wantErr: "not found", // KnownFields catches it first
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.Parse(strings.NewReader(tt.yaml))
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.wantErr != "" && err == nil:
				t.Fatalf("expected an error containing %q, got none", tt.wantErr)
			case tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr):
				t.Fatalf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

func mustParse(t *testing.T, y string) *config.Config {
	t.Helper()
	cfg, err := config.Parse(strings.NewReader(y))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return cfg
}

const minimalConfig = `apiVersion: ` + config.APIVersion + `
kind: EnvironmentSpec
metadata:
  name: digest-test
topology:
  networking: nsx
  loadBalancer: nsx-lb
infrastructure:
  vcenter:
    fqdn: vcenter.example.com
services:
  dns:
    servers: ["192.0.2.53"]
    requireReverse: true
  ntp:
    servers: ["192.0.2.123"]
    maxSkewSeconds: 30
vsphere:
  datacenter: DC1
  cluster: C1
networks:
  management:
    name: management
    cidr: 192.0.2.0/24
    gateway: 192.0.2.1
    routable: true
kubernetes:
  podCIDRs: ["10.244.0.0/20"]
  serviceCIDR: 10.96.0.0/22
nsx:
  tier0Gateway: T0
scale:
  supervisorControlPlaneNodes: 3
policy:
  allowInvasive: false
`

// nil and empty are different answers for externalCIDRs: nil means "nobody has
// been asked", empty means "asked, and the answer was none". Collapsing them
// makes a saved config re-prompt forever, which defeats the point of saving it.
func TestEmptyExternalCIDRsSurvivesARoundTrip(t *testing.T) {
	t.Parallel()

	answered := strings.Replace(minimalConfig,
		"  serviceCIDR: 10.96.0.0/22\n",
		"  serviceCIDR: 10.96.0.0/22\n  externalCIDRs: []\n", 1)

	cfg := mustParse(t, answered)
	if cfg.Kubernetes.ExternalCIDRs == nil {
		t.Fatal("an explicit empty list was read back as nil — the answer was lost")
	}
	if len(cfg.Kubernetes.ExternalCIDRs) != 0 {
		t.Errorf("got %v, want empty", cfg.Kubernetes.ExternalCIDRs)
	}

	// Absent means never asked, and must stay distinguishable.
	unanswered := mustParse(t, minimalConfig)
	if unanswered.Kubernetes.ExternalCIDRs != nil {
		t.Error("an absent key should read back as nil, not as an empty list")
	}
}
