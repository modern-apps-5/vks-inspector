# ADR-0011 — Topology is two separate settings, not one list

**Status:** Accepted · **Date:** 2026-07-29
**Supersedes:** the flat `Topology` enum introduced in the phase-1 scaffold.

## Context

The scaffold modelled topology as one list of five values: `nsx`, `nsx-alb`,
`vds-alb`, `vds-haproxy`, `nsx-vpc`. It broke down immediately.

`nsx-vpc` says nothing about the load balancer, which is a real question with a
real answer. Every requirement row had to list which of the five combinations it
applied to, so a sixth combination would mean editing every row and every check.
It also did not match how anyone describes an environment. In the field the
questions are "NSX or VDS?" and "Avi or the NSX load balancer?", asked
separately.

## Decision

Two settings, chosen separately:

```yaml
topology:
  networking:   nsx      # vds | nsx | nsx-vpc
  loadBalancer: alb      # nsx-lb | alb | haproxy
```

Each check declares an `Applicability` against whichever setting it actually
depends on. The NSX overlay MTU requirement depends on `networking` and does not
care about the load balancer. The ALB licence-tier requirement depends on
`loadBalancer` and does not care about the networking. Leaving one blank means
"any".

**Not every combination is valid.** `validCombinations` in
`internal/config/topology.go` decides. A combination that is not in there is
rejected rather than assumed to work — telling someone their unsupported design
passed preflight is the worst thing this tool could do. `vds` + `nsx-lb` is the
obvious impossible pairing.

**Combinations that work but come with a caveat carry a note** and still pass.
HAProxy and VPC-based networking both get one. Refusing to grade them would only
be more honest if we were sure they were unsupported, and we are not. The note
appears in the interactive prompt while you are choosing, and again in the
result.

## Consequences

- Supporting a new combination is one map entry, not an edit to every check.
- The interactive prompt asks the two questions separately, and only offers load
  balancers that work with the networking you picked, so it cannot produce a
  combination the loader would then reject.
- `--topology nsx+alb` is the shorthand on the command line. `Topology.String()`
  is `networking+loadBalancer`, and that is what appears in reports.
- **Cost:** two fields to set instead of one, and a table of valid combinations
  to keep up to date.
- **Cost:** requirement rows in the matrix still say "Topologies: nsx, nsx-alb".
  The matrix has not been rewritten in terms of the two settings. That is a docs
  gap rather than a code one, and it is listed in
  [CONTRIBUTING.md](../CONTRIBUTING.md).
- **Open:** whether `nsx-vpc` + `haproxy` should exist. It is absent today, on
  the assumption that nobody is putting an older appliance in front of VCF 9 VPC
  networking. Unverified, like everything else about VPC.
