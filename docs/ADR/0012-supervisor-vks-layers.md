# ADR-0012 — Requirements are tagged Supervisor or VKS

**Status:** Accepted · **Date:** 2026-07-29

## Context

The tool is called vks-inspector and the phase-1 requirements matrix was written
as "VKS deployment prerequisites". That framing was wrong, and reviewing the 88
rows made it obvious: the overwhelming majority are **Supervisor enablement**
prerequisites — management network sizing, Tier-0 and edge cluster, transport
zones, portgroups, ALB cloud configuration, control plane VIPs.

There is no VKS without a Supervisor. So most of what an operator means by "is
this ready for VKS" is really "can the Supervisor be enabled here". The
genuinely VKS-cluster-layer requirements are a much smaller set: TKr and content
library availability, workload node address sizing, per-cluster load balancer
service consumption.

Conflating them produces a report that answers neither question cleanly. They
are also asked at different times, often by different people: Supervisor
enablement is a one-off platform activity, VKS cluster provisioning is ongoing.

## Decision

Every check declares a `Layer`:

| Layer | Meaning |
|---|---|
| `supervisor` | Prerequisite for enabling the Supervisor. Must hold before anything else is possible. |
| `vks` | Prerequisite for provisioning VKS/TKG workload clusters on an enabled Supervisor. |
| `both` | Applies at both layers. |

`--layer supervisor|vks|both` filters a run. The layer is recorded on every
result and in `Run.Layer`, so a Supervisor-only run is never mistaken for full
coverage.

**The default for an unset `Meta.Layer` is `supervisor`, not `both`.** That is
the honest default — most checks genuinely are Supervisor-layer — and defaulting
to `both` would silently overstate what a `--layer vks` run had covered.

**A run in which every check was skipped says so.** The terminal renderer
replaces "no blockers, no warnings" with an explicit statement that nothing was
inspected. `--layer vks` against a build whose checks are all Supervisor-layer
would otherwise exit 0 and read as a clean bill of health.

## Consequences

- An operator can ask the two questions separately, which is how they arise.
- The README and matrix now say plainly that this is mostly a Supervisor
  readiness checker. That is a more accurate description of the product than its
  name suggests, and saying so is better than letting the name mislead.
- **Cost:** every matrix row needs a layer tag. The rows have not all been
  tagged yet — the field exists and the checks use it, but the markdown matrix
  has not been re-annotated. Listed as debt in CLAUDE.md.
- **Cost:** `both` is doing double duty as "applies at both layers" and as the
  filter value meaning "run everything". It reads naturally in both positions
  and a separate `all` value seemed like ceremony, but it is a slight conflation.
- **Unresolved:** whether the tool should refuse `--layer vks` in preflight
  mode. A VKS-layer check before the Supervisor exists is arguably meaningless.
  Currently allowed, and such checks will mostly skip on their own.
