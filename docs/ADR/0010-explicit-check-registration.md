# ADR-0010 — Explicit registration, no `init()` magic

**Status:** Accepted · **Date:** 2026-07-29

## Context

The way this usually gets done in Go is an `init()` in each check package that
registers itself into a global. It reads nicely and it fails in a nasty,
specific way: which checks exist becomes a matter of which packages happen to be
imported. A refactor that drops an import that looks unused quietly removes
checks from the build, the tests still pass, and the report just has fewer rows
in it than it should.

For a tool whose entire purpose is to tell someone their environment is ready,
a silently-shrinking check set is close to the worst possible bug. The report
looks healthy. It is just incomplete.

## Decision

Registration is explicit. `internal/checks/all.Registry()` names every check
package and calls its `Checks()` function. **If a check is not listed there, it
does not exist.**

The registry defends itself at startup, with panics rather than errors, because
these are programming errors that must not reach a customer:

- empty check ID
- duplicate check ID (a shadowed check silently omits a requirement)
- no `RequirementIDs` (ADR-0008)
- no declared `Modes`

`Registry.All()` returns checks sorted by ID, because report ordering and golden
files depend on determinism.

`Registry.Select()` returns a `Decision` **per registered check**, selected or
not, each carrying a reason. The engine turns non-selections into reported
skips. A report always accounts for every check the build knows about — a report
that silently omits checks overstates its own coverage, which is the same class
of bug as the `init()` problem, one layer up.

## Consequences

- Grepping `all.go` answers "what does this build check".
- Adding a check is two steps — write it, list it — and forgetting the second is
  caught by the missing check in the output rather than by silence.
- Misconfigured checks fail at startup, in CI, not in a customer's environment.
- **Cost:** two places to edit instead of one. Deliberate friction.
- **Cost:** panics are a blunt instrument. They are appropriate here because
  these conditions are unreachable in a correctly-built binary and the tests
  cover them, but a contributor's first encounter with one will be jarring. The
  panic messages say what to fix.
