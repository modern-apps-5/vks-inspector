# Check taxonomy

Every requirement in [REQUIREMENTS-MATRIX.md](REQUIREMENTS-MATRIX.md) falls into
one of three classes, determined by **what the check needs in order to observe
anything**. The class decides where the code lives, how it is tested, what it
does when its inputs are missing, and how much of it can run on a laptop.

| Class | Needs | Package | Capability | Runs without a lab? |
|---|---|---|---|---|
| **(c)** Config validation | Nothing | `internal/checks/configval` | — | Yes, always |
| **(a)** Network-only | A host on the right segment | `internal/checks/network` | `network` | Unit-testable with a fake probe |
| **(b)** Credentialed | Management-plane API access | `internal/checks/{vcenter,nsx,alb,flb}` | `vcenter` / `nsx` / `alb` | Only with recorded fixtures |

The ordering above is deliberate and is the order the engine runs them in.
Class (c) is free, instant and deterministic; class (a) costs seconds and a
network; class (b) costs credentials and a round trip. Running the cheap,
certain checks first means a broken address plan is reported before the tool
spends thirty seconds discovering it also cannot reach vCenter.

---

## The rule that decides the class

> **What is the minimum a check needs in order to produce a real observation?**

Not "where does the answer conceptually belong" — where does the *evidence* come
from. `COM-MTU-001` (underlay MTU) is a network fact, an NSX configuration fact
and a VDS configuration fact all at once; it appears in all three classes as
separate checks with separate IDs, because they are three different
observations with three different failure modes and three different
remediations. Collapsing them into one check would produce a single result that
cannot say *which* layer is wrong, which is the only thing the operator needs to
know.

---

## Class (c) — Pure config validation

**Needs:** the config document. No network, no credentials, no clock.

Arithmetic and containment over the declared address plan. This class catches
the single most common category of field failure — an address plan that could
never have worked — and it catches it in milliseconds, offline, before anyone
books a maintenance window.

**Requirements in this class:**

| ID | What it computes |
|---|---|
| `COM-CID-001` | Pairwise overlap across all declared ranges |
| `COM-CID-002` | Overlap against `kubernetes.externalCIDRs` |
| `COM-CID-003` | Pod/service CIDR collision with reachable infrastructure |
| `COM-CID-004` ⚑ | Minimum prefix size per role |
| `COM-MTU-002` | Declared segment MTUs mutually consistent |
| `SUP-MGT-001` ⚑ | Management range contiguity and count |
| `SUP-MGT-004` (part) | API VIP containment in the correct subnet |
| `NSX-ING-002` ⚑ | Ingress range sized against expected LB services |
| `NSX-EGR-002` ⚑ | Egress range sized against expected namespaces |
| `NSX-POD-001` (part) ⚑ | Pod block sized against expected namespaces |
| `LB-VIP-001` | VIP range containment in the frontend subnet (ALB and FLB) |
| `LB-VIP-002` | VIP range overlap with SE data / transit and node ranges (ALB and FLB) |
| `LB-VIP-006` ⚑ | VIP range sized against expected LB services (ALB and FLB) |
| `VDS-WKL-001` ⚑ | Workload range sized against expected node count |
| `COM-FW-006` (part) | `noProxy` covers internal ranges |
| `MET-001` | Topology recognised |
| `LB-FLB-002` | Declared arm mode (two-arm/one-arm/one-arm-one-nic) has its required networks |
| `LB-FLB-003` ⚑ | `one-arm-one-nic` only used with a Simplified Supervisor (blocked on a missing config field) |

**Two things this class is not:**

1. **It is not config-file schema validation.** "Is this valid YAML with a known
   `apiVersion`" is `internal/config`'s job and happens before any check runs; a
   malformed document is a startup error because the user cannot act on a report
   they cannot get. Everything gradeable is a Result.

2. **It is not preflight-only.** In verify mode these same checks re-run against
   the address plan *read back from the live environment* rather than the
   declared one — which is how you catch a Supervisor that was enabled with a
   different pod CIDR than the one in the config. That is the entire reason they
   are Checks behind the standard interface instead of a validation pass in the
   loader.

**Sizing checks depend on declared intent.** `scale.expectedNamespaces` and
friends are not discoverable in preflight — nothing exists yet. A sizing check
with no declared scale must report `skip` with a clear reason, never `pass`.
Passing a sizing check because nobody said how big the deployment would be is
the tool lying.

---

## Class (a) — Verifiable from the network alone

**Needs:** a host with an IP address on the relevant segment. No credentials.

This is the class that makes the tool useful to someone who has been handed a
jump host and no vCenter account, which is the common field situation.

**Requirements in this class:**

| ID | Probe |
|---|---|
| `COM-DNS-001` | Forward A/AAAA per name, per declared resolver |
| `COM-DNS-002` ⚑ | PTR lookup and forward-agreement |
| `COM-DNS-003` | Resolver liveness on 53/udp and 53/tcp |
| `COM-DNS-004` ⚑ | Supervisor endpoint name resolution |
| `COM-DNS-005` ⚑ | Cross-resolver agreement |
| `COM-NTP-001` | SNTP query on 123/udp — a real time query |
| `COM-NTP-002` ⚑ (part) | Offset against the declared sources |
| `COM-FW-001`…`005` | TCP connect, tri-state |
| `COM-FW-007` ⚑ | Port matrix, driven by supplied data |
| `COM-CRT-001`…`003` | TLS handshake, chain and expiry inspection |
| `COM-RTE-001` | Gateway liveness |
| `COM-RTE-002` | Routable-range reachability from outside |
| `COM-ADR-001` ⚑ | Duplicate/occupied address detection |
| `COM-MTU-005` | Path MTU — **INVASIVE** |
| `SUP-MGT-002` | Management-segment reachability of the management plane |
| `VDS-WKL-002` | Workload-segment reachability of the management plane |
| `LB-ALB-001` (part) | Controller reachability on 443 |
| `LB-ALB-008` | Controller reachability **from the management segment** |
| `LB-VIP-005` | SE data to workload reachability |
| `LB-HAP-001` | Data Plane API reachability |

### Three rules this class must not break

**1. Vantage point is part of the result, not a footnote.**
"TCP 443 to vCenter is open" is not a fact about the environment; it is a fact
about the environment *as seen from one host*. A pass from an operator's laptop
with wide access says nothing about whether the workload segment can reach the
registry. Every report records `run.vantage`, and requirements that need a
specific vantage (`COM-DNS-003`, `COM-FW-005`, `SUP-MGT-002`, `VDS-WKL-002`,
`LB-ALB-008`) say so explicitly. This is the single most likely way for this
tool to produce a green report and a failed deployment.

**2. Port state is tri-state, never boolean.**
`open` (connection accepted), `refused` (RST — reachable, nothing listening),
and `filtered` (silence). These are three different findings with three
different remediations. Collapsing them into a bool is how a firewall gets
reported as a dead service. `filtered` maps to `StatusUnknown`, not
`StatusFail`: we did not observe a failure and we do not get to assert one.

**3. Absence of evidence is not evidence.**
An address that does not answer ARP is not necessarily free. A resolver that
times out has not necessarily failed. Where a probe cannot distinguish, the
result is `unknown` with the ambiguity stated, and the exit code reflects that
(2, not 1).

### Privilege degradation

ICMP and ARP probes may need raw sockets — root or `CAP_NET_RAW`. A field
engineer on a customer laptop, unprivileged, is the **normal** case. When the
tool cannot get a raw socket it must report a skip with the reason and, where
possible, fall back to a TCP-based probe that answers a weaker version of the
question. It must never fail hard, and it must never silently pass.

---

## Class (b) — Requires management-plane credentials

**Needs:** vCenter, NSX and/or ALB API credentials.

Everything about how the infrastructure is *configured*, as opposed to how it
currently *behaves*: portgroup VLANs, IP pool allocations, transport zone
membership, license tiers.

**Requirements in this class:**

| ID group | Surface |
|---|---|
| `COM-API-001` ⚑, `COM-VER-001` ⚑ | vCenter — reachability, privileges, version |
| `COM-NTP-003` ⚑, `COM-NTP-004` ⚑ | vCenter — host and appliance time |
| `COM-MTU-003` | vCenter — VDS MTU |
| `VDS-PG-001`…`004` ⚑ | vCenter — portgroup existence, VLAN, security policy |
| `LB-HAP-004` | vCenter — appliance interface placement |
| `COM-API-002`, `COM-VER-002` ⚑, `COM-NTP-005` ⚑ | NSX — reachability, version, time |
| `NSX-T0-001`…`003` ⚑ | NSX — Tier-0 existence, uplinks, HA mode |
| `NSX-EDG-001` ⚑, `NSX-TZ-001`, `NSX-TZ-002` | NSX — edge cluster, transport zones, host prep |
| `NSX-ING-001`, `NSX-EGR-001`, `NSX-POD-001` ⚑ | NSX — IP block allocation state |
| `NSX-DFW-001` ⚑ | NSX — distributed firewall |
| `COM-MTU-004` ⚑ | NSX — uplink profile MTU |
| `VPC-*` ⚑ | NSX — VPC object model (all low confidence) |
| `LB-ALB-002`…`007` ⚑ | ALB — version, cluster, license, cloud, SE group |
| `LB-VIP-003`, `LB-VIP-004` | ALB — IPAM pools and allocation |
| `LB-HAP-002`, `LB-HAP-003` | HAProxy — Data Plane API |
| `LB-FLB-000` | vCenter — FLB version-existence boundary (implemented: `flb.version-supported`) |
| `LB-HAP-000` | vCenter — HAProxy support-lifecycle boundary (implemented: `hap.version-supported`, warning severity) |
| `LB-FLB-001` ⚑, `LB-FLB-004` ⚑ | vCenter — FLB VM placement/health and cluster HA prerequisites (no dedicated controller) |
| `LB-FLB-005` ⚑ | vCenter — FLB VIP allocation state, if/when exposed (LOW confidence) |

### Rules this class must not break

**1. Read-only, enforced by review.** No method in `internal/clients` may
create, modify or delete anything — the only permitted write is the session-open
call, and the session must be explicitly closed so the tool does not litter a
customer's vCenter with sessions. See
[ADR-0007](ADR/0007-read-only-by-default.md).

**2. Missing credentials produce skips, never failures.** "We could not log in
to NSX" is a statement about the tool's access, not about the customer's
network. The capability system in `internal/checks` handles this: a check
declares `Needs: []Capability{CapNSX}` and the engine skips it with a reason if
no NSX client could be built. A tool that reported failures for every
credentialed check when run without credentials would be unusable in the field
situation it is designed for.

**3. Every lab response gets recorded as a fixture.** This is how class (b)
becomes CI-testable at all. See [unit-test-coverage.md](unit-test-coverage.md).

---

## Checks that span classes

Some requirements are only fully answered by combining classes. The pattern is
**one check per class, distinct IDs, cross-referencing the same requirement** —
not one check that behaves differently depending on what it has.

| Requirement | Class (c) | Class (a) | Class (b) |
|---|---|---|---|
| MTU meets the overlay minimum | declared consistency (`COM-MTU-002`) | actual path MTU (`COM-MTU-005`, invasive) | configured VDS/uplink MTU (`COM-MTU-003`, `COM-MTU-004`) |
| VIP range is usable | containment and overlap (`LB-VIP-001/002`) | addresses are free (`COM-ADR-001`) | pool exists and is unallocated (`LB-VIP-003/004`) |
| Clock is sane | — | offset vs declared sources (`COM-NTP-002`) | host and appliance config (`COM-NTP-003/004/005`) |
| Ingress range is usable | sized (`NSX-ING-002`) | routable (`COM-RTE-002`) | unallocated (`NSX-ING-001`) |

Why not one check with three code paths? Because the operator's next action is
different in each case. "The VDS MTU is 1500" and "a hop in the underlay is
clamping at 1500" are the same symptom and completely different tickets. A check
that merges them produces a result that cannot tell the operator which one they
have.

---

## How the classes behave across modes

The class does not change with mode; the *source of the observation* does.

| Class | `check` (preflight) | `verify` | `snapshot` / `drift` |
|---|---|---|---|
| (c) | Arithmetic on declared config | Arithmetic on the address plan read back from the live environment | Records the plan; drift catches a changed plan |
| (a) | Probes from this host | Same probes, plus probes from inside the deployment where reachable | Records observations; drift catches changed reachability |
| (b) | Reads infrastructure config as it stands | Reads infrastructure config plus what the deployment created | Records config state; drift catches config changes |

This table is the whole architecture in one place. If a proposed check cannot
fill a cell in its row — if it is meaningless in verify mode, or produces
nothing a baseline could store — it is the wrong shape, and that is a design
conversation rather than an exception to grant.
