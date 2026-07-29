# ADR-0002 — Checks are mode-parametric, not preflight-specific

**Status:** Accepted · **Date:** 2026-07-29

## Context

Phase 1 delivers preflight only. Phases 2–4 add post-deploy verification,
baseline snapshot and drift detection. The obvious way to build phase 1 is a
preflight tool; the obvious consequence is that phase 2 becomes a second tool
sharing a name, and every check gets written twice with slightly different
semantics.

The failure mode is specific and predictable: a check written as "does this
config make sense" has no observation to make against a live environment, so
verify mode cannot reuse it, so verify mode grows its own parallel check set,
so the two disagree, so nobody trusts either.

## Decision

A check declares which modes it supports (`Meta.Modes`) and receives the mode at
run time (`RunContext.Mode`). There is **one engine body** — `engine.Run` — and
`check`, `verify` and `snapshot` differ only in the `Mode` they pass. No
preflight-specific code path exists.

All six subcommands are stubbed from day one, before any of them are needed, so
no internal package can grow an assumption that preflight is the only caller.

The rule for a new check: it must be able to fill its row in the mode table in
[CHECK-TAXONOMY.md](../CHECK-TAXONOMY.md). If it is meaningless in verify mode,
it is the wrong shape.

## Consequences

- Verify mode inherits every check written since phase 1 for free.
- The mode is recorded on every `Result`, so a baseline captured in verify mode
  is never silently compared against a preflight assertion.
- **Cost:** phase-1 checks carry a mode parameter most of them barely use, which
  looks like over-engineering until phase 2. Accepted deliberately.
- **Cost:** a check that is genuinely preflight-only — if one exists — has to
  justify itself in a comment rather than just being written. That friction is
  the point.
- **Unresolved:** whether a verify run that finds a *better* state than declared
  (a wider pool, a higher MTU) is a pass or a drift. Recorded in
  `cmd/vksinspect/verify.go`; must be settled before verify ships.
