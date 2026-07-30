# ADR-0003 — Results carry data, not just sentences

**Status:** Accepted · **Date:** 2026-07-29

## Context

Drift detection has to answer "what changed since the baseline" automatically.
That is impossible if a check's output is a sentence written for a person.
`"resolved to 192.0.2.10 in 3ms"` and `"resolved to 192.0.2.10 in 4ms"` say the
same thing and are different strings.

The tempting move is to add the data later, when drift gets built. That does not
work. By then there are dozens of checks writing sentences, and going back means
touching all of them and re-capturing every baseline.

## Decision

`Result.Expected` and `Result.Observed` are `results.Value`:

```go
type Value struct {
    Summary string         // for humans, rendered
    Data    map[string]any // for machines, diffed
}
```

Drift compares `Data`, never `Summary`. `Data` holds JSON-safe numbers, strings,
lists and maps — no values that wobble between runs, no formatted strings with
changing numbers inside them, no secrets.

`results.Text()` exists for values with no data to attach, and should be used
sparingly: a `Value` with no `Data` is invisible to drift.

## Consequences

- Every result can be compared from the first check written, not from phase 4.
- The JSON output is genuinely usable by other programs, which is what lets the
  future UI just read it like anything else would (ADR-0004).
- **Cost:** every check author writes the observation twice, once for people and
  once as data. That is real friction, and it is worth it.
- **Cost:** `map[string]any` is not type-safe. A check can put a `time.Duration`
  in `Data` and get inconsistent output. Only convention and review prevent it. A
  typed alternative was considered and judged too much for phase 1 — revisit if
  it causes trouble.
- **Care needed:** `Data` must not hold values that change every run for reasons
  nobody cares about. A round-trip time in `Data` makes every drift run report a
  change. Those belong in `Evidence`.
