# ADR-0015 — What a flag blocks, and when a version may be a constant

**Status:** Accepted · **Date:** 2026-07-30
**Refines:** [ADR-0008](0008-requirements-matrix-authority.md) rules 3 and 4. That
ADR stands; this narrows two of its rules where applying them literally produced
the wrong answer.

## Context

ADR-0008 rule 3 says flagged rows do not get implemented. Rule 4 says
version-dependent data is data, never code. Both are right in the general case
and both were being read more broadly than they can bear.

**Rule 3.** Four shipped checks cite a `⚑` row: `vc.api-reachable`,
`dns.reverse`, `dns.resolver-agreement`, `ntp.skew`. Read literally, four
violations. Read closely, none — in each case what the flag doubts is not what
the check asserts:

| Row | What the flag doubts | What the check does |
|---|---|---|
| `COM-NTP-002` | The 30s tolerance is a field heuristic, not a product limit | Takes the tolerance from config; skips when none is declared |
| `COM-DNS-002` | Whether PTR is required or merely recommended | Defers severity to `services.dns.requireReverse` |
| `COM-DNS-005` | That it is a diagnostic, not a sourced requirement | Stays `warning`, as the flag instructs |
| `COM-API-001` | Whether the *deployment* account's privileges should be checked | Checks the tool's own access, and says so |

Applying rule 3 literally would delete four useful checks to resolve a
bookkeeping conflict. Leaving it unstated invites the opposite error: someone
cites a flagged row for a check that *does* assert the doubted thing, points at
these four as precedent, and ships a guess as a finding.

**Rule 4.** `flb.version-supported` and `hap.version-supported` need a vCenter
major version to compare against. Rule 4 says that must be supplied data. But
the thing being encoded is not an interoperability grid — it is that Foundation
Load Balancer **did not exist** before vCenter 9.0, and that HAProxy is being
phased out from 9.x. Shipping a data file so a site can configure whether a
product existed is not flexibility, it is an invitation to misconfigure.

## Decision

**On flags.** *A row flagged on a dimension the check does not assert is not
blocked by that flag.* A row flagged on **the thing the check would assert**
stays blocked, and that is the common case.

The test is mechanical: name the doubt, then name what the check claims. If the
check's claim survives the doubt being resolved either way, the flag does not
block it. A check that parameterises the doubt — reads it from config, defers
severity, or scopes its claim to exclude it — passes this test by construction.

**On version constants.** A **single existence or support-lifecycle boundary
for one named product** may be a constant in the check that needs it, provided
it is named, commented with what would change it, and cited to a matrix row.

Everything else rule 4 covers stays data: patch-level interoperability grids,
port matrices, minimum prefix sizes — anything that is a *table* rather than a
*boundary*, or that changes independently of any release.

The distinguishing question: **would this need updating on a schedule the tool
does not control?** An interoperability matrix does. "FLB does not exist before
9.0" does not — it becomes wrong only if the product's history is rewritten.

## Consequences

- Four existing checks are legitimised without weakening the rule they appeared
  to break, and the reasoning is written down rather than rediscovered in review.
- The matrix carries a *Status keys* section stating both rules where an author
  will actually meet them.
- `flb.version-supported` and `hap.version-supported` keep their constants.
- **Cost:** rule 3's new form needs judgement, where the old one needed none.
  "Does the check assert the doubted thing" is a real question with arguable
  answers, and it will be got wrong occasionally. The blunt version was cheaper
  to apply and wrong more often.
- **Cost:** the version carve-out is a hole in rule 4, and holes widen. The
  boundary is "one product, one existence/lifecycle line, cited and commented".
  A second constant that does not fit that shape should be read as evidence the
  carve-out was too generous, not as precedent.
- **Open:** if a version boundary ever needs revising on a cadence — a support
  policy that shifts per release, say — it has stopped being a boundary and
  should become data. Nothing detects that automatically; it needs noticing.
