package engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/modern-apps-5/vks-inspector/internal/checks"
	"github.com/modern-apps-5/vks-inspector/internal/checks/all"
	"github.com/modern-apps-5/vks-inspector/internal/config"
	"github.com/modern-apps-5/vks-inspector/internal/engine"
	"github.com/modern-apps-5/vks-inspector/internal/probes"
	"github.com/modern-apps-5/vks-inspector/internal/registry"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

func testConfig() *config.Config {
	return &config.Config{
		APIVersion: config.APIVersion,
		Kind:       config.Kind,
		Metadata:   config.Metadata{Name: "engine-test"},
		Topology:   config.Topology{Networking: config.NetNSX, LoadBalancer: config.LBNSX},
		NSX:        &config.NSX{Tier0Gateway: "T0"},
	}
}

func fixedNow() func() time.Time {
	t := time.Date(2026, 3, 14, 9, 30, 0, 0, time.UTC)
	return func() time.Time { return t }
}

// End-to-end proof of the skeleton: registry → selection → run → report →
// exit code. If this breaks, the shape is broken, not a check.
func TestRunProducesAGradedReport(t *testing.T) {
	t.Parallel()

	rep, err := engine.Run(context.Background(), all.Registry(), engine.Options{
		Mode:   checks.ModePreflight,
		Config: testConfig(),
		Probes: &probes.Fake{},
		Now:    fixedNow(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if rep.SchemaVersion != results.SchemaVersion {
		t.Errorf("schema version = %d, want %d", rep.SchemaVersion, results.SchemaVersion)
	}
	if rep.Run.Mode != string(checks.ModePreflight) {
		t.Errorf("mode = %q", rep.Run.Mode)
	}
	if rep.Run.ConfigDigest == "" {
		t.Error("no config digest; drift cannot tell an intent change from an environment change")
	}
	if rep.Summary.Total == 0 {
		t.Fatal("report contains no results")
	}
	if got := results.ExitCode(rep.Results); got != results.ExitPass {
		t.Errorf("exit code = %d (%s), want 0 — the reference check should pass on a valid config",
			got, results.ExitCodeText(got))
	}
}

// Every mode must run through the same engine path and produce the same shape.
func TestEveryModeRuns(t *testing.T) {
	t.Parallel()
	for _, mode := range checks.AllModes {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			rep, err := engine.Run(context.Background(), all.Registry(), engine.Options{
				Mode: mode, Config: testConfig(), Probes: &probes.Fake{}, Now: fixedNow(),
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if rep.Summary.Total == 0 {
				t.Errorf("mode %s produced no results", mode)
			}
			for _, r := range rep.Results {
				if r.Mode != string(mode) {
					t.Errorf("result %s recorded mode %q, want %q", r.CheckID, r.Mode, mode)
				}
			}
		})
	}
}

// A check excluded by selection must appear as a reported skip, not vanish.
// A report that silently omits checks reads as more coverage than it has.
func TestExcludedChecksAreReportedAsSkips(t *testing.T) {
	t.Parallel()

	reg := all.Registry()
	total := reg.Len()

	rep, err := engine.Run(context.Background(), reg, engine.Options{
		Mode:   checks.ModePreflight,
		Config: testConfig(),
		Probes: &probes.Fake{},
		Now:    fixedNow(),
		Skip:   []string{"meta.topology-recognised"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if rep.Summary.Total != total {
		t.Errorf("report accounts for %d checks, registry has %d — a check went missing",
			rep.Summary.Total, total)
	}
	if rep.Summary.Skip == 0 {
		t.Error("skipped check was not reported as a skip")
	}
	for _, r := range rep.Results {
		if r.Status == results.StatusSkip && r.Observed.Summary == "" {
			t.Errorf("check %s was skipped with no reason given", r.CheckID)
		}
	}
}

// A check that panics must become a tool error (exit 3), never an environment
// verdict (exit 1), and must not take the rest of the report with it.
func TestPanickingCheckBecomesAToolError(t *testing.T) {
	t.Parallel()

	reg := registry.New()
	reg.Register(&panicCheck{})

	rep, err := engine.Run(context.Background(), reg, engine.Options{
		Mode: checks.ModePreflight, Config: testConfig(), Probes: &probes.Fake{}, Now: fixedNow(),
	})
	if err != nil {
		t.Fatalf("engine returned an error instead of a result: %v", err)
	}
	if rep.Summary.Errors != 1 {
		t.Fatalf("errors = %d, want 1", rep.Summary.Errors)
	}
	if got := results.ExitCode(rep.Results); got != results.ExitToolError {
		t.Errorf("exit code = %d, want %d (tool error)", got, results.ExitToolError)
	}
}

// A check returning no results has silently dropped a requirement. Surface it.
func TestSilentCheckBecomesAToolError(t *testing.T) {
	t.Parallel()

	reg := registry.New()
	reg.Register(&silentCheck{})

	rep, err := engine.Run(context.Background(), reg, engine.Options{
		Mode: checks.ModePreflight, Config: testConfig(), Probes: &probes.Fake{}, Now: fixedNow(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Summary.Errors != 1 {
		t.Errorf("a check returning no results was not reported: %+v", rep.Summary)
	}
}

// Policy severity overrides must be applied, and must be visible in the result
// so nobody has to guess why a blocker reported as a warning.
func TestSeverityOverrideIsAppliedAndRecorded(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.Policy.SeverityOverrides = map[string]string{
		"meta.topology-recognised": string(results.SeverityInfo),
	}

	rep, err := engine.Run(context.Background(), all.Registry(), engine.Options{
		Mode: checks.ModePreflight, Config: cfg, Probes: &probes.Fake{}, Now: fixedNow(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rep.Results {
		if r.CheckID != "meta.topology-recognised" {
			continue
		}
		if r.Severity != results.SeverityInfo {
			t.Errorf("severity = %s, want info", r.Severity)
		}
		if r.Evidence["severity_overridden_by"] == nil {
			t.Error("override was applied but not recorded in the result")
		}
		return
	}
	t.Fatal("reference check not present in report")
}

type panicCheck struct{}

func (panicCheck) Meta() checks.Meta {
	return checks.Meta{
		ID: "test.panic", Title: "panics on purpose",
		RequirementIDs: []string{"TEST-001"},
		Modes:          checks.AllModes,
		Severity:       results.SeverityBlocker,
	}
}
func (panicCheck) Run(context.Context, *checks.RunContext) ([]results.Result, error) {
	panic("boom")
}

type silentCheck struct{}

func (silentCheck) Meta() checks.Meta {
	return checks.Meta{
		ID: "test.silent", Title: "returns nothing",
		RequirementIDs: []string{"TEST-002"},
		Modes:          checks.AllModes,
		Severity:       results.SeverityBlocker,
	}
}
func (silentCheck) Run(context.Context, *checks.RunContext) ([]results.Result, error) {
	return nil, nil
}
