package renderers_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modern-apps-5/vks-inspector/internal/renderers"
	"github.com/modern-apps-5/vks-inspector/internal/results"
)

// update regenerates the golden files: `make golden`.
//
// The golden files ARE the output contract. When one of them changes, the diff
// is the review — an unexplained change to the terminal output is exactly as
// much of a regression as an unexplained change to the JSON, because a field
// engineer reads the terminal output under time pressure and a CI job parses
// the JSON without reading anything.
var update = flag.Bool("update", false, "regenerate golden files")

// fixtureReport builds a deterministic report covering every status and
// severity combination a renderer has to handle.
//
// Determinism is not incidental: fixed timestamps, no hostname lookup, no
// colour. A golden test that depends on the clock or the machine is a flaky
// test that will be deleted within a month.
func fixtureReport() *results.Report {
	at := func(s int) time.Time {
		return time.Date(2026, 3, 14, 9, 30, s, 0, time.UTC)
	}

	res := []results.Result{
		{
			CheckID:        "dns.forward",
			RequirementIDs: []string{"COM-DNS-001"},
			Title:          "Forward DNS resolves for every declared endpoint",
			Category:       results.CategoryDNS,
			Severity:       results.SeverityBlocker,
			Status:         results.StatusPass,
			Mode:           "preflight",
			Target:         "vcenter.lab.example.com",
			Expected:       results.Value{Summary: "resolves to 192.0.2.10", Data: map[string]any{"addresses": []string{"192.0.2.10"}}},
			Observed:       results.Value{Summary: "resolved to 192.0.2.10", Data: map[string]any{"addresses": []string{"192.0.2.10"}}},
			Remediation:    "Add an A record for the endpoint on every declared resolver.",
			Evidence:       map[string]any{"resolver": "192.0.2.53", "rtt_ms": 3},
			StartedAt:      at(0), FinishedAt: at(0), DurationMS: 3,
		},
		{
			CheckID:        "cidr.overlap",
			RequirementIDs: []string{"COM-CID-001", "COM-CID-002"},
			Title:          "No declared range overlaps another",
			Category:       results.CategoryCIDR,
			Severity:       results.SeverityBlocker,
			Status:         results.StatusFail,
			Mode:           "preflight",
			Target:         "kubernetes.podCIDRs[0] vs networks.workload[0]",
			Expected:       results.Value{Summary: "10.244.0.0/20 and 10.20.0.0/16 are disjoint"},
			Observed:       results.Value{Summary: "10.244.0.0/20 overlaps 10.244.0.0/16", Data: map[string]any{"overlap": "10.244.0.0/20"}},
			Remediation:    "Re-plan the pod CIDR so it does not intersect the workload network. Overlapping ranges will route unpredictably and cannot be fixed after deployment without rebuilding the Supervisor.",
			StartedAt:      at(1), FinishedAt: at(1), DurationMS: 1,
		},
		{
			CheckID:        "ntp.skew",
			RequirementIDs: []string{"COM-NTP-002"},
			Title:          "Clock offset is within policy",
			Category:       results.CategoryNTP,
			Severity:       results.SeverityWarning,
			Status:         results.StatusFail,
			Mode:           "preflight",
			Target:         "192.0.2.123",
			Expected:       results.Value{Summary: "offset within 30s", Data: map[string]any{"max_skew_seconds": 30}},
			Observed:       results.Value{Summary: "offset 47s", Data: map[string]any{"offset_seconds": 47}},
			Remediation:    "Point the host and the declared NTP sources at the same stratum.",
			StartedAt:      at(2), FinishedAt: at(2), DurationMS: 120,
		},
		{
			CheckID:        "tcp.port-open",
			RequirementIDs: []string{"COM-FW-001"},
			Title:          "Declared management port is reachable",
			Category:       results.CategoryFirewall,
			Severity:       results.SeverityBlocker,
			Status:         results.StatusUnknown,
			Mode:           "preflight",
			Target:         "nsx.lab.example.com:443",
			Expected:       results.Value{Summary: "TCP 443 accepts a connection"},
			Observed:       results.Value{Summary: "no response — filtered, not refused", Data: map[string]any{"state": "filtered"}},
			Remediation:    "A silent drop is a firewall, not a dead service. Confirm the path from this vantage point before treating it as a failure.",
			StartedAt:      at(3), FinishedAt: at(3), DurationMS: 5000,
		},
		{
			CheckID:        "nsx.tier0-uplink",
			RequirementIDs: []string{"NSX-T0-002"},
			Title:          "Tier-0 has an up uplink with north-south reachability",
			Category:       results.CategoryRouting,
			Severity:       results.SeverityBlocker,
			Status:         results.StatusSkip,
			Mode:           "preflight",
			Expected:       results.Value{Summary: "Tier-0 has an up uplink with north-south reachability"},
			Observed:       results.Value{Summary: "requires nsx credentials", Data: map[string]any{"skip_reason": "requires nsx credentials"}},
			StartedAt:      at(4), FinishedAt: at(4),
		},
		{
			CheckID:        "mtu.path",
			RequirementIDs: []string{"COM-MTU-001"},
			Title:          "Path MTU meets the overlay minimum",
			Category:       results.CategoryMTU,
			Severity:       results.SeverityBlocker,
			Status:         results.StatusSkip,
			Mode:           "preflight",
			Invasive:       true,
			Expected:       results.Value{Summary: "path MTU >= 1600"},
			Observed:       results.Value{Summary: "requires --invasive", Data: map[string]any{"skip_reason": "requires --invasive"}},
			StartedAt:      at(5), FinishedAt: at(5),
		},
		{
			CheckID:        "vc.api-reachable",
			RequirementIDs: []string{"COM-API-001"},
			Title:          "vCenter API answers and credentials authenticate",
			Category:       results.CategoryReachability,
			Severity:       results.SeverityBlocker,
			Status:         results.StatusError,
			Mode:           "preflight",
			Err:            "check returned no results",
			Expected:       results.Value{Summary: "API responds to /about"},
			Observed:       results.Value{Summary: "check did not complete"},
			StartedAt:      at(6), FinishedAt: at(6),
		},
	}

	return results.NewReport(
		results.KindReport,
		results.ToolInfo{Name: "vksinspect", Version: "0.1.0-test", Commit: "deadbee"},
		results.RunInfo{
			Mode:         "preflight",
			Topology:     "nsx",
			ConfigDigest: "0000000000000000000000000000000000000000000000000000000000000000",
			Vantage:      "jumphost.lab.example.com",
			Invasive:     false,
			StartedAt:    at(0),
			FinishedAt:   at(7),
			DurationMS:   7000,
		},
		res,
	)
}

// This is the reference GOLDEN TEST for the repo. Renderer output is compared
// byte for byte against a committed file.
func TestRenderersGolden(t *testing.T) {
	rep := fixtureReport()

	cases := []struct {
		name   string
		format string
		opts   renderers.Options
		golden string
	}{
		{
			name:   "terminal default hides skips",
			format: "terminal",
			opts:   renderers.Options{Colour: false},
			golden: "terminal.txt",
		},
		{
			name:   "terminal verbose shows evidence and skips",
			format: "terminal",
			opts:   renderers.Options{Colour: false, Verbose: true, ShowSkipped: true},
			golden: "terminal-verbose.txt",
		},
		{
			name:   "json always includes skips",
			format: "json",
			opts:   renderers.Options{ShowSkipped: false},
			golden: "report.json",
		},
		{
			name:   "junit",
			format: "junit",
			opts:   renderers.Options{},
			golden: "report.xml",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := renderers.New(tc.format, tc.opts)
			if err != nil {
				t.Fatalf("New(%q): %v", tc.format, err)
			}

			var buf bytes.Buffer
			if err := r.Render(&buf, rep); err != nil {
				t.Fatalf("Render: %v", err)
			}

			path := filepath.Join("testdata", "golden", tc.golden)
			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
				t.Logf("updated %s", path)
				return
			}

			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden %s: %v (run `make golden` to create it)", path, err)
			}
			if !bytes.Equal(want, buf.Bytes()) {
				t.Errorf("output does not match %s\n--- want ---\n%s\n--- got ---\n%s",
					path, want, buf.Bytes())
			}
		})
	}
}

// Renderers must be pure: same report in, same bytes out, every time.
// Without this, golden tests pass locally and fail in CI for reasons nobody
// can reproduce.
func TestRenderersAreDeterministic(t *testing.T) {
	rep := fixtureReport()
	for _, format := range renderers.Formats() {
		t.Run(format, func(t *testing.T) {
			r, err := renderers.New(format, renderers.Options{Verbose: true, ShowSkipped: true})
			if err != nil {
				t.Fatal(err)
			}
			var first bytes.Buffer
			if err := r.Render(&first, rep); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 20; i++ {
				var next bytes.Buffer
				if err := r.Render(&next, rep); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(first.Bytes(), next.Bytes()) {
					t.Fatalf("render %d differs from render 0 — output is not deterministic", i+1)
				}
			}
		})
	}
}

// The JSON renderer must never drop a skipped result, whatever the options say.
// A machine consumer that cannot tell "passed" from "never ran" will report a
// half-inspected environment as healthy.
func TestJSONNeverHidesSkips(t *testing.T) {
	rep := fixtureReport()
	r, err := renderers.New("json", renderers.Options{ShowSkipped: false})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := r.Render(&buf, rep); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"status": "skip"`)) {
		t.Error("JSON output omitted skipped results; a JSON consumer cannot distinguish pass from never-ran")
	}
}

func TestUnknownFormatIsAnError(t *testing.T) {
	if _, err := renderers.New("yaml", renderers.Options{}); err == nil {
		t.Error("expected an error for an unknown format")
	}
}
