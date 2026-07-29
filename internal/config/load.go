package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// Load reads and structurally validates a config file.
//
// "Structurally" is the important word. This layer answers "is this a
// well-formed EnvironmentSpec for a known topology" and nothing else. It does
// not answer "do these CIDRs overlap" or "is this pool big enough" — those are
// checks, they produce Results, and they must be reportable rather than fatal.
// Rejecting a config here means the user cannot even see the report.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()
	return Parse(f)
}

// Parse reads a config from any reader.
func Parse(r io.Reader) (*Config, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	// KnownFields catches typo'd keys instead of silently ignoring them. A
	// silently-ignored `ingressCidr:` would mean a check quietly asserting
	// against an empty string.
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.validateStructure(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) validateStructure() error {
	var errs []error

	if c.APIVersion != APIVersion {
		errs = append(errs, fmt.Errorf(
			"apiVersion %q is not supported by this build (expected %q)", c.APIVersion, APIVersion))
	}
	if c.Kind != Kind {
		errs = append(errs, fmt.Errorf("kind %q is not supported (expected %q)", c.Kind, Kind))
	}
	if c.Metadata.Name == "" {
		errs = append(errs, errors.New("metadata.name is required; it names the environment in reports and baselines"))
	}
	if !c.Topology.Valid() {
		errs = append(errs, fmt.Errorf("topology %q is not one of %v", c.Topology, AllTopologies))
	}

	// Per-topology structural requirements. These are about whether the
	// document is coherent enough to run checks against, not about whether the
	// values are correct.
	switch {
	case c.Topology.UsesNSX() && c.NSX == nil:
		errs = append(errs, fmt.Errorf("topology %q requires an `nsx:` block", c.Topology))
	case c.Topology.UsesALB() && c.ALB == nil:
		errs = append(errs, fmt.Errorf("topology %q requires an `alb:` block", c.Topology))
	case c.Topology.UsesHAProxy() && c.HAProxy == nil:
		errs = append(errs, fmt.Errorf("topology %q requires a `haproxy:` block", c.Topology))
	}
	if c.Topology == TopologyNSXVPC && (c.NSX == nil || c.NSX.VPC == nil) {
		errs = append(errs, errors.New("topology nsx-vpc requires an `nsx.vpc:` block"))
	}
	if c.Infrastructure.VCenter.FQDN == "" {
		errs = append(errs, errors.New("infrastructure.vcenter.fqdn is required"))
	}

	// Credentials must never appear in this document. Catching it here is a
	// safety net, not the mechanism — see internal/creds and
	// docs/ADR/0005-credential-handling.md.
	if err := c.assertNoSecrets(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// forbiddenKeys are field names that must never be accepted in the config
// document. yaml.KnownFields already rejects unknown keys, so this is a
// belt-and-braces assertion that no future struct field reintroduces one.
var forbiddenKeys = []string{"password", "passwd", "secret", "token", "apiKey", "privateKey"}

func (c *Config) assertNoSecrets() error {
	blob, err := json.Marshal(c)
	if err != nil {
		return nil // marshalling failure is not a secrets problem
	}
	var generic map[string]any
	if err := json.Unmarshal(blob, &generic); err != nil {
		return nil
	}
	var found []string
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			for k, vv := range t {
				for _, bad := range forbiddenKeys {
					if equalFold(k, bad) {
						found = append(found, k)
					}
				}
				walk(vv)
			}
		case []any:
			for _, vv := range t {
				walk(vv)
			}
		}
	}
	walk(generic)
	if len(found) > 0 {
		return fmt.Errorf("config contains credential-shaped keys %v; credentials belong in the credentials file or environment, never in the config", found)
	}
	return nil
}

// Digest returns a stable SHA-256 over the normalised config.
//
// Drift needs to distinguish "the environment changed" from "the declared
// intent changed". Without this, a config edit looks identical to an
// infrastructure regression.
func (c *Config) Digest() string {
	// JSON rather than the original YAML bytes: comments and key order must not
	// change the digest.
	blob, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(blob)
	return hex.EncodeToString(sum[:])
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
