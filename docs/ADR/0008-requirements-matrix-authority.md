# ADR-0008 — The requirements matrix is the source of truth

**Status:** Accepted · **Date:** 2026-07-29

## Context

A preflight tool's real product is its claim about what a correct environment
looks like. If that claim lives only in code, three things follow: nobody can
review it without reading Go, nobody can tell which requirements are covered
and which are not, and a check that is subtly wrong is indistinguishable from
one that is right.

There is a sharper version of the problem specific to this project. The
requirements themselves are **not fully known**. The matrix was written from
model knowledge with no live documentation access, and 46 of its 88 rows are
flagged as unconfirmed. Code that silently encodes an unconfirmed requirement
launders a guess into an assertion.

## Decision

[REQUIREMENTS-MATRIX.md](../REQUIREMENTS-MATRIX.md) is authoritative. Rules:

1. **Every check cites requirement IDs.** `registry.Register` panics at startup
   on a check with an empty `RequirementIDs`. A check that traces to nothing
   cannot explain why it is failing someone's deployment.
2. **Many-to-many.** One probe can evidence several rows; one row can need
   several probes. Forcing 1:1 would distort both.
3. **Flagged rows do not get implemented.** A row marked `⚑` is a question, not
   a specification. Building a check on it produces confident, wrong output —
   which is worse than no check, because it will be believed.
4. **Version-dependent data is data, never code.** Port matrices, version
   interoperability tables and minimum prefix sizes change independently of
   releases. Hardcoding them ships a lie with a shelf life. They are supplied as
   files the user or a release can update. See `COM-FW-007` and `COM-VER-001`.

## Consequences

- Coverage is legible: matrix rows minus implemented check IDs.
- A reviewer with product knowledge and a lab can correct the tool's claims
  without reading Go.
- Uncertainty is visible in the artifact rather than buried in an implementation.
- **Cost:** the matrix is markdown, so the coupling between it and the code is
  unverified. A check can cite `COM-DNS-999` and nothing notices. Fixing this
  needs a machine-readable form — a YAML sidecar embedded with `go:embed` —
  which is also what `explain` needs to explain requirements that have no check.
  Deliberately deferred: embedding unverified content would give it an authority
  it has not earned.
- **Cost:** the discipline is only as strong as review. Nothing stops a
  contributor citing a plausible ID that does not exist.
