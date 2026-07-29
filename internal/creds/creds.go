// Package creds resolves credentials from the environment or a separate
// credentials file.
//
// Rules, enforced here so no caller has to remember them:
//   - credentials never come from the config YAML;
//   - credentials are never written to a Result, a report, or a baseline;
//   - credentials are never logged, including in error strings.
//
// See docs/ADR/0005-credential-handling.md.
package creds

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// EnvPrefix is the environment-variable namespace, e.g. VKSINSPECT_VCENTER_PASSWORD.
const EnvPrefix = "VKSINSPECT_"

// Credential is one principal. The password field is deliberately unexported
// from serialisation: the yaml tags exist for *reading* a credentials file, and
// the type implements no JSON marshalling at all so it cannot end up in a
// report by accident.
type Credential struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	// Token is an alternative to username/password for APIs that accept one.
	Token string `yaml:"token,omitempty"`
	// CACertFile is a path, not PEM content.
	CACertFile string `yaml:"caCertFile,omitempty"`
	// InsecureSkipVerify disables TLS verification for this endpoint only.
	// Off by default; using it makes any cert check for this endpoint
	// meaningless and the tool says so in the report.
	InsecureSkipVerify bool `yaml:"insecureSkipVerify,omitempty"`
}

// String redacts. This is the whole point of the type — a stray %v or %s in a
// log line or an error must not leak.
func (c Credential) String() string {
	if c.Username != "" {
		return fmt.Sprintf("Credential{username:%s, password:<redacted>}", c.Username)
	}
	return "Credential{<redacted>}"
}

// GoString redacts under %#v too.
func (c Credential) GoString() string { return c.String() }

// MarshalJSON refuses. If something tries to serialise a credential into an
// artifact, that is a bug and it should fail loudly rather than leak quietly.
func (c Credential) MarshalJSON() ([]byte, error) {
	return nil, errors.New("credentials must never be serialised into an artifact")
}

// Set is the resolved credential map, keyed by credential reference
// ("vcenter", "nsx", "alb", "haproxy", or a custom credentialRef from config).
type Set struct {
	byRef map[string]Credential
}

// Keys returns the available credential references, sorted. Safe to log — these
// are names, not secrets.
func (s *Set) Keys() []string {
	if s == nil {
		return nil
	}
	out := make([]string, 0, len(s.byRef))
	for k := range s.byRef {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Get returns a credential by reference. Nil-safe: an absent credentials set
// behaves as an empty one, so checks handle it via capability skipping rather
// than nil panics.
func (s *Set) Get(ref string) (Credential, bool) {
	if s == nil || s.byRef == nil {
		return Credential{}, false
	}
	c, ok := s.byRef[strings.ToLower(ref)]
	return c, ok
}

// Has reports whether a credential reference is available.
func (s *Set) Has(ref string) bool {
	_, ok := s.Get(ref)
	return ok
}

// file is the on-disk credentials format.
type file struct {
	APIVersion  string                `yaml:"apiVersion"`
	Kind        string                `yaml:"kind"`
	Credentials map[string]Credential `yaml:"credentials"`
}

// Load resolves credentials. Environment variables win over the file, so a
// pipeline can inject a secret without rewriting a file on disk.
//
// path may be empty, in which case only the environment is consulted.
func Load(path string) (*Set, error) {
	s := &Set{byRef: map[string]Credential{}}

	if path != "" {
		if err := s.loadFile(path); err != nil {
			return nil, err
		}
	}
	s.loadEnv()
	return s, nil
}

func (s *Set) loadFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("credentials file: %w", err)
	}
	// A world- or group-readable credentials file is a finding in itself.
	// Refuse rather than warn: the alternative is a customer leaving 0644
	// credentials on a shared jump host because a warning scrolled past.
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return fmt.Errorf("credentials file %s has permissions %04o; tighten to 0600 before use", path, perm)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read credentials file: %w", err)
	}
	var f file
	if err := yaml.Unmarshal(raw, &f); err != nil {
		// Deliberately not wrapping the underlying error's context beyond the
		// message: yaml errors can quote the offending line.
		return errors.New("parse credentials file: invalid YAML")
	}
	for ref, cred := range f.Credentials {
		s.byRef[strings.ToLower(ref)] = cred
	}
	return nil
}

// loadEnv reads VKSINSPECT_<REF>_USERNAME / _PASSWORD / _TOKEN.
func (s *Set) loadEnv() {
	for _, kv := range os.Environ() {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(k, EnvPrefix) || v == "" {
			continue
		}
		rest := strings.TrimPrefix(k, EnvPrefix)
		ref, field, ok := lastCut(rest, "_")
		if !ok {
			continue
		}
		lref := strings.ToLower(ref)
		cred := s.byRef[lref]
		switch strings.ToUpper(field) {
		case "USERNAME":
			cred.Username = v
		case "PASSWORD":
			cred.Password = v
		case "TOKEN":
			cred.Token = v
		case "CACERTFILE":
			cred.CACertFile = v
		default:
			continue
		}
		s.byRef[lref] = cred
	}
}

func lastCut(s, sep string) (before, after string, found bool) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+len(sep):], true
}
