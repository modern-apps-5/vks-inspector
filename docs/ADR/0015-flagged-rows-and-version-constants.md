# ADR-0015 — What a flag blocks, and when a version may be a constant

**Status:** Accepted · **Date:** 2026-07-30
**Refines:** [ADR-0008](0008-requirements-matrix-authority.md) rules 3 and 4. That
ADR stands; this narrows two of its rules where applying them literally produced
the wrong answer.

## Context

ADR-0008 rule 3 says flagged rows do not get implemented. Rule 4 says
version-dependent data is data, never code. Both are right in the general case
and both were being read more broadly than they can bear.

**Rule 3.** Four checks that shipped name a `⚑` row: `vc.api-reachable`,
`dns.reverse`, `dns.resolver-agreement`, `ntp.skew`. Read literally, that is four
violations. Read closely, it is none — in each case, what the flag doubts is not
what the check claims:

| Row | What the flag doubts | What the check does |
|---|---|---|
| `COM-NTP-002` | The 30s tolerance is a field heuristic, not a product limit | Takes the tolerance from config; skips when none is declared |
| `COM-DNS-002` | Whether PTR is required or merely recommended | Defers severity to `services.dns.requireReverse` |
| `COM-DNS-005` | That it is a diagnostic, not a sourced requirement | Stays `warning`, as the flag instructs |
| `COM-API-001` | Whether the *deployment* account's privileges should be checked | Checks the tool's own access, and says so |

Applying rule 3 literally would delete four useful checks to settle a
bookkeeping argument. Leaving it unsaid invites the opposite mistake: someone
names a flagged row for a check that *does* claim the doubted thing, points at
these four as precedent, and ships a guess as a finding.

**Rule 4.** `flb.version-supported` and `hap.version-supported` need a vCenter
major version to compare against. Rule 4 says that must be supplied data. But
the thing being encoded is not an interoperability grid — it is that Foundation
Load Balancer **did not exist** before vCenter 9.0, and that HAProxy is being
phased out from 9.x. Shipping a data file so a site can configure whether a
product existed is not flexibility, it is an invitation to misconfigure.

## Decision

**On flags.** *If a row is flagged over something the check does not claim, that
flag does not block it.* If it is flagged over **the very thing the check would
claim**, it stays blocked, and that is the usual case.

The test is mechanical: say what the doubt is, then say what the check claims. If
the claim holds up whichever way the doubt is resolved, the flag does not block
it. A check that takes the doubtful part from you — reads it from config, takes
its severity from a setting, or narrows its claim to leave it out — passes this
test automatically.

**On version constants.** The **single version at which one named product
starts or stops being supported** may be a constant inside the check that needs
it, as long as it is named, has a comment saying what would change it, and points
at a matrix row.

Everything else rule 4 covers stays data: patch-level compatibility grids, port
lists, minimum prefix sizes — anything that is a *table* rather than a single
line, or that changes on a schedule of its own.

The distinguishing question: **would this need updating on a schedule the tool
does not control?** An interoperability matrix does. "FLB does not exist before
9.0" does not — it becomes wrong only if the product's history is rewritten.

## Consequences

- Four existing checks are allowed to stand without weakening the rule they
  appeared to break, and the reasoning is written down rather than rediscovered
  in review every time.
- The matrix has a *Status keys* section stating both rules where an author will
  actually run into them.
- `flb.version-supported` and `hap.version-supported` keep their constants.
- **Cost:** the new form of rule 3 needs judgement, where the old one needed
  none. "Does the check claim the doubted thing" is a real question with arguable
  answers, and someone will get it wrong occasionally. The blunt version was
  easier to apply and wrong more often.
- **Cost:** the version exception is a hole in rule 4, and holes widen. The limit
  is "one product, one support boundary, cited and commented". A second constant
  that does not fit that shape means the exception was written too widely, not
  that it should be widened further.
- **Open:** if a version boundary ever needs revising on a cadence — a support
  policy that shifts per release, say — it has stopped being a boundary and
  should become data. Nothing detects that automatically; it needs noticing.
