# The three kinds of check

Every requirement in [REQUIREMENTS-MATRIX.md](REQUIREMENTS-MATRIX.md) is checked
in one of three ways, and the difference is simply **what the check needs before
it can see anything**. That decides where the code lives, how it is tested, what
it does when its inputs are missing, and how much of it can run on a laptop.

| Kind | Needs | Package | Access it asks for | Runs without a lab? |
|---|---|---|---|---|
| **Config check** | Nothing | `internal/checks/configval` | — | Yes, always |
| **Network check** | A host on the right network | `internal/checks/network` | `network` | Yes, with a fake probe |
| **API check** | vCenter / NSX / ALB login | `internal/checks/{vcenter,nsx,alb,flb}` | `vcenter` / `nsx` / `alb` | Only with recorded responses |

The engine runs them in that order, on purpose. Config checks are free, instant
and always give the same answer. Network checks cost seconds and a network. API
checks cost credentials and a round trip. Running the cheap, certain ones first
means a broken address plan gets reported straight away, instead of after thirty
seconds of discovering that vCenter is also unreachable.

---

## How to tell which kind a check is

> **What is the least a check needs before it can observe anything real?**

The question is not "where does this topic belong". It is "where does the
*evidence* come from".

Take `COM-MTU-001`, the underlay MTU. That is a network fact, an NSX setting and
a VDS setting all at once, so it shows up as three separate checks with three
separate IDs — one of each kind. They are three different observations, they
fail for three different reasons, and each one needs a different fix. A single
combined check would produce one result that cannot say *which* layer is wrong,
and that is the only thing the person reading the report needs to know.

---

## Config checks

**Need:** the config file. No network, no credentials, no clock.

These do arithmetic on the address plan you declared — overlaps, containment,
range sizes. It is the most common way a deployment fails in the field: an
address plan that could never have worked. These checks catch it in
milliseconds, offline, before anyone books a maintenance window.

**Requirements checked this way:**

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

### What these are not

**1. They are not YAML validation.** "Is this valid YAML with a known
`apiVersion`" is `internal/config`'s job, and it happens before any check runs. A
malformed file is a startup error, because you cannot act on a report you never
get. Anything that can be graded is a Result instead.

**2. They are not preflight-only.** In `verify` mode the same checks run again,
this time against the address plan read back out of the live environment. That is
how you catch a Supervisor that was enabled with a different pod CIDR than the
config says. It is also the whole reason these are Checks behind the normal
interface, rather than extra validation inside the config loader.

**Sizing checks need you to say how big the deployment will be.**
`scale.expectedNamespaces` and its siblings cannot be discovered during preflight
— nothing exists yet. A sizing check with no declared scale reports `skip` with
a reason. It must never report `pass`: passing a sizing check because nobody said
how big the deployment would be is the tool lying.

---

## Network checks

**Need:** a host with an IP address on the relevant network. No credentials.

This is what makes the tool useful to someone who has been handed a jump host and
no vCenter account, which is the usual field situation.

**Requirements checked this way:**

| ID | Probe |
|---|---|
| `COM-DNS-001` | Forward A/AAAA per name, per declared resolver |
| `COM-DNS-002` ⚑ | PTR lookup and forward-agreement |
| `COM-DNS-003` | Resolver liveness on 53/udp and 53/tcp |
| `COM-DNS-004` ⚑ | Supervisor endpoint name resolution |
| `COM-DNS-005` ⚑ | Cross-resolver agreement |
| `COM-NTP-001` | SNTP query on 123/udp — a real time query |
| `COM-NTP-002` ⚑ (part) | Offset against the declared sources |
| `COM-FW-001`…`005` | TCP connect, three outcomes |
| `COM-FW-007` ⚑ | Port matrix, driven by supplied data |
| `COM-CRT-001`…`003` | TLS handshake, chain and expiry inspection |
| `COM-RTE-001` | Gateway liveness |
| `COM-RTE-002` | Routable-range reachability from outside |
| `COM-ADR-001` ⚑ | Duplicate/occupied address detection |
| `COM-MTU-005` | Path MTU — **INVASIVE** |
| `SUP-MGT-002` | Management network can reach the management plane |
| `VDS-WKL-002` | Workload network can reach the management plane |
| `LB-ALB-001` (part) | Controller reachability on 443 |
| `LB-ALB-008` | Controller reachability **from the management network** |
| `LB-VIP-005` | SE data to workload reachability |
| `LB-HAP-001` | Data Plane API reachability |

### Three rules for network checks

**1. Where the check ran is part of the answer, not a footnote.**
"TCP 443 to vCenter is open" is not a fact about the environment. It is a fact
about the environment *as seen from one machine*. A pass from an operator laptop
with wide access says nothing about whether the workload network can reach the
registry. Every report records the host the probes ran from, and the
requirements that only mean something from a specific network say so:
`COM-DNS-003`, `COM-FW-005`, `SUP-MGT-002`, `VDS-WKL-002`, `LB-ALB-008`. This is
the most likely way for this tool to produce a green report and a failed
deployment.

**2. A port has three states, not two.**
`open` (connection accepted), `refused` (RST — something is there, nothing is
listening), and `filtered` (silence). Three different findings, three different
fixes. Squashing them into true/false is how a firewall gets reported as a dead
service. `filtered` becomes `StatusUnknown`, not `StatusFail`: we did not see a
failure, so we do not get to report one.

**3. Not seeing something is not the same as it not being there.**
An address that does not answer ARP is not necessarily free. A resolver that
times out has not necessarily failed. When a probe cannot tell the difference,
the result is `unknown`, the report says what was ambiguous, and the exit code
reflects it (2, not 1).

### Running without root

ICMP and ARP probes may need raw sockets, which means root or `CAP_NET_RAW`. A
field engineer on a customer laptop, without either, is the **normal** case. When
the tool cannot open a raw socket it reports a skip with the reason and, where it
can, falls back to a TCP probe that answers a weaker version of the question. It
must never fail hard, and it must never quietly pass.

---

## API checks

**Need:** vCenter, NSX and/or ALB credentials.

These cover how the infrastructure is *configured*, as opposed to how it
currently *behaves*: portgroup VLANs, IP pool allocations, transport zone
membership, licence tiers.

**Requirements checked this way:**

| ID group | Where it reads from |
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

### Three rules for API checks

**1. Read-only, enforced by review.** No method in `internal/clients` may create,
change or delete anything. The one write allowed is opening a session, and the
session must be closed explicitly so the tool does not leave sessions lying
around in a customer's vCenter. See [ADR-0007](ADR/0007-read-only-by-default.md).

**2. Missing credentials produce skips, never failures.** "We could not log in to
NSX" says something about the tool's access, not about the customer's network.
`internal/checks` handles this: a check declares `Needs: []Capability{CapNSX}`,
and if no NSX client could be built the engine skips it with a reason. A tool
that failed every credentialed check when run without credentials would be
useless in exactly the situation it was built for.

**3. Every real API response gets recorded.** Saving responses from a lab is the
only way these checks can be tested in CI at all. See
[unit-test-coverage.md](unit-test-coverage.md).

---

## Requirements that need more than one kind

Some requirements are only fully answered by combining kinds. The pattern is
**one check per kind, each with its own ID, all pointing at the same
requirement** — not one check that behaves differently depending on what it has.

| Requirement | Config check | Network check | API check |
|---|---|---|---|
| MTU meets the overlay minimum | declared consistency (`COM-MTU-002`) | actual path MTU (`COM-MTU-005`, invasive) | configured VDS/uplink MTU (`COM-MTU-003`, `COM-MTU-004`) |
| VIP range is usable | containment and overlap (`LB-VIP-001/002`) | addresses are free (`COM-ADR-001`) | pool exists and is unallocated (`LB-VIP-003/004`) |
| Clock is sane | — | offset vs declared sources (`COM-NTP-002`) | host and appliance config (`COM-NTP-003/004/005`) |
| Ingress range is usable | sized (`NSX-ING-002`) | routable (`COM-RTE-002`) | unallocated (`NSX-ING-001`) |

Why not one check with three code paths? Because what you do next is different in
each case. "The VDS MTU is 1500" and "a hop in the underlay is clamping at 1500"
look identical in the symptoms and are completely different tickets. A check that
merges them gives you a result that cannot tell you which one you have.

---

## The same check in every mode

The kind of check never changes with the mode. What changes is where the
observation comes from.

| Kind | `check` (preflight) | `verify` | `snapshot` / `drift` |
|---|---|---|---|
| Config | Arithmetic on the declared config | Arithmetic on the address plan read back from the live environment | Records the plan; drift catches a changed plan |
| Network | Probes from this host | Same probes, plus probes from inside the deployment where reachable | Records observations; drift catches changed reachability |
| API | Reads infrastructure config as it stands | Reads infrastructure config plus what the deployment created | Records config state; drift catches config changes |

That table is the whole design in one place. If a proposed check cannot fill a
cell in its row — if it means nothing in verify mode, or produces nothing a
baseline could store — it is the wrong shape, and needs a design discussion
rather than an exception.
