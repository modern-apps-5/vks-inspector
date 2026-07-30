// Package docs_test keeps docs/REQUIREMENTS-MATRIX.md honest about what this
// build actually does.
//
// Two things are enforced here, both of which were previously listed as debt in
// docs/CONTRIBUTING.md:
//
//  1. Every requirement ID cited by a check exists in the matrix. The registry
//     rejects an *empty* RequirementIDs list but cannot detect an invented ID,
//     so a check citing COM-DNS-999 would ship unnoticed.
//  2. The per-section summary tables in the matrix match the registry. The
//     tables are generated, not authored — a hand-maintained coverage table
//     drifts from the code within one release, which is the whole reason
//     coverage lived in a separate file before and rotted there too.
//
// Regenerate after adding or retargeting a check:
//
//	make matrix     # or: go test ./internal/docs/... -update
//
// Review the diff. It is the coverage claim this project makes.
package docs_test

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/modern-apps-5/vks-inspector/internal/checks/all"
)

var update = flag.Bool("update", false, "rewrite the summary tables in the matrix")

const (
	matrixPath = "../../docs/REQUIREMENTS-MATRIX.md"
	beginMark  = "<!-- BEGIN GENERATED SUMMARY -->"
	endMark    = "<!-- END GENERATED SUMMARY -->"
)

// blockedBy explains why an *unflagged* row has no check yet. It is editorial:
// it cannot be derived from the code, because "this needs an NSX client" is a
// judgement about work not done rather than a fact about work done.
//
// Rows absent from this map fall back to a flag-derived status, so forgetting
// an entry degrades to "—" rather than to a wrong claim. Flagged rows are not
// listed here at all — they are blocked on confirming the requirement, which
// outranks any implementation concern.
var blockedBy = map[string]string{
	// Settled requirement, nothing new needed — only the work.
	"COM-MTU-002": "ready",
	"LB-FLB-002":  "ready",
	"LB-VIP-002":  "ready",
	"LB-HAP-001":  "ready",
	"LB-HAP-004":  "ready",

	// Buildable, but only meaningful from a specific segment. Writing these as
	// ordinary local probes would produce exactly the false green that
	// CHECK-TAXONOMY.md calls this tool's likeliest failure mode.
	"COM-DNS-003": "vantage",
	"COM-RTE-002": "vantage",
	"SUP-MGT-002": "vantage",
	"VDS-WKL-002": "vantage",
	"LB-ALB-008":  "vantage",
	"LB-VIP-005":  "vantage",

	// Blocked on a probe capability that does not exist.
	"COM-RTE-001": "raw socket",
	"COM-MTU-005": "invasive probe",

	// Blocked on a management-plane client that does not exist.
	"COM-API-002": "NSX client",
	"NSX-T0-001":  "NSX client",
	"NSX-T0-002":  "NSX client",
	"NSX-TZ-001":  "NSX client",
	"NSX-TZ-002":  "NSX client",
	"NSX-ING-001": "NSX client",
	"NSX-EGR-001": "NSX client",
	"LB-ALB-001":  "ALB client",
	"LB-ALB-003":  "ALB client",
	"LB-ALB-007":  "ALB client",
	"LB-VIP-003":  "ALB client",
	"LB-VIP-004":  "ALB client",
	"LB-HAP-002":  "HAProxy API",
	"LB-HAP-003":  "HAProxy API",
}

// Two ID shapes exist in the matrix: three-segment (COM-DNS-001) and
// two-segment (MET-001, NSX-T0-001). A pattern assuming three silently drops
// four rows and misreports coverage.
var (
	rowRe      = regexp.MustCompile("^#### `([A-Z][A-Z0-9-]*)`\\s*·\\s*(.*)$")
	severityRe = regexp.MustCompile(`\*\*Severity\*\*\s+([a-z]+)`)
)

type row struct {
	id, title, severity string
	flagged             bool
	line                int
}

type section struct {
	rows     []row
	firstRow int
}

func parseMatrix(t *testing.T) (lines []string, secs []*section) {
	t.Helper()
	blob, err := os.ReadFile(matrixPath)
	if err != nil {
		t.Fatalf("read matrix: %v", err)
	}
	all := strings.Split(string(blob), "\n")

	// Drop any previously generated block so parsing sees only authored prose.
	//
	// The blank line that renderTable emits *after* the end marker has to be
	// swallowed too. It sits outside the marked region, so leaving it would
	// accumulate one blank line per section on every regeneration and the
	// generator would never reach a fixed point.
	skipping, justEnded := false, false
	for _, ln := range all {
		switch strings.TrimSpace(ln) {
		case beginMark:
			skipping, justEnded = true, false
			continue
		case endMark:
			skipping, justEnded = false, true
			continue
		}
		if justEnded {
			justEnded = false
			if strings.TrimSpace(ln) == "" {
				continue
			}
		}
		if !skipping {
			lines = append(lines, ln)
		}
	}

	var cur *section
	for i, ln := range lines {
		if strings.HasPrefix(ln, "# ") && !strings.HasPrefix(ln, "## ") {
			cur = &section{firstRow: -1}
			secs = append(secs, cur)
		}
		m := rowRe.FindStringSubmatch(ln)
		if m == nil || cur == nil {
			continue
		}
		meta := ""
		if i+1 < len(lines) {
			meta = lines[i+1]
		}
		sev := ""
		if s := severityRe.FindStringSubmatch(meta); s != nil {
			sev = s[1]
		}
		if cur.firstRow < 0 {
			cur.firstRow = i
		}
		cur.rows = append(cur.rows, row{
			id:       m[1],
			title:    strings.TrimSpace(m[2]),
			severity: sev,
			flagged:  strings.Contains(meta, "**Flag** ⚑"),
			line:     i,
		})
	}
	return lines, secs
}

// rowToChecks maps requirement ID -> check IDs, read from the registry rather
// than from source text so it cannot go stale relative to what actually runs.
func rowToChecks() map[string][]string {
	out := map[string][]string{}
	for _, c := range all.Registry().All() {
		m := c.Meta()
		for _, rid := range m.RequirementIDs {
			out[rid] = append(out[rid], m.ID)
		}
	}
	for k := range out {
		sort.Strings(out[k])
		out[k] = dedupe(out[k])
	}
	return out
}

func dedupe(in []string) []string {
	var out []string
	for i, v := range in {
		if i == 0 || v != in[i-1] {
			out = append(out, v)
		}
	}
	return out
}

func status(r row, covered map[string][]string) string {
	if ids, ok := covered[r.id]; ok {
		quoted := make([]string, len(ids))
		for i, id := range ids {
			quoted[i] = "`" + id + "`"
		}
		return "✅ " + strings.Join(quoted, ", ")
	}
	if r.flagged {
		// Blocked on confirming the requirement, which outranks any
		// implementation concern.
		return "confirm first"
	}
	if b, ok := blockedBy[r.id]; ok {
		return b
	}
	return "—"
}

func renderTable(s *section, covered map[string][]string) []string {
	out := []string{
		beginMark, "",
		"| ID | Requirement | Severity | ⚑ | Status |",
		"|---|---|---|---|---|",
	}
	done := 0
	for _, r := range s.rows {
		flag := ""
		if r.flagged {
			flag = "⚑"
		}
		if _, ok := covered[r.id]; ok {
			done++
		}
		out = append(out, fmt.Sprintf("| `%s` | %s | %s | %s | %s |",
			r.id, r.title, r.severity, flag, status(r, covered)))
	}
	out = append(out, "", fmt.Sprintf("*%d of %d implemented.*", done, len(s.rows)), endMark, "")
	return out
}

// A check citing a requirement ID that does not exist is a claim traceable to
// nothing. The registry cannot catch it; this does.
func TestEveryCitedRequirementExists(t *testing.T) {
	_, secs := parseMatrix(t)
	known := map[string]bool{}
	for _, s := range secs {
		for _, r := range s.rows {
			known[r.id] = true
		}
	}
	for _, c := range all.Registry().All() {
		m := c.Meta()
		for _, rid := range m.RequirementIDs {
			if !known[rid] {
				t.Errorf("check %q cites %q, which is not a row in the matrix", m.ID, rid)
			}
		}
	}
}

// The summary tables are generated. This regenerates them under -update and
// otherwise fails if they have drifted from the registry.
func TestSummaryTablesAreCurrent(t *testing.T) {
	lines, secs := parseMatrix(t)
	covered := rowToChecks()

	// Insert bottom-up so earlier indices stay valid.
	out := append([]string(nil), lines...)
	for i := len(secs) - 1; i >= 0; i-- {
		s := secs[i]
		if len(s.rows) == 0 {
			continue
		}
		// Absorb any blank lines already sitting above the first row, then emit
		// exactly one. Without this the spacing depends on what a previous run
		// left behind, and blank lines creep in a section at a time.
		at := s.firstRow
		for at > 0 && strings.TrimSpace(out[at-1]) == "" {
			at--
		}
		tbl := append([]string{""}, renderTable(s, covered)...)

		// Tail resumes at the row itself, not at `at` — the blank lines between
		// them were absorbed and must not survive, or they accumulate.
		merged := make([]string, 0, len(out)+len(tbl))
		merged = append(merged, out[:at]...)
		merged = append(merged, tbl...)
		merged = append(merged, out[s.firstRow:]...)
		out = merged
	}
	want := strings.Join(out, "\n")

	current, err := os.ReadFile(matrixPath)
	if err != nil {
		t.Fatalf("read matrix: %v", err)
	}
	if string(current) == want {
		return
	}
	if *update {
		if err := os.WriteFile(matrixPath, []byte(want), 0o644); err != nil {
			t.Fatalf("write matrix: %v", err)
		}
		t.Log("summary tables regenerated; review the diff before committing")
		return
	}
	t.Errorf("matrix summary tables are stale. Regenerate with:\n\n    make matrix\n")
}
