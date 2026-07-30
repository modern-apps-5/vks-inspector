# ADR-0009 — A baseline is just a Report, not a second format

**Status:** Accepted · **Date:** 2026-07-29

## Context

`snapshot` records the current state and `drift` compares against it. The
natural design is a dedicated baseline format holding "the state", and it is a
trap. A separate format falls out of step with the report format. Then `drift`
is comparing something subtly different from what `check` produces, and the two
disagree in ways nobody can explain.

## Decision

**A baseline is a `results.Report` with `Kind: "vksinspect.baseline/v1"`.** One
file format, not two. `snapshot` runs the same engine in `ModeSnapshot` and
writes the report; `drift` loads two reports and compares them.

Supporting decisions:

- **Stable ordering.** `WriteBaseline` sorts results by `CheckID` + `Target`
  before writing, so two runs against the same environment produce files that can
  be compared byte for byte. Unstable ordering makes every comparison noisy and
  drift useless.
- **Versioned format.** `SchemaVersion` is checked on read. A baseline written by
  a different version is refused, with instructions to re-capture, rather than
  parsed as best it can be.
- **Config fingerprint.** `Run.ConfigDigest` is a SHA-256 over the normalised
  config. Drift has to tell "the environment changed" apart from "what we
  declared changed" — without this, editing the config looks exactly like
  infrastructure breaking. The fingerprint is taken over normalised JSON, so
  comments and key order do not affect it.
- **Which host ran it.** `Run.Vantage` names the host the probes ran from. "Port
  443 was reachable" means nothing a year later without knowing who was asking.
- **The mode, per result**, so a baseline captured in verify mode never gets
  quietly compared against preflight claims.

## Consequences

- The baseline format already exists and is already exercised in phase 1: every
  `--format json` run produces the shape `snapshot` will write.
- Anything that can read a report can read a baseline.
- **Cost:** baselines carry fields drift does not need (the fix text, the
  supporting detail). Bigger files, in exchange for one that explains itself and
  can be read without the tool that made it. Worth it.
- **Unresolved, recorded in `cmd/vksinspect/snapshot.go`:** a baseline captured
  *without* credentials is a different thing from one captured with them, and has
  to be labelled as such — otherwise drift reports every credentialed check as
  newly appeared. Same question for whether snapshot should refuse to write a
  partial baseline when checks errored.
- **Untested today:** drift depends on `WriteBaseline` writing in a stable order,
  and there is no round-trip test for it. Listed as a known gap in
  [unit-test-coverage.md](../unit-test-coverage.md).
