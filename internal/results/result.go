// Package results defines the structured output of every check.
//
// Nothing in this package prints. Checks build Results; renderers consume them.
// The shapes here are the tool's contract: they are what the JSON renderer
// emits, what `snapshot` writes as a baseline, and what a future `drift` run
// diffs. Treat a change to any JSON tag as breaking and bump SchemaVersion.
//
// The reason Observed and Expected are structured Values rather than strings is
// drift: a later run has to be able to say "this changed" mechanically, and you
// cannot diff a human sentence. See docs/ADR/0003-structured-observations.md.
package results

import "time"

// SchemaVersion is the version of the Report/Result wire format. Bump on any
// breaking change to the JSON shape; drift refuses to compare across versions.
const SchemaVersion = 1

// Kind identifies an artifact so a file can be validated before it is trusted.
const (
	KindReport   = "vksinspect.report/v1"
	KindBaseline = "vksinspect.baseline/v1"
)

// Status is what the check observed. It deliberately does NOT encode how much
// the observation matters — that is Severity's job. A failed info-severity
// check and a failed blocker-severity check are both StatusFail.
type Status string

const (
	// StatusPass — the expected condition holds.
	StatusPass Status = "pass"
	// StatusFail — the expected condition demonstrably does not hold.
	StatusFail Status = "fail"
	// StatusSkip — the check does not apply here (wrong topology, missing
	// credentials, needs --invasive). Not a problem, not evidence of health.
	StatusSkip Status = "skip"
	// StatusUnknown — the check ran but could not determine an answer, e.g. a
	// filtered port that neither accepts nor rejects. Distinct from Fail
	// because we are not entitled to assert a failure we did not observe.
	StatusUnknown Status = "unknown"
	// StatusError — the check itself malfunctioned. A tool fault, not an
	// environment fault. Always exit code 3.
	StatusError Status = "error"
)

// Severity is how much a non-pass matters to the deployment.
type Severity string

const (
	// SeverityBlocker — deployment will fail or be unsupported. Exit code 1.
	SeverityBlocker Severity = "blocker"
	// SeverityWarning — deployment may succeed but is degraded, unsupported at
	// scale, or will bite later. Exit code 2.
	SeverityWarning Severity = "warning"
	// SeverityInfo — recorded for the baseline, never gates anything.
	SeverityInfo Severity = "info"
)

// Category groups requirements for reporting and for --only/--skip filtering.
// These values are the same vocabulary used by docs/REQUIREMENTS-MATRIX.md;
// they must stay in sync with it.
type Category string

const (
	CategoryCIDR         Category = "cidr"
	CategoryRouting      Category = "routing"
	CategoryMTU          Category = "mtu"
	CategoryDNS          Category = "dns"
	CategoryNTP          Category = "ntp"
	CategoryFirewall     Category = "firewall"
	CategoryCerts        Category = "certs"
	CategoryReachability Category = "reachability"
	CategoryIPPool       Category = "ippool"
	// CategoryInventory covers "does this named vSphere/NSX/ALB object exist
	// and is it configured the way the config claims" — VDS presence, portgroup
	// VLAN, edge cluster. Not in the original brief's category list; added
	// because a large share of credentialed checks land here and would
	// otherwise be miscategorised as reachability.
	CategoryInventory Category = "inventory"
	// CategoryMeta covers the tool's own self-checks.
	CategoryMeta Category = "meta"
)

// Value is one side of an assertion. Summary is for humans; Data is for
// machines — drift diffs Data, never Summary.
//
// Data must contain only JSON-safe scalars, slices and maps, and must never
// contain a credential. See docs/ADR/0005-credential-handling.md.
type Value struct {
	Summary string         `json:"summary"`
	Data    map[string]any `json:"data,omitempty"`
}

// Text builds a Value with no machine-comparable payload. Use sparingly: a
// Value with no Data is invisible to drift.
func Text(summary string) Value {
	return Value{Summary: summary}
}

// Result is the single output unit of a check. Every field here survives into
// the baseline artifact.
type Result struct {
	// CheckID is the stable identifier of the check that produced this, e.g.
	// "dns.forward-reverse". Stable across releases; renaming one is breaking.
	CheckID string `json:"check_id"`
	// RequirementIDs are the docs/REQUIREMENTS-MATRIX.md rows this check
	// evidences. Many-to-many on purpose: one probe can satisfy several rows,
	// and one row can need several probes.
	RequirementIDs []string `json:"requirement_ids,omitempty"`

	Title    string   `json:"title"`
	Category Category `json:"category"`
	Severity Severity `json:"severity"`
	Status   Status   `json:"status"`

	// Mode records which run mode produced this, so a baseline captured in
	// verify mode is never silently compared against a preflight assertion.
	Mode string `json:"mode"`
	// Target is what was under test — a host, CIDR, portgroup name. Used to
	// key results when one check fans out over many targets.
	Target string `json:"target,omitempty"`

	Expected Value `json:"expected"`
	Observed Value `json:"observed"`

	// Remediation is the operator-facing "what do I do about it". Populated
	// even on pass, so `explain` and the future UI have something to show.
	Remediation string `json:"remediation,omitempty"`

	// Evidence is raw supporting detail (resolved addresses, RTTs, cert
	// fingerprints). Rendered only in verbose/JSON. Never secrets.
	Evidence map[string]any `json:"evidence,omitempty"`

	// Err is set only when Status is StatusError.
	Err string `json:"error,omitempty"`

	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	DurationMS int64     `json:"duration_ms"`

	// Invasive records whether this result required --invasive to produce, so a
	// reader can tell a non-invasive baseline from an invasive one.
	Invasive bool `json:"invasive,omitempty"`
}

// OK reports whether the result should count as healthy.
func (r Result) OK() bool { return r.Status == StatusPass || r.Status == StatusSkip }

// ToolInfo identifies the binary that produced a report, so an old baseline can
// be interpreted correctly.
type ToolInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

// RunInfo records the circumstances of a run. Vantage point matters: "port 443
// was reachable" is meaningless without knowing which host asked.
type RunInfo struct {
	Mode     string `json:"mode"`
	Topology string `json:"topology"`
	// ConfigDigest is a SHA-256 over the normalised config, so drift can tell
	// "the environment changed" from "the declared intent changed".
	ConfigDigest string `json:"config_digest,omitempty"`
	// Vantage is the host the probes ran from.
	Vantage    string    `json:"vantage,omitempty"`
	Invasive   bool      `json:"invasive"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	DurationMS int64     `json:"duration_ms"`
}

// Summary is the roll-up. Renderers display it; the exit code is derived from
// the results themselves, not from here.
type Summary struct {
	Total    int `json:"total"`
	Pass     int `json:"pass"`
	Fail     int `json:"fail"`
	Skip     int `json:"skip"`
	Unknown  int `json:"unknown"`
	Errors   int `json:"errors"`
	Blockers int `json:"blockers"`
	Warnings int `json:"warnings"`
}

// Report is the complete output of one run. It is also, unchanged, the baseline
// artifact format: `snapshot` writes a Report with Kind=KindBaseline and
// `drift` diffs two of them. One artifact type, not two.
// See docs/ADR/0009-baseline-artifact.md.
type Report struct {
	SchemaVersion int      `json:"schema_version"`
	Kind          string   `json:"kind"`
	Tool          ToolInfo `json:"tool"`
	Run           RunInfo  `json:"run"`
	Results       []Result `json:"results"`
	Summary       Summary  `json:"summary"`
}

// NewReport returns a Report with the schema fields set.
func NewReport(kind string, tool ToolInfo, run RunInfo, res []Result) *Report {
	rep := &Report{
		SchemaVersion: SchemaVersion,
		Kind:          kind,
		Tool:          tool,
		Run:           run,
		Results:       res,
	}
	rep.Summary = Summarise(res)
	return rep
}

// Summarise rolls up a result set.
func Summarise(res []Result) Summary {
	var s Summary
	s.Total = len(res)
	for _, r := range res {
		switch r.Status {
		case StatusPass:
			s.Pass++
		case StatusFail:
			s.Fail++
		case StatusSkip:
			s.Skip++
		case StatusUnknown:
			s.Unknown++
		case StatusError:
			s.Errors++
		}
		if r.Status == StatusFail {
			switch r.Severity {
			case SeverityBlocker:
				s.Blockers++
			case SeverityWarning:
				s.Warnings++
			}
		}
	}
	return s
}
