# ADR-0008 — The requirements matrix is the master list

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
flagged as unconfirmed. Code that quietly builds in an unconfirmed requirement
turns a guess into a stated fact.

## Decision

[REQUIREMENTS-MATRIX.md](../REQUIREMENTS-MATRIX.md) is the master list. Rules:

1. **Every check names requirement IDs.** `registry.Register` panics at startup
   on a check with an empty `RequirementIDs`. A check that points at nothing
   cannot explain why it is failing someone's deployment.
2. **Many to many.** One probe can cover several rows; one row can need several
   probes. Forcing one-to-one would distort both.
3. **Flagged rows do not get built.** A row marked `⚑` is a question, not a
   specification. Building a check on it produces confident, wrong output, which
   is worse than no check at all because people will believe it.
4. **Version-dependent information is data, never code.** Port lists, version
   compatibility tables and minimum prefix sizes change on their own schedule.
   Hardcoding them ships a lie with an expiry date. They are supplied as files
   that the user or a release can update. See `COM-FW-007` and `COM-VER-001`.

## Consequences

- Coverage is easy to read off: matrix rows, minus the ones a check covers.
- Someone with product knowledge and a lab can correct the tool's claims without
  reading any Go.
- What we are unsure about is visible in the document rather than buried in code.
- **Cost:** the matrix is markdown, so nothing checks that it and the code agree.
  A check can name `COM-DNS-999` and nothing notices. Fixing that needs a form a
  program can read — a YAML file alongside it, embedded with `go:embed` — which
  is also what `explain` needs in order to explain requirements that have no
  check yet.
  Deliberately deferred: embedding unverified content would give it an authority
  it has not earned.
- **Cost:** the discipline is only as strong as review. Nothing stops a
  contributor citing a plausible ID that does not exist.
