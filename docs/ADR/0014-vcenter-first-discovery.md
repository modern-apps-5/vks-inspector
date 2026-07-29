# ADR-0014 — vCenter is the entry point; discover the rest

**Status:** Accepted · **Date:** 2026-07-29 · **Partially implemented**

## Context

The phase-1 scaffold treated vCenter, NSX Manager and the ALB controller as
three equally-declared endpoints, all typed into a config by hand, with the
credentialed clients deferred to a late phase. That inverts the actual workflow.

An operator has a vCenter address and credentials. vCenter already knows the
datacenter, the clusters, the hosts, the distributed switches, and — when NSX is
in play — the registered NSX Manager. Asking a human to retype all of that is
both tedious and a source of error: a typo produces a check that inspects the
wrong object and reports confidently on something that is not the environment
under test.

## Decision

**vCenter is the entry point.** `--vcenter <fqdn>` plus credentials is the
minimum input. Everything the tool can learn from vCenter, it learns:
datacenter, cluster, host inventory, distributed switches, registered NSX
Manager, and version information.

Discovered values are **reported, not asked**. The prompt flow prints a
"Discovered from vCenter" block before asking anything, so the operator sees
which questions were answered for them and can correct one that is wrong.

Discovered values are written into the saved config, so a later non-interactive
run grades against the same objects without re-discovering them — and so drift
can tell "the cluster changed" from "we looked at a different cluster".

What is **not** discoverable and must still be declared: the intended address
plan. Ingress and egress CIDRs, pool ranges and expected scale describe an
environment that does not exist yet. That is the irreducible core of the
question set.

## Consequences

- The minimum viable invocation is one flag and a credential.
- Fewer typo-shaped failures, and the ones that remain are visible in the
  discovery block before any check runs.
- **Cost:** a run without vCenter credentials asks more questions. Acceptable —
  it degrades to the config-only path rather than failing.
- **Cost:** discovery makes the tool's behaviour depend on the target
  environment before any check has run. A malformed vCenter response could
  derail the prompt flow. Discovery must therefore be best-effort: any failure
  falls back to asking, and never aborts the run.

## Implementation status

**The seam is in place; the vCenter client is not.** `internal/clients/vcenter`
is still a stub, `prompt.Discovered` is defined and rendered but always nil, and
`cmd/vksinspect/check.go` carries the TODO where discovery will be called.
Every credentialed check consequently reports as a skip with a reason.

This ADR is written now, ahead of the implementation, because the decision
shapes the config schema and the prompt flow — both of which are being built
now and would be wrong if they assumed hand-declared endpoints.
