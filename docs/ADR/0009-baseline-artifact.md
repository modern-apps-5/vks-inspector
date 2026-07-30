# ADR-0009 — A baseline is a Report, not a second format

**Status:** Accepted · **Date:** 2026-07-29

## Context

`snapshot` captures state; `drift` compares against it. The natural design is a
purpose-built baseline format holding "the state" — and it is a trap. A separate
format drifts from the report format, and then `drift` is comparing something
subtly different from what `check` produces, and the two disagree in ways nobody
can explain.

## Decision

**A baseline is a `results.Report` with `Kind: "vksinspect.baseline/v1"`.** One
artifact type. `snapshot` runs the same engine in `ModeSnapshot` and writes the
report; `drift` loads two reports and diffs them.

Supporting decisions:

- **Deterministic ordering.** `WriteBaseline` sorts results by
  `CheckID` + `Target` before writing, so two runs of the same environment
  produce byte-comparable files. Unstable ordering makes every diff noisy and
  drift useless.
- **Schema versioning.** `SchemaVersion` is checked on read; a baseline from a
  different schema is refused with an instruction to re-capture, not
  best-effort parsed.
- **Config digest.** `Run.ConfigDigest` is a SHA-256 over the normalised config.
  Drift must distinguish "the environment changed" from "the declared intent
  changed" — without it, a config edit looks identical to an infrastructure
  regression. The digest is over normalised JSON, so comments and key order do
  not affect it.
- **Vantage recorded.** `Run.Vantage` names the host the probes ran from. "Port
  443 was reachable" is meaningless a year later without knowing who asked.
- **Mode recorded per result**, so a verify-mode baseline is never silently
  compared against preflight assertions.

## Consequences

- The baseline format already exists and is already exercised in phase 1 —
  every `--format json` run produces the shape `snapshot` will write.
- Any consumer that reads a report reads a baseline.
- **Cost:** baselines carry fields irrelevant to drift (remediation text,
  evidence). Larger files, in exchange for a self-describing artifact that is
  interpretable without the tool that made it. Worth it.
- **Unresolved, recorded in `cmd/vksinspect/snapshot.go`:** a baseline captured
  *without* credentials is a different kind of baseline from one captured with
  them, and must be labelled, or drift will report every credentialed check as
  "newly appeared". Likewise whether snapshot should refuse to write a partial
  baseline when checks errored.
- **Untested today:** `WriteBaseline`'s ordering guarantee is load-bearing and
  has no round-trip test. Listed as a known gap in
  [unit-test-coverage.md](../unit-test-coverage.md).
