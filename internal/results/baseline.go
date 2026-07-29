package results

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// A baseline is just a Report with Kind == KindBaseline. There is deliberately
// no second struct: if snapshot wrote a different shape from check, the two
// would drift apart and drift-detection would be comparing apples to oranges.
// See docs/ADR/0009-baseline-artifact.md.

// WriteBaseline serialises a report as a baseline artifact.
//
// Results are sorted by CheckID+Target first so two runs of the same
// environment produce byte-comparable files. Unstable ordering would make every
// diff noisy and drift detection useless.
func WriteBaseline(w io.Writer, rep *Report) error {
	b := *rep
	b.Kind = KindBaseline
	b.Results = append([]Result(nil), rep.Results...)
	sort.SliceStable(b.Results, func(i, j int) bool {
		if b.Results[i].CheckID != b.Results[j].CheckID {
			return b.Results[i].CheckID < b.Results[j].CheckID
		}
		return b.Results[i].Target < b.Results[j].Target
	})

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(&b)
}

// ReadBaseline loads and validates a baseline artifact.
func ReadBaseline(r io.Reader) (*Report, error) {
	var rep Report
	if err := json.NewDecoder(r).Decode(&rep); err != nil {
		return nil, fmt.Errorf("decode baseline: %w", err)
	}
	if rep.Kind != KindBaseline && rep.Kind != KindReport {
		return nil, fmt.Errorf("not a vksinspect artifact: kind=%q", rep.Kind)
	}
	if rep.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf(
			"baseline schema version %d is not readable by this build (expects %d); re-capture the baseline",
			rep.SchemaVersion, SchemaVersion)
	}
	return &rep, nil
}

// ChangeKind classifies one entry in a drift diff.
type ChangeKind string

const (
	ChangeAdded    ChangeKind = "added"    // present now, absent in baseline
	ChangeRemoved  ChangeKind = "removed"  // present in baseline, absent now
	ChangeStatus   ChangeKind = "status"   // status transitioned
	ChangeObserved ChangeKind = "observed" // same status, different observation
)

// Change is one drift finding.
type Change struct {
	Kind     ChangeKind `json:"kind"`
	CheckID  string     `json:"check_id"`
	Target   string     `json:"target,omitempty"`
	Was      *Result    `json:"was,omitempty"`
	Now      *Result    `json:"now,omitempty"`
	Fields   []string   `json:"fields,omitempty"` // Observed.Data keys that differ
	Severity Severity   `json:"severity"`
}

// DiffBaseline compares a current report against a stored baseline.
//
// TODO(phase-4): implement. The signature is fixed now so the drift command and
// the JSON schema can be designed against it, and so nothing in the check layer
// accidentally assumes results are only ever consumed once. Note that a correct
// implementation must compare Observed.Data key-by-key, must treat a change in
// Run.ConfigDigest as "intent changed, not environment changed", and must
// refuse to diff across differing Run.Mode.
func DiffBaseline(baseline, current *Report) ([]Change, error) {
	return nil, fmt.Errorf("drift detection is not implemented in phase 1")
}
