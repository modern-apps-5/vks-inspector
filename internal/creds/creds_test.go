package creds_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modern-apps-5/vks-inspector/internal/creds"
)

// The single most important property in this package: a credential must not
// leak through a format verb. Stray %v in a log line or an error string is how
// passwords end up in support bundles.
func TestCredentialRedactsUnderEveryFormatVerb(t *testing.T) {
	t.Parallel()

	c := creds.Credential{Username: "readonly@vsphere.local", Password: "hunter2", Token: "abcd"}

	for _, verb := range []string{"%v", "%s", "%+v", "%#v", "%q"} {
		got := fmt.Sprintf(verb, c)
		if strings.Contains(got, "hunter2") {
			t.Errorf("password leaked through %s: %s", verb, got)
		}
	}
}

// Serialising a credential into an artifact must fail loudly rather than
// succeed quietly.
func TestCredentialRefusesToMarshal(t *testing.T) {
	t.Parallel()

	if _, err := json.Marshal(creds.Credential{Password: "hunter2"}); err == nil {
		t.Error("credential marshalled to JSON; it must refuse")
	}
}

func TestEnvironmentOverridesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "creds.yaml")
	writeCredsFile(t, path, `apiVersion: vksinspect/v1alpha1
kind: Credentials
credentials:
  vcenter:
    username: from-file
    password: from-file-pw
`)

	t.Setenv(creds.EnvPrefix+"VCENTER_PASSWORD", "from-env-pw")

	set, err := creds.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := set.Get("vcenter")
	if !ok {
		t.Fatal("vcenter credential not found")
	}
	if got.Username != "from-file" {
		t.Errorf("username = %q, want from-file", got.Username)
	}
	if got.Password != "from-env-pw" {
		t.Error("environment did not override the file")
	}
}

// A world-readable credentials file on a shared jump host is a real finding.
// Refusing beats warning: a warning scrolls past.
func TestLooseFilePermissionsAreRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "creds.yaml")
	if err := os.WriteFile(path, []byte("credentials: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := creds.Load(path); err == nil {
		t.Error("expected a 0644 credentials file to be refused")
	} else if !strings.Contains(err.Error(), "0600") {
		t.Errorf("error should tell the user how to fix it, got: %v", err)
	}
}

// Absent credentials are a capability question, not a startup failure — checks
// report them as skips.
func TestAbsentCredentialsAreNotAnError(t *testing.T) {
	t.Parallel()

	set, err := creds.Load("")
	if err != nil {
		t.Fatalf("loading with no file should succeed: %v", err)
	}
	if set.Has("vcenter") {
		t.Error("unexpected credential")
	}
	var nilSet *creds.Set
	if nilSet.Has("vcenter") {
		t.Error("nil Set must be safe to query")
	}
	if nilSet.Keys() != nil {
		t.Error("nil Set must return no keys")
	}
}

func writeCredsFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
