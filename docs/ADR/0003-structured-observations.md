# ADR-0003 — Observations are structured values, not prose

**Status:** Accepted · **Date:** 2026-07-29

## Context

Drift detection has to answer "what changed since the baseline" mechanically.
That is impossible if a check's output is a human sentence: `"resolved to
192.0.2.10 in 3ms"` and `"resolved to 192.0.2.10 in 4ms"` are the same
observation and different strings.

The temptation is to add structure later, when drift is built. That does not
work — by then there are dozens of checks emitting prose, and retrofitting means
touching all of them and re-capturing every baseline.

## Decision

`Result.Expected` and `Result.Observed` are `results.Value`:

```go
type Value struct {
    Summary string         // for humans, rendered
    Data    map[string]any // for machines, diffed
}
```

Drift compares `Data`, never `Summary`. `Data` must contain JSON-safe scalars,
slices and maps — no timing jitter, no formatted strings that embed varying
numbers, no secrets.

`results.Text()` exists for values with no machine payload, and is documented as
something to use sparingly: a `Value` with no `Data` is invisible to drift.

## Consequences

- Every result is diffable from the first check written, not from phase 4.
- The JSON output is genuinely machine-consumable, which is what makes the
  future UI a plain consumer rather than a special case (ADR-0004).
- **Cost:** every check author writes the observation twice, once for humans and
  once for machines. This is real friction and it is worth it.
- **Cost:** `map[string]any` is not type-safe. A check can put a `time.Duration`
  in `Data` and produce something that serialises inconsistently. Only convention
  and review prevent this. A typed value union was considered and rejected as
  disproportionate for phase 1 — revisit if it bites.
- **Discipline required:** `Data` must not contain values that change every run
  for reasons nobody cares about. An RTT in `Data` makes every drift run report
  a change. RTTs belong in `Evidence`.
