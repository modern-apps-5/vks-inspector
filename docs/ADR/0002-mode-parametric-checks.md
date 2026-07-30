# ADR-0002 — One check works in every mode

**Status:** Accepted · **Date:** 2026-07-29

## Context

Phase 1 delivers preflight only. Phases 2–4 add checking after deployment,
saving a baseline, and detecting drift. The obvious way to build phase 1 is as a
preflight tool. The obvious result is that phase 2 becomes a second tool sharing
a name, with every check written twice and the two versions meaning slightly
different things.

The way that fails is predictable. A check written as "does this config make
sense" has nothing to observe in a live environment, so verify mode cannot reuse
it, so verify mode grows its own parallel set of checks, so the two disagree, so
nobody trusts either.

## Decision

A check says which modes it supports (`Meta.Modes`) and is told the mode when it
runs (`RunContext.Mode`). There is **one engine** — `engine.Run` — and `check`,
`verify` and `snapshot` differ only in the `Mode` they pass it. There is no
preflight-only code path anywhere.

All six subcommands exist as stubs from day one, before any of them are needed,
so no package can quietly start assuming preflight is the only caller.

The rule for a new check: it has to fill its row in the mode table in
[check-types.md](../check-types.md). If it means nothing in verify mode, it is
the wrong shape.

## Consequences

- Verify mode gets every check written since phase 1 for free.
- The mode is recorded on every `Result`, so a baseline captured in verify mode
  never gets quietly compared against a preflight claim.
- **Cost:** phase-1 checks carry a mode parameter most of them barely use, which
  looks like over-engineering until phase 2 arrives. Accepted deliberately.
- **Cost:** a check that really is preflight-only — if one exists — has to
  justify itself in a comment instead of just being written. That friction is
  the point.
- **Unresolved:** whether a verify run that finds a *better* state than declared
  (a wider pool, a higher MTU) is a pass or a drift. Recorded in
  `cmd/vksinspect/verify.go`; must be settled before verify ships.
