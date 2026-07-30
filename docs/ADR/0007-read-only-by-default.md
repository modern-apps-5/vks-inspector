# ADR-0007 — Read-only by default; invasive probes are gated

**Status:** Accepted · **Date:** 2026-07-29

## Context

This tool runs against production. It is frequently run by someone who did not
build the environment, under time pressure, during a change window. The worst
possible outcome is that a preflight tool causes the outage it was run to
prevent.

Most checks are harmless. A few are not. Path-MTU discovery deliberately emits
large DF-flagged packets across production paths; duplicate-IP detection sprays
probes at addresses that are supposed to be unused and may not be; a DHCP probe
consumes a lease.

## Decision

**Read-only against the target environment, always.** No method in
`internal/clients` may create, modify or delete anything. The only permitted
write is the session-open call, and the session must be explicitly closed so the
tool does not litter a customer's vCenter with sessions.

Nothing in the type system enforces this — only convention and review do. That
is why it is stated loudly in the package docs, in
[check-types.md](../check-types.md), and here.

**Anything potentially disruptive is gated.** A check sets `Meta.Invasive` and
declares `CapInvasive`. Without `--invasive` (or `policy.allowInvasive`), the
engine skips it **with a reason that appears in the report**. Nobody should be
able to mistake "not run" for "passed".

Every invasive check is documented as such in the requirements matrix.

**Running without root is part of the design, not an error case.** Raw sockets
for ICMP and ARP need root or `CAP_NET_RAW`. A field engineer on a customer
laptop with neither is the *normal* case. When the tool cannot get that access:
report a skip with the reason, fall back to a weaker probe if one exists, never
fail hard, and never quietly pass.

## Consequences

- The tool is safe to hand to someone and safe to run during a change window.
- Default runs are honest about what they did not do, because skips are
  reported rather than omitted.
- **Cost:** the default run gives an incomplete answer. Path MTU is one of the
  highest-value checks in the matrix and it is off by default. The mitigation is
  that the skip is loud, not that the check is enabled.
- **Cost:** read-only is unenforced by the compiler. A future contributor can
  add a write. Only review catches it. An interface split (read-only interfaces
  in `checks`, concrete clients elsewhere) partially helps and is already how
  `checks.VCenterClient` is shaped, but it does not prevent a client from doing
  something destructive inside a method named `About`.
- **Unresolved:** whether duplicate-IP detection should be invasive. It is
  read-only in effect but noisy. See `COM-ADR-001` in the matrix.
- **Offline by construction:** the tool makes no outbound internet calls at
  runtime. It probes only what the config declares. This is why version and port
  matrices must be supplied as data rather than fetched (ADR-0008).
