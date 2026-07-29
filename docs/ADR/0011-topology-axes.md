# ADR-0011 — Topology is orthogonal axes, not a flat enum

**Status:** Accepted · **Date:** 2026-07-29
**Supersedes:** the flat `Topology` enum introduced in the phase-1 scaffold.

## Context

The scaffold modelled topology as one enum with five values: `nsx`, `nsx-alb`,
`vds-alb`, `vds-haproxy`, `nsx-vpc`. It strained immediately.

`nsx-vpc` says nothing about the load balancer, which is a real question with a
real answer. Every requirement row had to re-list which of the five combinations
it applied to, so adding a sixth combination would mean editing every row and
every check. And it did not match how anyone actually describes an environment —
the questions asked in the field are "NSX or VDS?" and "Avi or the NSX load
balancer?", asked separately.

## Decision

Two independent axes:

```yaml
topology:
  networking:   nsx      # vds | nsx | nsx-vpc
  loadBalancer: alb      # nsx-lb | alb | haproxy
```

Checks declare an `Applicability` against whichever axis they actually depend
on. The NSX overlay MTU requirement depends on `networking` and is indifferent
to the load balancer; the ALB licence-tier requirement depends on
`loadBalancer` and is indifferent to the networking. An empty axis means "any".

**Not every combination is valid.** `validCombinations` in
`internal/config/topology.go` is the authority. A combination absent from it is
rejected rather than assumed workable — telling someone their unsupported design
passed preflight is the worst thing this tool could do. `vds` + `nsx-lb` is the
obvious impossible pairing.

**Valid-but-caveated combinations carry a note** and still pass. HAProxy and
VPC-based networking both get one. Refusing to grade them would only be more
honest if we were certain they were unsupported, and we are not. The note is
shown in the interactive prompt at the moment of choosing, and in the result.

## Consequences

- A new supported combination is one map entry, not an edit to every check.
- The interactive prompt asks the two questions separately and only ever offers
  load balancers valid with the chosen networking — it cannot produce a
  combination the loader would then reject.
- `--topology nsx+alb` is the flag shorthand; `Topology.String()` is
  `networking+loadBalancer` and is what appears in reports.
- **Cost:** two fields to set instead of one, and a validity table to maintain.
- **Cost:** requirement rows in the matrix still say "Topologies: nsx, nsx-alb"
  in prose. The matrix has not been mechanically re-expressed in terms of axes.
  That is a docs debt, not a code one, and it is listed in
  [CONTRIBUTING.md](../CONTRIBUTING.md).
- **Open:** whether `nsx-vpc` + `haproxy` should exist. Currently absent, on the
  assumption that nobody is putting a legacy appliance in front of VCF 9 VPC
  networking. Unverified like everything else about VPC.
