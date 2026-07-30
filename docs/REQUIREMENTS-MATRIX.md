# VKS networking requirements matrix

The master list of networking prerequisites this tool grades against. Every
check in the codebase has to name one or more IDs from this file. A check that
points at nothing here does not ship.

---

## Read this before you trust a single row

**Where this list came from.** It was written from model knowledge with a May
2026 training cutoff. **No product documentation was read while writing it.**
That was deliberate given how the work was commissioned — the reviewer confirms
rows against the product docs and a lab — but it means you should start by
doubting every row, not trusting it.

**There are no URLs here, on purpose.** A made-up documentation link is worse
than no link: it looks official and costs a reviewer real time to disprove. The
`Source` field names the *part of the documentation to go and look in*, not a
URL. Read every one as "go find this", not "this was read".

**Confidence levels.**

| Level | Meaning |
|---|---|
| `HIGH` | A general networking fact, or VMware behaviour that has been stable across many releases. Still worth spot-checking, but unlikely to be wrong. |
| `MED` | Believed correct for TKGS / vSphere with Tanzu. Whether it carried over unchanged into VCF 9 / VKS is genuinely unknown. |
| `LOW` | Pieced together or inferred. Assume it is wrong until someone proves otherwise. |

**Coverage.** 97 rows · **25 done** (26%) · 27 not done but buildable now ·
45 waiting on someone confirming the requirement. Of the 27 buildable, 11 need
nothing new built first. Per-row detail is in each section's summary table; see
[Status keys](#status-keys).

**Flags.** `⚑` marks a row that has to be confirmed before any check is built on
it. **49 of the 97 rows are flagged.** Where the doubt is concentrated, worst
first:

1. **Every VPC row (`VPC-*`)** — VCF 9 VPC-based Supervisor networking is the
   least reliable section here. Even the object *names* may be wrong, never mind
   the values. Do not build anything from this document for VPC.
2. **Every number** — MTU minimums, "5 consecutive IPs", clock-skew tolerance,
   minimum prefix sizes, pool-sizing ratios. Numbers are exactly what changes
   between releases and exactly what this document is least able to get right.
   Where a number appears without a flag, it is a general networking constant
   rather than a product requirement.
3. **The port list (`COM-FW-*`)** — deliberately *not* written out. See
   `COM-FW-007`.
4. **HAProxy (`LB-HAP-*`)** — the topology still matters. An operator confirmed
   it is fully supported on vCenter 8.x and being phased out starting with 9.x
   (see `LB-HAP-000`, now implemented). The Data Plane API rows (`LB-HAP-001`
   through `004`) are still flagged and unbuilt, for their own separate reasons.
5. **Version-compatibility rows (`COM-VER-*`, `LB-ALB-002`)** — these depend on
   an external interoperability matrix that changes on its own schedule, and it
   must never be hardcoded here.

**What a flag does NOT mean.** A row without a flag has not been verified. It is
just one where being wrong costs little, because it is a general networking
fact. No row in this file has been confirmed against a product document.

**One exception: the Foundation Load Balancer section (`LB-FLB-*`).** Unlike the
rest of this matrix, that section's high-level facts — the deployment model, the
two-arm / one-arm / one-arm-one-nic network layouts, HA modes, sizing, and the
vCenter 9.0+ requirement — came from a Broadcom TechDocs page that was actually
read ("Architecture of vSphere Supervisor with Foundation Load Balancer" and its
"Requirements" sibling), rather than from model knowledge. That makes *what FLB
is* more reliable. It does not help with the detail this tool would need to
check FLB properly: which vCenter objects FLB VMs appear as, what in the config
says "Simplified Supervisor", and how FLB allocates VIPs. That page did not
cover any of it, so those rows stay flagged like everything else.

**The brief and the README disagreed, and this file keeps both.** The brief
listed four topologies. The original README listed four *different* ones,
including `NSX + AVI`, which the brief left out. This matrix covers all of them.
`NSX + ALB` is a real supported combination, and dropping it just because the
brief did not mention it would be a scope decision made quietly.

---

## Topology keys

The code models topology as two separate settings
([ADR-0011](ADR/0011-topology-axes.md)); the rows below still use the older flat
names in their **Topologies** field. The mapping is:

| Row says | Means |
|---|---|
| `nsx` | `networking=nsx`, `loadBalancer=nsx-lb` |
| `nsx-alb` | `networking=nsx`, `loadBalancer=alb` |
| `vds-alb` | `networking=vds`, `loadBalancer=alb` |
| `vds-haproxy` | `networking=vds`, `loadBalancer=haproxy` *(fully supported on vCenter 8.x; being phased out on 9.x — see `LB-HAP-000`)* |
| `vds-flb` | `networking=vds`, `loadBalancer=flb` *(new in VCF 9.1; see below)* |
| `nsx-vpc` ⚑ | `networking=nsx-vpc`, either load balancer *(lowest confidence section)* |
| `all` | every combination |

**⚑ Still to do:** rewriting every row in terms of the two settings. Where a row
says "nsx, nsx-alb" it almost always means "networking=nsx, any load balancer",
and a row saying "vds-alb, vds-haproxy" almost always means "networking=vds".
Worth doing once the rows are confirmed, since many will change anyway.

## How each row can be checked

| Key | Meaning |
|---|---|
| `net` | Checkable from a host on the network, no credentials — a network check |
| `api` | Needs vCenter / NSX / ALB credentials — an API check |
| `cfg` | Pure arithmetic on the declared config, nothing contacted — a config check |
| `net+api` | Needs both for a full answer; `net` alone usually gives a partial one |
| `INVASIVE` | Sends traffic that may disturb the network; needs `--invasive` |

Severity: `blocker` (the deployment fails or is unsupported) · `warning` (works,
but degrades or bites later) · `info` (recorded, never gates anything).

---

## Status keys

Each section opens with a generated summary table. The **Status** column says
what this build actually does about the row. That is the backlog, and it lives
next to the requirement rather than in a separate file that would fall out of
date.

| Status | Meaning |
|---|---|
| ✅ `check.id` | Done. The named check covers this row. |
| `ready` | The requirement is settled and nothing new is needed — just the work. The cheapest coverage available. |
| `run location` | Buildable, but only means anything when run from a specific network. **Writing these as ordinary local probes produces a false green** — see [check-types.md](check-types.md). Decide where they should run first. |
| `NSX client` / `ALB client` / `HAProxy API` | Waiting on a client that does not exist yet. `internal/clients/{nsx,alb}` are stubs. |
| `raw socket` / `invasive probe` | Waiting on a kind of probe the tool cannot do yet. Raw sockets need a plan for running without root first — no root is the *normal* case in the field. |
| `confirm first` | The row is flagged ⚑. It cannot be built from this repository; it needs product docs or a lab. |

**`confirm first` does not mean "not done yet".** Those rows are not waiting on
engineering time. Half this matrix is unconfirmed, and that — not how much code
anyone can write — is what limits coverage.

### A ⚑ row may still be implemented

Four rows that are done are also flagged. That looks like it breaks
[ADR-0008](ADR/0008-requirements-matrix-authority.md) rule 3, and it does not. In
each case the check **takes the uncertain part from you instead of deciding it**:
`COM-NTP-002` gets its tolerance from your config rather than a number this tool
invented, `COM-DNS-002` takes its severity from `services.dns.requireReverse`,
`COM-DNS-005` stays a warning exactly as its flag says it should, and
`COM-API-001` only checks the tool's own access and says so.

So the rule is: *if a row is flagged over something the check does not claim,
that flag does not block it.* If the row is flagged over **the very thing the
check would claim**, it stays blocked — and that is the usual case. The test is
"say what the doubt is, then say what the check claims", not "find a way to argue
past the flag". Settled in
[ADR-0015](ADR/0015-flagged-rows-and-version-constants.md).

---

# Meta

<!-- BEGIN GENERATED SUMMARY -->

| ID | Requirement | Severity | ⚑ | Status |
|---|---|---|---|---|
| `MET-001` | Declared topology is supported by this build | blocker |  | ✅ `meta.topology-recognised` |

*1 of 1 implemented.*
<!-- END GENERATED SUMMARY -->

#### `MET-001` · Declared topology is supported by this build
**Topologies** all · **Category** meta · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** `topology:` names a shape this build knows how to grade.
- **From the network:** `cfg`.
- **Remediation:** Set a supported topology. If the environment uses a shape this build does not know, no other pass in the report means anything.
- **Source:** This tool. Not a product requirement.

---

# Common — DNS

<!-- BEGIN GENERATED SUMMARY -->

| ID | Requirement | Severity | ⚑ | Status |
|---|---|---|---|---|
| `COM-DNS-001` | Forward resolution for every management endpoint | blocker |  | ✅ `dns.forward` |
| `COM-DNS-002` | Reverse (PTR) resolution agrees with forward | blocker | ⚑ | ✅ `dns.reverse` |
| `COM-DNS-003` | Declared resolvers answer from the relevant segments | blocker |  | run location |
| `COM-DNS-004` | Supervisor control plane name resolves | warning | ⚑ | confirm first |
| `COM-DNS-005` | Resolvers agree with each other | warning | ⚑ | ✅ `dns.resolver-agreement` |

*3 of 5 implemented.*
<!-- END GENERATED SUMMARY -->

#### `COM-DNS-001` · Forward resolution for every management endpoint
**Topologies** all · **Category** dns · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** Every declared endpoint FQDN (vCenter, NSX Manager, ESXi hosts, ALB controller, registry) resolves, from every declared resolver, to the declared address.
- **From the network:** `net` — A/AAAA query per name per resolver.
- **Remediation:** Create the missing A records. Resolution must work from the management segment, not only from the operator's workstation.
- **Source:** Supervisor / VKS installation prerequisites, networking requirements section.

#### `COM-DNS-002` · Reverse (PTR) resolution agrees with forward
**Topologies** all · **Category** dns · **Severity** blocker ⚑ · **Confidence** MED · **Flag** ⚑
- **Expected:** Every declared endpoint address has a PTR record resolving back to the same FQDN.
- **From the network:** `net`.
- **Remediation:** Create matching PTR records in the reverse zone.
- **Source:** Supervisor installation prerequisites, DNS requirements.
- **⚑ Confirm:** Whether PTR is a *hard* requirement for Supervisor enablement in VCF 9 or only strongly recommended. This drives blocker-vs-warning, and getting it wrong in either direction is expensive: as a warning it lets a broken deployment through, as a blocker it stops a working one. The config exposes `services.dns.requireReverse` so the site decides until this is settled.

#### `COM-DNS-003` · Declared resolvers answer from the relevant segments
**Topologies** all · **Category** dns · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** Each declared DNS server answers on 53/udp and 53/tcp from the management segment and from each workload segment.
- **From the network:** `net` — has to be run from a host on each segment. A pass from the jump host says nothing about the workload network, which is exactly why the report records which host the probes ran from.
- **Remediation:** Open 53/udp+tcp, or place a resolver reachable from each segment.
- **Source:** Generic. Also implied by Supervisor networking prerequisites.

#### `COM-DNS-004` · Supervisor control plane name resolves
**Topologies** all · **Category** dns · **Severity** warning ⚑ · **Confidence** LOW · **Flag** ⚑
- **Expected:** If the Supervisor API endpoint is addressed by FQDN, that name resolves to the declared VIP.
- **From the network:** `net`.
- **Remediation:** Pre-create the A record for the Supervisor API endpoint.
- **Source:** Supervisor enablement workflow, control plane network settings.
- **⚑ Confirm:** Whether VCF 9 requires a *pre-created* DNS record for the Supervisor VIP, creates one itself, or is happy with an IP-only endpoint. Currently unknown, which is why this is a warning.

#### `COM-DNS-005` · Resolvers agree with each other
**Topologies** all · **Category** dns · **Severity** warning · **Confidence** HIGH · **Flag** ⚑
- **Expected:** All declared resolvers return the same answer for the same name.
- **From the network:** `net`.
- **Remediation:** Reconcile split-horizon zones. Disagreeing resolvers produce failures that move between reboots.
- **Source:** None — field heuristic.
- **⚑ Confirm:** This is **not a documented product requirement**; it is a diagnostic that catches a real and painful failure. Flagged so it is not mistaken for a sourced requirement. Keep it as a warning.

---

# Common — NTP and time

<!-- BEGIN GENERATED SUMMARY -->

| ID | Requirement | Severity | ⚑ | Status |
|---|---|---|---|---|
| `COM-NTP-001` | Declared NTP sources are reachable and answer | blocker |  | ✅ `ntp.reachable` |
| `COM-NTP-002` | Clock skew is within tolerance | blocker | ⚑ | ✅ `ntp.skew` |
| `COM-NTP-003` | ESXi hosts have NTP configured and running | blocker | ⚑ | confirm first |
| `COM-NTP-004` | vCenter appliance is time-synced | blocker | ⚑ | confirm first |
| `COM-NTP-005` | NSX Manager is time-synced | blocker | ⚑ | confirm first |

*2 of 5 implemented.*
<!-- END GENERATED SUMMARY -->

#### `COM-NTP-001` · Declared NTP sources are reachable and answer
**Topologies** all · **Category** ntp · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** Each declared NTP server answers an SNTP query on 123/udp.
- **From the network:** `net` — a real SNTP query. **Not** ICMP and **not** curl: the previous test-coverage document specified "Ping/curl NTP", which tests neither the NTP service nor the protocol. A host can ping perfectly and serve no time at all.
- **Remediation:** Open 123/udp to the declared sources, or declare sources that are actually reachable.
- **Source:** Supervisor installation prerequisites, NTP requirements.

#### `COM-NTP-002` · Clock skew is within tolerance
**Topologies** all · **Category** ntp · **Severity** blocker · **Confidence** LOW ⚑ · **Flag** ⚑
- **Expected:** Offset between this host, the declared NTP sources, vCenter and the ESXi hosts is within `services.ntp.maxSkewSeconds`.
- **From the network:** `net` for source offset; `api` for vCenter and host clocks.
- **Remediation:** Point every component at the same stratum. Certificate validation and Kubernetes token handling both fail in confusing ways when clocks disagree.
- **Source:** Unknown.
- **⚑ Confirm:** **The threshold is the problem.** The previous test-coverage document stated 30 seconds as fact. No documented product threshold is known to this document, and 30s appears to be a field heuristic. It is configurable rather than hardcoded so the tool does not present an unsourced number as a product requirement. Confirm whether a documented tolerance exists; if not, keep it configurable and say so in the output.

#### `COM-NTP-003` · ESXi hosts have NTP configured and running
**Topologies** all · **Category** ntp · **Severity** blocker · **Confidence** MED · **Flag** ⚑
- **Expected:** Every host in the target cluster has the NTP client enabled, a start policy of start-with-host, and configured servers matching the declared list.
- **From the network:** `api` (vCenter).
- **Remediation:** Configure NTP on each host and set the service to start with the host.
- **Source:** vSphere host configuration prerequisites for Supervisor enablement.
- **⚑ Confirm:** Whether VCF 9 enforces this at enablement time or merely recommends it, and whether PTP is now an accepted alternative.

#### `COM-NTP-004` · vCenter appliance is time-synced
**Topologies** all · **Category** ntp · **Severity** blocker · **Confidence** MED · **Flag** ⚑
- **Expected:** vCenter is synchronised to the declared NTP sources.
- **From the network:** `api`.
- **Remediation:** Configure NTP on the appliance.
- **Source:** vCenter deployment prerequisites.
- **⚑ Confirm:** Exact requirement wording for VCF 9.

#### `COM-NTP-005` · NSX Manager is time-synced
**Topologies** `nsx`, `nsx-alb`, `nsx-vpc` · **Category** ntp · **Severity** blocker · **Confidence** MED · **Flag** ⚑
- **Expected:** NSX Manager nodes are synchronised to the declared sources and agree with vCenter.
- **From the network:** `api`.
- **Remediation:** Configure NTP on the NSX Manager cluster.
- **Source:** NSX installation prerequisites.
- **⚑ Confirm:** Whether NSX-to-vCenter skew has a documented tolerance distinct from `COM-NTP-002`.

---

# Common — firewall and ports

<!-- BEGIN GENERATED SUMMARY -->

| ID | Requirement | Severity | ⚑ | Status |
|---|---|---|---|---|
| `COM-FW-001` | vCenter management port reachable | blocker |  | ✅ `tcp.port-open` |
| `COM-FW-002` | NSX Manager management port reachable | blocker |  | ✅ `tcp.port-open` |
| `COM-FW-003` | ESXi host management ports reachable | blocker | ⚑ | confirm first |
| `COM-FW-004` | Supervisor API endpoint path is open | blocker | ⚑ | confirm first |
| `COM-FW-005` | Image registry reachable from the workload network | blocker | ⚑ | confirm first |
| `COM-FW-006` | Declared egress proxy is reachable and consistent | warning | ⚑ | confirm first |
| `COM-FW-007` | Full inter-component port matrix | blocker | ⚑ | confirm first |

*2 of 7 implemented.*
<!-- END GENERATED SUMMARY -->

#### `COM-FW-001` · vCenter management port reachable
**Topologies** all · **Category** firewall · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** TCP 443 to vCenter accepts a connection from the management segment.
- **From the network:** `net` — three outcomes. "Refused" proves the host is reachable with nothing listening; silence proves nothing and must be reported as unknown, never as a failure.
- **Remediation:** Open TCP 443 from the management segment to vCenter.
- **Source:** VMware Ports and Protocols for the exact VCF/VKS version.

#### `COM-FW-002` · NSX Manager management port reachable
**Topologies** `nsx`, `nsx-alb`, `nsx-vpc` · **Category** firewall · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** TCP 443 to NSX Manager accepts a connection from the management segment.
- **From the network:** `net`.
- **Remediation:** Open TCP 443 from the management segment to the NSX Manager cluster VIP and to each node.
- **Source:** VMware Ports and Protocols.

#### `COM-FW-003` · ESXi host management ports reachable
**Topologies** all · **Category** firewall · **Severity** blocker · **Confidence** MED · **Flag** ⚑
- **Expected:** TCP 443 and 902 to each ESXi host accept connections from the management segment.
- **From the network:** `net`.
- **Remediation:** Open the host management ports.
- **Source:** VMware Ports and Protocols.
- **⚑ Confirm:** Whether 902 is still required in the VCF 9 Supervisor path, or is legacy.

#### `COM-FW-004` · Supervisor API endpoint path is open
**Topologies** all · **Category** firewall · **Severity** blocker · **Confidence** MED · **Flag** ⚑
- **Expected:** TCP 6443 (and 443 for the kubectl plugin download) to the Supervisor VIP is permitted from wherever operators and CI will connect.
- **From the network:** Preflight: `net` — can only verify the *path and firewall policy*, since nothing is listening yet. Verify: `net` — full check.
- **Remediation:** Open the operator-to-VIP path.
- **Source:** VMware Ports and Protocols; Supervisor access documentation.
- **⚑ Confirm:** The exact port set VCF 9 exposes on the Supervisor VIP.

#### `COM-FW-005` · Image registry reachable from the workload network
**Topologies** all · **Category** firewall · **Severity** blocker · **Confidence** MED · **Flag** ⚑
- **Expected:** The declared registry answers on 443 **from the workload segment**, or the declared proxy does.
- **From the network:** `net`, and it must be probed from the workload segment — this is a common false pass when the check is run from a jump host with wider access.
- **Remediation:** Open the workload-to-registry path, or configure the proxy.
- **Source:** VKS / TKr image source prerequisites.
- **⚑ Confirm:** What VCF 9 actually pulls, from where, and whether the depot model changed the answer. This tool never reaches the internet itself; it only probes what the config declares.

#### `COM-FW-006` · Declared egress proxy is reachable and consistent
**Topologies** all · **Category** firewall · **Severity** warning · **Confidence** MED · **Flag** ⚑
- **Expected:** The declared proxy answers, and `noProxy` covers every internal range and domain the cluster must reach directly.
- **From the network:** `net` + `cfg`.
- **Remediation:** Add internal CIDRs and the local domain to `noProxy`. A proxy that intercepts vCenter or NSX traffic breaks certificate validation.
- **Source:** Proxy configuration guidance for Supervisor / VKS.
- **⚑ Confirm:** The supported proxy configuration surface in VCF 9.

#### `COM-FW-007` · Full inter-component port matrix
**Topologies** all · **Category** firewall · **Severity** blocker · **Confidence** LOW ⚑ · **Flag** ⚑
- **Expected:** Every port in the official matrix for the exact VCF/VKS version is open on the paths it applies to.
- **From the network:** `net`.
- **Remediation:** Consult the official matrix for the version being deployed.
- **Source:** VMware Ports and Protocols tool, filtered to the exact product version.
- **⚑ Confirm — and note the design decision:** **This matrix deliberately does not enumerate the port list.** It is long, version-specific, and precisely the kind of content that is dangerous to reproduce from memory: a plausible-looking but wrong port list would be actively harmful, because it would be believed. The tool should consume a per-version port list as *data* — a YAML file the user supplies or that ships alongside a release — rather than hardcoding ports in Go. `COM-FW-001` to `COM-FW-005` cover the handful of paths confident enough to name individually.

---

# Common — certificates

<!-- BEGIN GENERATED SUMMARY -->

| ID | Requirement | Severity | ⚑ | Status |
|---|---|---|---|---|
| `COM-CRT-001` | vCenter certificate chain validates | blocker |  | ✅ `tls.chain` |
| `COM-CRT-002` | NSX Manager certificate chain validates | blocker |  | ✅ `tls.chain` |
| `COM-CRT-003` | No certificate expires inside the deployment window | warning |  | ✅ `tls.expiry` |
| `COM-CRT-004` | Private CA is trusted where one is used | blocker | ⚑ | confirm first |

*3 of 4 implemented.*
<!-- END GENERATED SUMMARY -->

#### `COM-CRT-001` · vCenter certificate chain validates
**Topologies** all · **Category** certs · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** The vCenter TLS chain validates to a trusted root, and the presented SAN covers the FQDN (and IP, if addressed by IP).
- **From the network:** `net` — TLS handshake and chain inspection, no credentials needed.
- **Remediation:** Replace the certificate or install the issuing CA. A SAN that omits the address actually used is the common failure.
- **Source:** Certificate requirements in the Supervisor prerequisites.

#### `COM-CRT-002` · NSX Manager certificate chain validates
**Topologies** `nsx`, `nsx-alb`, `nsx-vpc` · **Category** certs · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** As `COM-CRT-001`, for the NSX Manager cluster VIP and each node.
- **From the network:** `net`.
- **Remediation:** Replace the certificate or install the issuing CA.
- **Source:** NSX certificate management documentation.

#### `COM-CRT-003` · No certificate expires inside the deployment window
**Topologies** all · **Category** certs · **Severity** warning · **Confidence** HIGH · **Flag** —
- **Expected:** No endpoint certificate expires within a configurable horizon (default 90 days).
- **From the network:** `net`.
- **Remediation:** Renew before deploying. A certificate that expires mid-lifecycle produces failures far from their cause.
- **Source:** Generic.

#### `COM-CRT-004` · Private CA is trusted where one is used
**Topologies** all · **Category** certs · **Severity** blocker · **Confidence** LOW ⚑ · **Flag** ⚑
- **Expected:** Where endpoints use a private CA, that CA is available to be injected into the Supervisor and workload cluster trust stores.
- **From the network:** `net` for chain discovery; `api` for what is configured.
- **Remediation:** Supply the CA bundle through the supported mechanism.
- **Source:** Trusted-certificate / TLS trust configuration for Supervisor and VKS clusters.
- **⚑ Confirm:** The **name and shape of the mechanism in VCF 9**. This changed across the TKGS generations and this document should not guess at it.

---

# Common — MTU

<!-- BEGIN GENERATED SUMMARY -->

| ID | Requirement | Severity | ⚑ | Status |
|---|---|---|---|---|
| `COM-MTU-001` | Underlay MTU meets the overlay minimum | blocker | ⚑ | confirm first |
| `COM-MTU-002` | Declared segment MTUs are mutually consistent | warning |  | ready |
| `COM-MTU-003` | VDS MTU meets the requirement | blocker |  | ✅ `vc.vds-mtu` |
| `COM-MTU-004` | NSX uplink profile MTU is consistent with the VDS | blocker | ⚑ | confirm first |
| `COM-MTU-005` | Path MTU verified end to end · **INVASIVE** | blocker |  | invasive probe |

*1 of 5 implemented.*
<!-- END GENERATED SUMMARY -->

#### `COM-MTU-001` · Underlay MTU meets the overlay minimum
**Topologies** `nsx`, `nsx-alb`, `nsx-vpc` · **Category** mtu · **Severity** blocker · **Confidence** MED ⚑ · **Flag** ⚑
- **Expected:** Every hop carrying Geneve traffic supports at least the required MTU end to end.
- **From the network:** `net` + `INVASIVE` for true path verification; `api` for configured values.
- **Remediation:** Raise MTU on the physical underlay, the VDS and the NSX uplink profile together. Raising one and not the others produces intermittent, size-dependent failures that look like application bugs.
- **Source:** NSX installation prerequisites, transport network MTU requirements.
- **⚑ Confirm the number.** 1600 is the long-standing Geneve minimum and 9000 is commonly recommended in practice, but the VCF 9 requirement is not confirmed here. `nsx.overlayMTU` is configurable, so the tool checks against the value the site declared rather than a number this document invented.

#### `COM-MTU-002` · Declared segment MTUs are mutually consistent
**Topologies** all · **Category** mtu · **Severity** warning · **Confidence** HIGH · **Flag** —
- **Expected:** No declared segment has an MTU lower than something that must traverse it.
- **From the network:** `cfg`.
- **Remediation:** Reconcile the declared MTUs before deploying.
- **Source:** Arithmetic.

#### `COM-MTU-003` · VDS MTU meets the requirement
**Topologies** all · **Category** mtu · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** The declared VDS is configured with an MTU at least equal to the topology's minimum.
- **From the network:** `api` (vCenter).
- **Remediation:** Raise the VDS MTU. Note this is a fabric-wide change and needs the physical underlay to match first.
- **Source:** NSX and Supervisor networking prerequisites.

#### `COM-MTU-004` · NSX uplink profile MTU is consistent with the VDS
**Topologies** `nsx`, `nsx-alb`, `nsx-vpc` · **Category** mtu · **Severity** blocker · **Confidence** MED · **Flag** ⚑
- **Expected:** The uplink profile MTU matches or is below the VDS and physical MTU, and at or above the overlay minimum.
- **From the network:** `api` (NSX).
- **Remediation:** Align the uplink profile with the transport network.
- **Source:** NSX transport node configuration.
- **⚑ Confirm:** Whether VCF 9 still exposes uplink-profile MTU as an independently-settable value, or derives it.

#### `COM-MTU-005` · Path MTU verified end to end · **INVASIVE**
**Topologies** all · **Category** mtu · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** A DF-flagged probe of the required size traverses the path without fragmentation.
- **From the network:** `net` + `INVASIVE`.
- **Remediation:** Find the hop that is clamping and raise it.
- **Source:** Generic.
- **Note on invasiveness:** This deliberately emits large DF-flagged packets across production paths. It is gated behind `--invasive`, is skipped-with-reason by default, and the skip appears in the report so nobody mistakes "not run" for "passed". This is the only check currently classified invasive; see the open question on duplicate-IP detection at `COM-ADR-001`.

---

# Common — CIDR and addressing

<!-- BEGIN GENERATED SUMMARY -->

| ID | Requirement | Severity | ⚑ | Status |
|---|---|---|---|---|
| `COM-CID-001` | No declared range overlaps another | blocker |  | ✅ `cidr.overlap` |
| `COM-CID-002` | No declared range collides with existing infrastructure | blocker |  | ✅ `cidr.external-collision` |
| `COM-CID-003` | Cluster-internal ranges do not collide with reachable infrastructure | blocker |  | ✅ `cidr.infra-collision` |
| `COM-CID-004` | Ranges meet minimum prefix sizes | blocker | ⚑ | confirm first |
| `COM-CID-005` | Declared ranges sit inside their own subnet | blocker |  | ✅ `range.containment` |
| `COM-ADR-001` | Declared static ranges are actually free | blocker | ⚑ | confirm first |

*4 of 6 implemented.*
<!-- END GENERATED SUMMARY -->

#### `COM-CID-001` · No declared range overlaps another
**Topologies** all · **Category** cidr · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** Management, workload, frontend, pod, service, ingress and egress ranges are pairwise disjoint.
- **From the network:** `cfg`.
- **Remediation:** Re-plan the address space. Overlaps cannot be fixed after enablement without rebuilding the Supervisor.
- **Source:** Arithmetic, plus the address-planning section of the Supervisor prerequisites.

#### `COM-CID-002` · No declared range collides with existing infrastructure
**Topologies** all · **Category** cidr · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** No declared range overlaps anything in `kubernetes.externalCIDRs`.
- **From the network:** `cfg`.
- **Remediation:** Re-plan, or extend `externalCIDRs` if the list is incomplete.
- **Source:** Arithmetic.
- **Note:** This check is only ever as good as the declared external list. An empty list means the tool can find self-collisions and nothing else — the report should say so rather than passing silently.

#### `COM-CID-003` · Cluster-internal ranges do not collide with reachable infrastructure
**Topologies** all · **Category** cidr · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** Pod and service CIDRs do not overlap the addresses of vCenter, NSX, DNS, NTP, the registry, or any declared management range.
- **From the network:** `cfg`.
- **Remediation:** Re-plan the pod or service CIDR. A collision here makes an infrastructure endpoint permanently unreachable from inside the cluster, and the symptom appears long after enablement.
- **Source:** Arithmetic.

#### `COM-CID-004` · Ranges meet minimum prefix sizes
**Topologies** all · **Category** cidr · **Severity** blocker · **Confidence** LOW ⚑ · **Flag** ⚑
- **Expected:** Each declared range is at least the documented minimum size for its role.
- **From the network:** `cfg`.
- **Remediation:** Widen the range before deploying.
- **Source:** Address-planning tables in the Supervisor / VKS prerequisites.
- **⚑ Confirm every number.** Minimum prefix sizes per role (pod, service, ingress, egress, management, workload) are **not** supplied by this document. They are version-specific and are exactly the kind of value that must not be guessed. Until confirmed, this check should be implemented as data-driven with the minimums supplied by the reviewer, not baked into Go.

#### `COM-CID-005` · Declared ranges sit inside their own subnet
**Topologies** all · **Category** cidr · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** Every declared static address range falls entirely within the CIDR of the network it belongs to.
- **From the network:** `cfg`.
- **Remediation:** Correct the range or the CIDR. A range extending past its subnet boundary yields addresses that are unreachable from the segment they were allocated on.
- **Source:** Arithmetic.
- **Note:** added during implementation. `LB-VIP-001` covers the VIP-specific case the product documentation calls out; this row covers the identical arithmetic for management, workload and SE data ranges, which the matrix previously had no row for. Implemented by `range.containment`.

#### `COM-ADR-001` · Declared static ranges are actually free
**Topologies** all · **Category** ippool · **Severity** blocker · **Confidence** HIGH · **Flag** ⚑
- **Expected:** No address in a declared static range currently answers.
- **From the network:** `net` — ARP on the local segment, ICMP/TCP probing otherwise.
- **Remediation:** Choose a genuinely unused range, or reclaim the addresses.
- **Source:** Generic; also the previous test-coverage document's "Duplicate IP Validation".
- **⚑ Open question, not a doc question:** should this be gated behind `--invasive`? It is read-only in effect but sprays probes at addresses that are supposed to be unused and may not be. Absence of a reply also does not prove an address is free (a powered-off host still owns its DHCP reservation). The current inclination is: not invasive, but a positive result is a blocker and a negative result is `unknown`, never a pass. Needs a decision before implementation.

---

# Common — routing and reachability

<!-- BEGIN GENERATED SUMMARY -->

| ID | Requirement | Severity | ⚑ | Status |
|---|---|---|---|---|
| `COM-RTE-001` | Declared gateways respond | blocker |  | raw socket |
| `COM-RTE-002` | Routable ranges are actually routed | blocker |  | run location |
| `COM-RTE-003` | No NAT between the Supervisor and the management plane | blocker | ⚑ | confirm first |

*0 of 3 implemented.*
<!-- END GENERATED SUMMARY -->

#### `COM-RTE-001` · Declared gateways respond
**Topologies** all · **Category** routing · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** Each declared gateway address is live and is inside its declared subnet.
- **From the network:** `net` (ICMP or ARP) + `cfg` for containment.
- **Remediation:** Correct the gateway address, or bring up the SVI.
- **Source:** Generic.

#### `COM-RTE-002` · Routable ranges are actually routed
**Topologies** all · **Category** routing · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** Ranges declared `routable: true` are reachable from outside their own segment and are not blackholed.
- **From the network:** `net` — and where you run it is the whole point. A pass from inside the segment says nothing.
- **Remediation:** Advertise or statically route the range.
- **Source:** Generic; also the ingress/egress advertisement requirements under NSX.

#### `COM-RTE-003` · No NAT between the Supervisor and the management plane
**Topologies** all · **Category** routing · **Severity** blocker · **Confidence** LOW ⚑ · **Flag** ⚑
- **Expected:** Traffic between the Supervisor management network and vCenter/NSX is not NATed.
- **From the network:** `net` (partial — observed source address versus expected) + `api`.
- **Remediation:** Remove the NAT, or re-place the management network.
- **Source:** Supervisor networking prerequisites.
- **⚑ Confirm:** Whether this is a stated requirement in VCF 9 or an inherited assumption. It is believed to matter but the confidence is low and the check is hard to do well.

---

# Common — management plane API and versions

<!-- BEGIN GENERATED SUMMARY -->

| ID | Requirement | Severity | ⚑ | Status |
|---|---|---|---|---|
| `COM-API-001` | vCenter API answers and credentials authenticate | blocker | ⚑ | ✅ `vc.api-reachable` |
| `COM-API-002` | NSX Manager API answers and credentials authenticate | blocker |  | NSX client |
| `INV-VC-001` | Declared datacenter and cluster exist | blocker |  | ✅ `vc.cluster-exists` |
| `INV-VC-002` | Declared distributed switch exists | blocker |  | ✅ `vc.vds-exists` |
| `COM-VER-001` | vCenter and ESXi versions are supported | blocker | ⚑ | confirm first |
| `COM-VER-002` | NSX version is supported | blocker | ⚑ | confirm first |

*3 of 6 implemented.*
<!-- END GENERATED SUMMARY -->

#### `COM-API-001` · vCenter API answers and credentials authenticate
**Topologies** all · **Category** reachability · **Severity** blocker · **Confidence** HIGH · **Flag** ⚑
- **Expected:** The vCenter API responds and the supplied credentials authenticate with sufficient read privileges.
- **From the network:** `api`.
- **Remediation:** Correct the credentials or grant the required read role.
- **Source:** Required privileges for Supervisor enablement.
- **⚑ Confirm:** The **exact privilege set** required. This tool needs only read privileges for its own work, but the *deployment* account needs more, and preflight should arguably check the deployment account's privileges rather than its own. That is a scope question worth deciding: it is a real and common failure and it is not a networking one.

#### `COM-API-002` · NSX Manager API answers and credentials authenticate
**Topologies** `nsx`, `nsx-alb`, `nsx-vpc` · **Category** reachability · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** The NSX Policy API responds and the supplied credentials authenticate.
- **From the network:** `api`.
- **Remediation:** Correct the credentials or grant a read role.
- **Source:** NSX API documentation.

#### `INV-VC-001` · Declared datacenter and cluster exist
**Topologies** all · **Category** inventory · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** The datacenter and cluster named in the config exist in vCenter.
- **From the network:** `api`.
- **Remediation:** Correct `vsphere.datacenter` / `vsphere.cluster`, or create the cluster.
- **Source:** Trivially true — you cannot enable a Supervisor on a cluster that does not exist.
- **Note:** added during implementation; the matrix had no row for object existence despite several checks depending on it. A typo here makes every other inventory check inspect the wrong object, so it is a blocker rather than a convenience. Implemented by `vc.cluster-exists`, which also lists the clusters that *do* exist — "not found" alone is a guessing game.

#### `INV-VC-002` · Declared distributed switch exists
**Topologies** all · **Category** inventory · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** The VDS named in the config exists in vCenter.
- **From the network:** `api`.
- **Remediation:** Correct `vsphere.distributedSwitch`, or create the switch.
- **Source:** As `INV-VC-001`.
- **Note:** added during implementation. Implemented by `vc.vds-exists`. Useful under NSX too, not only VDS topologies — host uplinks still ride a switch.

#### `COM-VER-001` · vCenter and ESXi versions are supported
**Topologies** all · **Category** inventory · **Severity** blocker · **Confidence** LOW ⚑ · **Flag** ⚑
- **Expected:** vCenter and host builds are within the supported set for the VKS release being deployed.
- **From the network:** `api`.
- **Remediation:** Upgrade to a supported build.
- **Source:** Product interoperability matrix.
- **⚑ Confirm — and note the design constraint:** the interoperability matrix changes independently of any release, so **this must never be hardcoded**. It has to be supplied as data the user can update, or the check must report the observed versions as `info` and let a human judge. Reporting a stale hardcoded table as if it were current would be worse than not checking at all.

#### `COM-VER-002` · NSX version is supported
**Topologies** `nsx`, `nsx-alb`, `nsx-vpc` · **Category** inventory · **Severity** blocker · **Confidence** LOW ⚑ · **Flag** ⚑
- **Expected:** The NSX build is within the supported set for the VKS release.
- **From the network:** `api`.
- **Remediation:** Upgrade to a supported build.
- **Source:** Product interoperability matrix.
- **⚑ Confirm:** As `COM-VER-001`. Same data-not-code constraint.

---

# Supervisor management network

<!-- BEGIN GENERATED SUMMARY -->

| ID | Requirement | Severity | ⚑ | Status |
|---|---|---|---|---|
| `SUP-MGT-001` | Management network has enough consecutive free addresses | blocker | ⚑ | confirm first |
| `SUP-MGT-002` | Management network reaches the management plane | blocker |  | run location |
| `SUP-MGT-003` | Static range does not overlap a DHCP scope | blocker | ⚑ | confirm first |
| `SUP-MGT-004` | Supervisor API VIP is free and correctly placed | blocker | ⚑ | confirm first |

*0 of 4 implemented.*
<!-- END GENERATED SUMMARY -->

#### `SUP-MGT-001` · Management network has enough consecutive free addresses
**Topologies** all · **Category** ippool · **Severity** blocker · **Confidence** MED ⚑ · **Flag** ⚑
- **Expected:** The declared management range contains the required number of *consecutive* free static addresses.
- **From the network:** `cfg` for contiguity and count; `net` for actually-free (`COM-ADR-001`).
- **Remediation:** Reserve a contiguous block. Non-contiguous addresses are not accepted.
- **Source:** Supervisor control plane network requirements.
- **⚑ Confirm the count.** The long-standing rule is **five consecutive addresses** — one floating/VIP, three control plane VMs, one spare for rolling upgrade. That is a vSphere-with-Tanzu-era number carried forward, and whether VCF 9 still requires exactly five is unconfirmed. Do not hardcode 5.

#### `SUP-MGT-002` · Management network reaches the management plane
**Topologies** all · **Category** routing · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** From the management segment, vCenter, NSX Manager (where applicable), DNS and NTP are all reachable.
- **From the network:** `net`, run from the management segment.
- **Remediation:** Fix routing or firewall policy on the management segment.
- **Source:** Supervisor networking prerequisites.

#### `SUP-MGT-003` · Static range does not overlap a DHCP scope
**Topologies** all · **Category** ippool · **Severity** blocker · **Confidence** HIGH · **Flag** ⚑
- **Expected:** No declared static range falls inside an active DHCP scope.
- **From the network:** `cfg` against declared scopes; `net` can only detect it indirectly.
- **Remediation:** Exclude the static range from the DHCP scope.
- **Source:** Generic.
- **⚑ Gap in the config schema:** the config has no field for declaring DHCP scopes, so this check currently cannot be implemented as stated. Either add `networks[].dhcpScopes` or drop the check. Flagged rather than silently omitted.

#### `SUP-MGT-004` · Supervisor API VIP is free and correctly placed
**Topologies** all · **Category** ippool · **Severity** blocker · **Confidence** MED · **Flag** ⚑
- **Expected:** The declared API VIP is inside the correct subnet for the topology, and is currently unused.
- **From the network:** `cfg` + `net`.
- **Remediation:** Choose a free address in the correct range.
- **Source:** Supervisor enablement, control plane settings.
- **⚑ Confirm:** *Which* subnet the API VIP belongs to differs by topology (management network under VDS; ingress range under NSX). Confirm per topology before implementing containment logic — getting this wrong produces a confident, wrong blocker.

---

# NSX topologies (`nsx`, `nsx-alb`)

<!-- BEGIN GENERATED SUMMARY -->

| ID | Requirement | Severity | ⚑ | Status |
|---|---|---|---|---|
| `NSX-T0-001` | Tier-0 gateway exists | blocker |  | NSX client |
| `NSX-T0-002` | Tier-0 has working north-south connectivity | blocker |  | NSX client |
| `NSX-T0-003` | Tier-0 HA mode and placement are appropriate | warning | ⚑ | confirm first |
| `NSX-EDG-001` | Edge cluster exists and is healthy | blocker | ⚑ | confirm first |
| `NSX-TZ-001` | Overlay transport zone covers the cluster | blocker |  | NSX client |
| `NSX-TZ-002` | Every host in the cluster is prepared for NSX | blocker |  | NSX client |
| `NSX-ING-001` | Ingress range is routable and unallocated | blocker |  | NSX client |
| `NSX-ING-002` | Ingress range is large enough | warning | ⚑ | confirm first |
| `NSX-EGR-001` | Egress range is routable and unallocated | blocker |  | NSX client |
| `NSX-EGR-002` | Egress range is large enough for the expected namespace count | warning | ⚑ | confirm first |
| `NSX-POD-001` | Pod / namespace IP block is valid and sized | blocker | ⚑ | confirm first |
| `NSX-DFW-001` | Distributed firewall does not block Supervisor traffic | warning | ⚑ | confirm first |

*0 of 12 implemented.*
<!-- END GENERATED SUMMARY -->

#### `NSX-T0-001` · Tier-0 gateway exists
**Topologies** `nsx`, `nsx-alb` · **Category** inventory · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** The declared Tier-0 gateway exists in NSX.
- **From the network:** `api`.
- **Remediation:** Create the Tier-0, or correct the name in the config.
- **Source:** Supervisor-on-NSX prerequisites.

#### `NSX-T0-002` · Tier-0 has working north-south connectivity
**Topologies** `nsx`, `nsx-alb` · **Category** routing · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** The Tier-0 has at least one up uplink, and BGP peers are established or static routes are present.
- **From the network:** `api` for state; `net` for a reachability sanity check from outside.
- **Remediation:** Bring up the uplink or fix the peering. A Tier-0 that exists but is not routing produces a Supervisor that enables and is then unreachable.
- **Source:** NSX Tier-0 configuration; Supervisor-on-NSX prerequisites.

#### `NSX-T0-003` · Tier-0 HA mode and placement are appropriate
**Topologies** `nsx`, `nsx-alb` · **Category** inventory · **Severity** warning · **Confidence** LOW ⚑ · **Flag** ⚑
- **Expected:** The Tier-0 HA mode is supported for the intended use.
- **From the network:** `api`.
- **Remediation:** Reconfigure the Tier-0.
- **Source:** NSX Tier-0 documentation.
- **⚑ Confirm:** Whether VCF 9 constrains Tier-0 HA mode for Supervisor at all. May not be a requirement — if not, drop the row rather than shipping a check nobody asked for.

#### `NSX-EDG-001` · Edge cluster exists and is healthy
**Topologies** `nsx`, `nsx-alb` · **Category** inventory · **Severity** blocker · **Confidence** MED · **Flag** ⚑
- **Expected:** The declared edge cluster exists, its nodes are up, and the node form factor meets the requirement.
- **From the network:** `api`.
- **Remediation:** Deploy or repair the edge cluster; redeploy edge nodes at the required size.
- **Source:** NSX Edge sizing for Supervisor.
- **⚑ Confirm the form factor.** The historical requirement was Large edge nodes for Supervisor. Unconfirmed for VCF 9. Do not hardcode a size.

#### `NSX-TZ-001` · Overlay transport zone covers the cluster
**Topologies** `nsx`, `nsx-alb` · **Category** inventory · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** The declared overlay transport zone exists and is attached to the transport nodes backing the target vSphere cluster.
- **From the network:** `api`.
- **Remediation:** Attach the transport zone.
- **Source:** NSX host preparation.

#### `NSX-TZ-002` · Every host in the cluster is prepared for NSX
**Topologies** `nsx`, `nsx-alb` · **Category** inventory · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** Every host in the target cluster is a configured, healthy transport node.
- **From the network:** `api`.
- **Remediation:** Prepare the remaining hosts. A partially-prepared cluster fails enablement in ways that point at the wrong component.
- **Source:** NSX host preparation.

#### `NSX-ING-001` · Ingress range is routable and unallocated
**Topologies** `nsx`, `nsx-alb` · **Category** ippool · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** The declared ingress CIDR is routable from outside, advertised by the Tier-0, and not already allocated in NSX.
- **From the network:** `api` + `net`.
- **Remediation:** Choose a free range and advertise it.
- **Source:** Supervisor-on-NSX address planning.

#### `NSX-ING-002` · Ingress range is large enough
**Topologies** `nsx`, `nsx-alb` · **Category** ippool · **Severity** warning · **Confidence** LOW ⚑ · **Flag** ⚑
- **Expected:** Ingress range size covers the expected number of load balancer services plus headroom.
- **From the network:** `cfg`.
- **Remediation:** Widen the ingress range before enabling. Growing it afterwards is disruptive.
- **Source:** Supervisor-on-NSX address planning.
- **⚑ Confirm the ratio.** How many ingress addresses a Supervisor consumes at rest, and per LB service, is not supplied by this document. Data-driven, not hardcoded.

#### `NSX-EGR-001` · Egress range is routable and unallocated
**Topologies** `nsx`, `nsx-alb` · **Category** ippool · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** The declared egress CIDR is routable, advertised, and not already allocated.
- **From the network:** `api` + `net`.
- **Remediation:** Choose a free range and advertise it.
- **Source:** Supervisor-on-NSX address planning.

#### `NSX-EGR-002` · Egress range is large enough for the expected namespace count
**Topologies** `nsx`, `nsx-alb` · **Category** ippool · **Severity** warning · **Confidence** MED ⚑ · **Flag** ⚑
- **Expected:** Egress range covers the expected namespace count plus headroom.
- **From the network:** `cfg`.
- **Remediation:** Widen the egress range before enabling.
- **Source:** Supervisor-on-NSX address planning.
- **⚑ Confirm the ratio.** The historical rule is **one SNAT address per namespace**, which makes egress sizing a direct function of `scale.expectedNamespaces`. Unconfirmed for VCF 9, and VPC-based networking may change it entirely.

#### `NSX-POD-001` · Pod / namespace IP block is valid and sized
**Topologies** `nsx`, `nsx-alb` · **Category** cidr · **Severity** blocker · **Confidence** MED · **Flag** ⚑
- **Expected:** The namespace/pod IP block exists in NSX, does not overlap anything declared, and is large enough for the expected namespace and pod count.
- **From the network:** `api` + `cfg`.
- **Remediation:** Create or widen the IP block.
- **Source:** Supervisor-on-NSX address planning.
- **⚑ Confirm:** The per-namespace subnet size NSX carves from this block, which determines how many namespaces a given block supports. Not supplied here.

#### `NSX-DFW-001` · Distributed firewall does not block Supervisor traffic
**Topologies** `nsx`, `nsx-alb` · **Category** firewall · **Severity** warning · **Confidence** LOW ⚑ · **Flag** ⚑
- **Expected:** No DFW rule blocks required Supervisor or workload traffic.
- **From the network:** `api`.
- **Remediation:** Add exclusions.
- **Source:** NSX DFW documentation; Supervisor prerequisites.
- **⚑ Confirm:** Whether this is checkable in any meaningful automated way. Evaluating effective DFW policy against hypothetical future workloads is a hard problem and a naive implementation will produce false confidence. Consider reporting DFW *presence* as `info` rather than attempting a verdict.

---

# VPC-based networking (`nsx-vpc`, VCF 9)

> **Treat this entire section as unverified.** It is the lowest-confidence part
> of the document. The object names may be wrong, the model may be wrong, and
> some rows may describe things that do not exist. Nothing here should be
> implemented from this document — it exists to give the reviewer a starting
> shape to correct, not a specification. If the reviewer's lab contradicts a row,
> the lab is right.

<!-- BEGIN GENERATED SUMMARY -->

| ID | Requirement | Severity | ⚑ | Status |
|---|---|---|---|---|
| `VPC-CFG-001` | VPC connectivity profile exists | blocker | ⚑ | confirm first |
| `VPC-CFG-002` | Transit gateway is configured | blocker | ⚑ | confirm first |
| `VPC-POO-001` | Private IP blocks are defined and sized | blocker | ⚑ | confirm first |
| `VPC-POO-002` | Public / external IP blocks are defined, routable and sized | blocker | ⚑ | confirm first |
| `VPC-RTE-001` | VPC external connectivity works | blocker | ⚑ | confirm first |
| `VPC-MTU-001` | MTU on VPC segments meets the overlay minimum | blocker | ⚑ | confirm first |
| `VPC-SUP-001` | Supervisor VPC prerequisites are met | blocker | ⚑ | confirm first |

*0 of 7 implemented.*
<!-- END GENERATED SUMMARY -->

#### `VPC-CFG-001` · VPC connectivity profile exists
**Topologies** `nsx-vpc` · **Category** inventory · **Severity** blocker · **Confidence** LOW ⚑ · **Flag** ⚑
- **Expected:** The declared VPC connectivity profile exists and is associated with the project the Supervisor will use.
- **From the network:** `api`.
- **Remediation:** Create or correct the profile.
- **Source:** VCF 9 NSX VPC documentation.
- **⚑ Confirm the object model exists as described.**

#### `VPC-CFG-002` · Transit gateway is configured
**Topologies** `nsx-vpc` · **Category** routing · **Severity** blocker · **Confidence** LOW ⚑ · **Flag** ⚑
- **Expected:** A transit gateway provides the intended connectivity mode for the VPC.
- **From the network:** `api`.
- **Remediation:** Configure the transit gateway.
- **Source:** VCF 9 NSX VPC documentation.
- **⚑ Confirm the object model and the connectivity modes available.**

#### `VPC-POO-001` · Private IP blocks are defined and sized
**Topologies** `nsx-vpc` · **Category** ippool · **Severity** blocker · **Confidence** LOW ⚑ · **Flag** ⚑
- **Expected:** Private IP blocks exist, are large enough for the expected namespaces and pods, and do not overlap declared external ranges.
- **From the network:** `api` + `cfg`.
- **Remediation:** Define or widen the blocks.
- **Source:** VCF 9 NSX VPC documentation.
- **⚑ Confirm everything, including whether "private IP block" is the right term.**

#### `VPC-POO-002` · Public / external IP blocks are defined, routable and sized
**Topologies** `nsx-vpc` · **Category** ippool · **Severity** blocker · **Confidence** LOW ⚑ · **Flag** ⚑
- **Expected:** Public or external IP blocks exist, are routable from outside, and cover expected VIP and SNAT consumption.
- **From the network:** `api` + `net` + `cfg`.
- **Remediation:** Define or widen the blocks and advertise them.
- **Source:** VCF 9 NSX VPC documentation.
- **⚑ Confirm everything.** In particular whether ingress/egress remain separate concepts under VPC or are unified.

#### `VPC-RTE-001` · VPC external connectivity works
**Topologies** `nsx-vpc` · **Category** routing · **Severity** blocker · **Confidence** LOW ⚑ · **Flag** ⚑
- **Expected:** Routes for the VPC's external blocks are advertised and reachable from outside.
- **From the network:** `net` + `api`.
- **Remediation:** Fix route advertisement.
- **Source:** VCF 9 NSX VPC documentation.
- **⚑ Confirm.**

#### `VPC-MTU-001` · MTU on VPC segments meets the overlay minimum
**Topologies** `nsx-vpc` · **Category** mtu · **Severity** blocker · **Confidence** LOW ⚑ · **Flag** ⚑
- **Expected:** VPC segments carry at least the required overlay MTU.
- **From the network:** `api`; `net` + `INVASIVE` for path verification.
- **Remediation:** Raise MTU across the underlay and the VPC configuration.
- **Source:** VCF 9 NSX VPC documentation.
- **⚑ Confirm whether MTU is settable per VPC at all, or inherited.** Note the previous test-coverage document specifically called out "MTU Checker for VPC based Supervisor deployments", which suggests the reviewer already considers this important — all the more reason not to guess at it.

#### `VPC-SUP-001` · Supervisor VPC prerequisites are met
**Topologies** `nsx-vpc` · **Category** inventory · **Severity** blocker · **Confidence** LOW ⚑ · **Flag** ⚑
- **Expected:** Whatever project / VPC association the Supervisor requires is in place before enablement.
- **From the network:** `api`.
- **Remediation:** Unknown.
- **Source:** VCF 9 Supervisor-on-VPC enablement documentation.
- **⚑ This row is a placeholder** and is honest about it. There are almost certainly VPC prerequisites this document does not know exist. Expect to add rows here, not just correct them.

---

# VDS-based topologies — common (`vds-alb`, `vds-haproxy`)

<!-- BEGIN GENERATED SUMMARY -->

| ID | Requirement | Severity | ⚑ | Status |
|---|---|---|---|---|
| `VDS-PG-001` | Management portgroup exists with the declared VLAN | blocker |  | ✅ `vc.portgroup-exists` |
| `VDS-PG-002` | Workload portgroup(s) exist | blocker |  | ✅ `vc.portgroup-exists` |
| `VDS-PG-003` | Frontend portgroup exists | blocker |  | ✅ `vc.portgroup-exists` |
| `VDS-PG-004` | Portgroup security policy permits the load balancer | warning | ⚑ | confirm first |
| `VDS-WKL-001` | Workload static range is sized for the expected node count | blocker | ⚑ | confirm first |
| `VDS-WKL-002` | Workload network reaches the management plane | blocker |  | run location |
| `VDS-DHCP-001` | Declared DHCP scope is valid where DHCP is used | warning | ⚑ | confirm first |

*3 of 7 implemented.*
<!-- END GENERATED SUMMARY -->

#### `VDS-PG-001` · Management portgroup exists with the declared VLAN
**Topologies** `vds-alb`, `vds-haproxy` · **Category** inventory · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** The declared management portgroup exists on the declared VDS and carries the declared VLAN.
- **From the network:** `api`.
- **Remediation:** Create the portgroup or correct the config.
- **Source:** Supervisor-on-VDS prerequisites.

#### `VDS-PG-002` · Workload portgroup(s) exist
**Topologies** `vds-alb`, `vds-haproxy` · **Category** inventory · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** Every declared workload portgroup exists on the declared VDS with the declared VLAN.
- **From the network:** `api`.
- **Remediation:** Create the portgroups.
- **Source:** Supervisor-on-VDS prerequisites.

#### `VDS-PG-003` · Frontend portgroup exists
**Topologies** `vds-alb`, `vds-haproxy` · **Category** inventory · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** The declared frontend/VIP portgroup exists on the declared VDS.
- **From the network:** `api`.
- **Remediation:** Create the portgroup.
- **Source:** Supervisor-on-VDS prerequisites.

#### `VDS-PG-004` · Portgroup security policy permits the load balancer
**Topologies** `vds-alb`, `vds-haproxy` · **Category** inventory · **Severity** warning · **Confidence** LOW ⚑ · **Flag** ⚑
- **Expected:** Portgroup security settings (forged transmits, MAC changes, promiscuous mode) are whatever the load balancer requires.
- **From the network:** `api`.
- **Remediation:** Adjust the portgroup security policy.
- **Source:** ALB / HAProxy deployment guidance for vSphere.
- **⚑ Confirm what is actually required, per load balancer.** The requirements differ between ALB Service Engines and HAProxy, and enabling promiscuous mode unnecessarily is a real security cost. Do not build a check that claims one blanket answer.

#### `VDS-WKL-001` · Workload static range is sized for the expected node count
**Topologies** `vds-alb`, `vds-haproxy` · **Category** ippool · **Severity** blocker · **Confidence** MED · **Flag** ⚑
- **Expected:** The workload network's static range covers Supervisor nodes plus expected workload cluster nodes plus headroom.
- **From the network:** `cfg`.
- **Remediation:** Widen the range before enabling.
- **Source:** Supervisor-on-VDS address planning.
- **⚑ Confirm the per-node consumption**, including whether upgrades transiently need extra addresses. Rolling upgrades that run out of addresses are a classic and miserable failure.

#### `VDS-WKL-002` · Workload network reaches the management plane
**Topologies** `vds-alb`, `vds-haproxy` · **Category** routing · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** From the workload segment, vCenter and the load balancer control plane are reachable.
- **From the network:** `net`, run from the workload segment.
- **Remediation:** Fix routing or firewall policy.
- **Source:** Supervisor-on-VDS prerequisites.

#### `VDS-DHCP-001` · Declared DHCP scope is valid where DHCP is used
**Topologies** `vds-alb`, `vds-haproxy` · **Category** ippool · **Severity** warning · **Confidence** MED · **Flag** ⚑
- **Expected:** Where the workload network uses DHCP, a scope exists, is reachable, and does not overlap declared static ranges.
- **From the network:** `net` (DHCP probing) + `cfg`.
- **Remediation:** Correct the scope.
- **Source:** Supervisor-on-VDS network configuration.
- **⚑ Two things to confirm:** whether DHCP is supported for the workload network in VCF 9, and — as with `SUP-MGT-003` — **the config schema has no DHCP fields**, so this cannot currently be implemented as written. A DHCP probe is also arguably invasive: it consumes a lease.

---

# Load balancer — NSX ALB (`vds-alb`, `nsx-alb`)

<!-- BEGIN GENERATED SUMMARY -->

| ID | Requirement | Severity | ⚑ | Status |
|---|---|---|---|---|
| `LB-ALB-001` | Controller is reachable and authenticates | blocker |  | ALB client |
| `LB-ALB-002` | Controller version is compatible | blocker | ⚑ | confirm first |
| `LB-ALB-003` | Controller cluster is healthy | blocker |  | ALB client |
| `LB-ALB-004` | License tier supports the required features | blocker | ⚑ | confirm first |
| `LB-ALB-005` | Cloud is configured and healthy | blocker | ⚑ | confirm first |
| `LB-ALB-006` | Service Engine group exists with capacity | blocker | ⚑ | confirm first |
| `LB-ALB-007` | Controller has DNS and NTP configured | warning |  | ALB client |
| `LB-ALB-008` | Controller is reachable **from the Supervisor management network** | blocker |  | run location |
| `LB-VIP-001` | VIP range sits inside its frontend subnet | blocker |  | ✅ `range.containment` |
| `LB-VIP-002` | VIP range does not overlap other allocations | blocker |  | ready |
| `LB-VIP-003` | VIP network has an allocatable pool in ALB | blocker |  | ALB client |
| `LB-VIP-004` | VIP range is not already allocated | blocker |  | ALB client |
| `LB-VIP-005` | SE data / transit network reaches the workload network | blocker |  | run location |
| `LB-VIP-006` | VIP range is large enough | warning | ⚑ | confirm first |

*1 of 14 implemented.*
<!-- END GENERATED SUMMARY -->

#### `LB-ALB-001` · Controller is reachable and authenticates
**Topologies** `vds-alb`, `nsx-alb` · **Category** reachability · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** The ALB controller answers on 443 and the supplied credentials authenticate.
- **From the network:** `net` for reachability, `api` for authentication.
- **Remediation:** Fix reachability or credentials.
- **Source:** ALB deployment prerequisites.
- **Note:** the previous test-coverage document listed exactly this ("AVI Controller Reachability Login and SW Version"), which is kept and split into `LB-ALB-001` and `LB-ALB-002` because reachability, authentication and version are three different findings with three different remediations.

#### `LB-ALB-002` · Controller version is compatible
**Topologies** `vds-alb`, `nsx-alb` · **Category** inventory · **Severity** blocker · **Confidence** LOW ⚑ · **Flag** ⚑
- **Expected:** The controller version is supported with the vCenter and VKS versions being deployed.
- **From the network:** `api`.
- **Remediation:** Upgrade or downgrade the controller.
- **Source:** Product interoperability matrix.
- **⚑ Confirm:** As `COM-VER-001` — data-driven, never hardcoded.

#### `LB-ALB-003` · Controller cluster is healthy
**Topologies** `vds-alb`, `nsx-alb` · **Category** inventory · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** Where clustered, all controller nodes are up and the cluster has quorum.
- **From the network:** `api`; `net` can check each declared node address.
- **Remediation:** Repair the cluster before deploying.
- **Source:** ALB cluster documentation.

#### `LB-ALB-004` · License tier supports the required features
**Topologies** `vds-alb`, `nsx-alb` · **Category** inventory · **Severity** blocker · **Confidence** LOW ⚑ · **Flag** ⚑
- **Expected:** The controller's license tier includes what VKS requires.
- **From the network:** `api`.
- **Remediation:** Apply the correct license.
- **Source:** ALB licensing documentation; VKS load balancer requirements.
- **⚑ Confirm both the tier names and which tier VKS requires** in the VCF 9 generation. Licensing terminology here has changed more than once and this document should not guess.

#### `LB-ALB-005` · Cloud is configured and healthy
**Topologies** `vds-alb`, `nsx-alb` · **Category** inventory · **Severity** blocker · **Confidence** MED · **Flag** ⚑
- **Expected:** The declared cloud exists, is of the right type for the topology, and is in a healthy state.
- **From the network:** `api`.
- **Remediation:** Fix the cloud configuration.
- **Source:** ALB cloud configuration for vSphere / NSX.
- **⚑ Confirm:** Which cloud type is required per topology — a vCenter cloud for `vds-alb` versus an NSX-T cloud for `nsx-alb`. Claiming the wrong one produces a confident, false blocker.

#### `LB-ALB-006` · Service Engine group exists with capacity
**Topologies** `vds-alb`, `nsx-alb` · **Category** inventory · **Severity** blocker · **Confidence** MED · **Flag** ⚑
- **Expected:** The declared SE group exists and can place the required Service Engines.
- **From the network:** `api`.
- **Remediation:** Create or resize the SE group.
- **Source:** ALB Service Engine documentation.
- **⚑ Confirm:** How many SEs a VKS deployment requires, and whether VKS creates its own SE group.

#### `LB-ALB-007` · Controller has DNS and NTP configured
**Topologies** `vds-alb`, `nsx-alb` · **Category** ntp · **Severity** warning · **Confidence** MED · **Flag** —
- **Expected:** The controller's own DNS and NTP settings are populated and working.
- **From the network:** `api`.
- **Remediation:** Configure DNS and NTP on the controller.
- **Source:** ALB deployment prerequisites.

#### `LB-ALB-008` · Controller is reachable **from the Supervisor management network**
**Topologies** `vds-alb`, `nsx-alb` · **Category** reachability · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** The controller answers on 443 from the management segment, not merely from the operator's jump host.
- **From the network:** `net`, run from the management segment.
- **Remediation:** Open the management-to-controller path.
- **Source:** Supervisor load balancer configuration prerequisites.
- **Note:** this is a separate row from `LB-ALB-001` on purpose. Where the check runs from is the difference between catching this in preflight and producing a green report followed by a failed deployment.

#### `LB-VIP-001` · VIP range sits inside its frontend subnet
**Topologies** `vds-alb`, `nsx-alb`, `vds-flb` · **Category** cidr · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** The declared VIP range is contained within the frontend network's CIDR.
- **From the network:** `cfg`.
- **Remediation:** Correct the VIP range or the frontend CIDR.
- **Source:** Arithmetic.

#### `LB-VIP-002` · VIP range does not overlap other allocations
**Topologies** `vds-alb`, `nsx-alb`, `vds-flb` · **Category** cidr · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** The VIP range does not overlap the SE data range, any static node range, or a DHCP scope.
- **From the network:** `cfg`.
- **Remediation:** Re-plan the frontend address space.
- **Source:** Arithmetic. (DHCP portion blocked on the schema gap noted in `SUP-MGT-003`.)

#### `LB-VIP-003` · VIP network has an allocatable pool in ALB
**Topologies** `vds-alb`, `nsx-alb` · **Category** ippool · **Severity** blocker · **Confidence** MED · **Flag** —
- **Expected:** The VIP network is configured in the controller with a static pool covering the declared range.
- **From the network:** `api`.
- **Remediation:** Configure the network and pool in the controller.
- **Source:** ALB IPAM configuration.

#### `LB-VIP-004` · VIP range is not already allocated
**Topologies** `vds-alb`, `nsx-alb` · **Category** ippool · **Severity** blocker · **Confidence** MED · **Flag** —
- **Expected:** No address in the declared VIP range is already allocated in ALB IPAM or in use on the network.
- **From the network:** `api` + `net` (`COM-ADR-001`).
- **Remediation:** Choose a free range.
- **Source:** ALB IPAM configuration.

#### `LB-VIP-005` · SE data / transit network reaches the workload network
**Topologies** `vds-alb`, `nsx-alb`, `vds-flb` · **Category** routing · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** The Service Engine data network (ALB) or transit network (FLB two-arm mode) can route to the workload network where the backend pods and nodes live.
- **From the network:** `net`, run from the SE data / transit segment.
- **Remediation:** Fix routing between the SE data / transit and workload segments.
- **Source:** ALB Service Engine network design; FLB architecture (transit network forwards traffic to workload IP pool members).
- **Note:** for FLB, this row only applies in `two-arm` mode — `one-arm` and `one-arm-one-nic` combine the transit network with the virtual server network, so there is nothing distinct to route.

#### `LB-VIP-006` · VIP range is large enough
**Topologies** `vds-alb`, `nsx-alb`, `vds-flb` · **Category** ippool · **Severity** warning · **Confidence** MED · **Flag** ⚑
- **Expected:** The VIP range covers the expected load balancer service count plus headroom, plus whatever the Supervisor itself consumes.
- **From the network:** `cfg`.
- **Remediation:** Widen the range before enabling.
- **Source:** ALB and Supervisor address planning.
- **⚑ Confirm the Supervisor's own baseline VIP consumption** before deriving the total from `scale.expectedLoadBalancerServices` alone.

---

# Load balancer — HAProxy (`vds-haproxy`)

> **Section status, revised.** Earlier drafts of this section flagged the whole
> topology as possibly gone in the VCF 9 generation. Operator-confirmed status,
> corroborating the general direction VMware has signalled for the appliance
> model: HAProxy is **not removed**, but it **is being phased out starting with
> the vCenter 9.x generation**, and it remains **fully supported on vCenter
> 8.x**. `LB-HAP-000` is implemented on that basis and is no longer flagged.
> Everything below it (the Data Plane API rows) is unaffected by this and stays
> flagged/unimplemented for its own reasons.

<!-- BEGIN GENERATED SUMMARY -->

| ID | Requirement | Severity | ⚑ | Status |
|---|---|---|---|---|
| `LB-HAP-000` | HAProxy support status for the vCenter version in play | warning |  | ✅ `hap.version-supported` |
| `LB-HAP-001` | Data Plane API is reachable | blocker |  | ready |
| `LB-HAP-002` | Data Plane API credentials and CA are correct | blocker |  | HAProxy API |
| `LB-HAP-003` | Configured VIP range matches the declared one | blocker |  | HAProxy API |
| `LB-HAP-004` | Appliance interfaces are on the right networks | blocker |  | ready |

*1 of 5 implemented.*
<!-- END GENERATED SUMMARY -->

#### `LB-HAP-000` · HAProxy support status for the vCenter version in play
**Topologies** `vds-haproxy` · **Category** inventory · **Severity** warning · **Confidence** MED · **Flag** —
- **Expected:** vCenter is earlier than the 9.x generation, where HAProxy is fully supported. On 9.x it still works but is being phased out — a warning, not a blocker.
- **From the network:** `api` (vCenter version, already read by `vc.api-reachable`).
- **Remediation:** Migrate to NSX Advanced Load Balancer or Foundation Load Balancer before upgrading vCenter past the 8.x generation.
- **Source:** Operator-confirmed (this is not from a fetched product doc — treat the exact version boundary as MED, not HIGH, confidence, same caution as everything else in this file that lacks a citable source).
- **Implemented by:** `hap.version-supported`, in `internal/checks/alb`.

#### `LB-HAP-001` · Data Plane API is reachable
**Topologies** `vds-haproxy` · **Category** reachability · **Severity** blocker · **Confidence** MED · **Flag** —
- **Expected:** The HAProxy appliance answers on its Data Plane API port (5556 by default) from the management segment.
- **From the network:** `net`.
- **Remediation:** Open the path or correct the port.
- **Source:** HAProxy-for-Supervisor deployment guidance.

#### `LB-HAP-002` · Data Plane API credentials and CA are correct
**Topologies** `vds-haproxy` · **Category** certs · **Severity** blocker · **Confidence** MED · **Flag** —
- **Expected:** The declared credentials authenticate and the declared CA certificate matches the appliance's.
- **From the network:** `api` + `net`.
- **Remediation:** Correct the credentials or the CA certificate.
- **Source:** HAProxy-for-Supervisor deployment guidance.

#### `LB-HAP-003` · Configured VIP range matches the declared one
**Topologies** `vds-haproxy` · **Category** ippool · **Severity** blocker · **Confidence** MED · **Flag** —
- **Expected:** The range HAProxy is configured to own matches the declared load balancer CIDR and does not overlap the workload range.
- **From the network:** `api` + `cfg`.
- **Remediation:** Reconcile the appliance configuration with the declared plan.
- **Source:** HAProxy-for-Supervisor deployment guidance.

#### `LB-HAP-004` · Appliance interfaces are on the right networks
**Topologies** `vds-haproxy` · **Category** inventory · **Severity** blocker · **Confidence** MED · **Flag** —
- **Expected:** The appliance's management, workload and frontend interfaces are attached to the declared portgroups.
- **From the network:** `api` (vCenter).
- **Remediation:** Reattach the interfaces.
- **Source:** HAProxy-for-Supervisor deployment guidance.

---

# Load balancer — Foundation Load Balancer (`vds-flb`)

> **Where this section came from.** Its high-level facts were confirmed against a
> fetched Broadcom TechDocs page (see the exception noted at the top of this
> file), which is why several rows carry higher confidence than the rest of
> this document's usual default. That page describes intent and topology, not
> the tool-facing details — vCenter object model, health signals, IPAM
> mechanism — so most rows are still flagged for those specifics.

<!-- BEGIN GENERATED SUMMARY -->

| ID | Requirement | Severity | ⚑ | Status |
|---|---|---|---|---|
| `LB-FLB-000` | vCenter version supports Foundation Load Balancer | blocker |  | ✅ `flb.version-supported` |
| `LB-FLB-001` | FLB VM(s) are deployed, powered on and correctly placed | blocker | ⚑ | confirm first |
| `LB-FLB-002` | Declared network topology (arm mode) has its required networks present | blocker |  | ready |
| `LB-FLB-003` | `one-arm-one-nic` is only declared for a Simplified Supervisor deployment | blocker | ⚑ | confirm first |
| `LB-FLB-004` | HA mode has its DRS/host-anti-affinity prerequisites | warning | ⚑ | confirm first |
| `LB-FLB-005` | VIP range is not already allocated | blocker | ⚑ | confirm first |

*1 of 6 implemented.*
<!-- END GENERATED SUMMARY -->

#### `LB-FLB-000` · vCenter version supports Foundation Load Balancer
**Topologies** `vds-flb` · **Category** inventory · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** FLB requires vCenter 9.0 or later; it does not exist before that.
- **From the network:** `api` (vCenter version, already read by `vc.api-reachable`).
- **Remediation:** Upgrade vCenter, or choose ALB/NSX-LB instead.
- **Source:** "Requirements for Deploying vSphere Supervisor with Foundation Load Balancer" (Broadcom TechDocs).
- **Implemented by:** `flb.version-supported`, in `internal/checks/flb`.

#### `LB-FLB-001` · FLB VM(s) are deployed, powered on and correctly placed
**Topologies** `vds-flb` · **Category** inventory · **Severity** blocker · **Confidence** MED · **Flag** ⚑
- **Expected:** One FLB VM (single-VM mode) or two (active-passive HA mode) exist, are powered on, and sit in the `Namespaces > <Supervisor Name>` folder and resource pool alongside the Supervisor control plane VMs.
- **From the network:** `api` (vCenter). No client method exists yet.
- **Remediation:** Redeploy or power on the FLB VM(s); correct folder/resource pool placement.
- **Source:** "Architecture of vSphere Supervisor with Foundation Load Balancer".
- **⚑ Confirm:** the exact vCenter object model exposed for FLB VMs. The fetched page confirms *where* they live, not how this tool should query or distinguish them from other VMs in that resource pool.

#### `LB-FLB-002` · Declared network topology (arm mode) has its required networks present
**Topologies** `vds-flb` · **Category** inventory · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** `two-arm` declares distinct virtual-server and transit networks plus management; `one-arm` declares a combined virtual-server/transit network plus management; `one-arm-one-nic` declares a single network for everything.
- **From the network:** `cfg`.
- **Remediation:** Add the missing network block for the declared mode, or correct the mode.
- **Source:** "Architecture of vSphere Supervisor with Foundation Load Balancer" (two-arm / one-arm / one-arm-one-nic topologies).

#### `LB-FLB-003` · `one-arm-one-nic` is only declared for a Simplified Supervisor deployment
**Topologies** `vds-flb` · **Category** inventory · **Severity** blocker · **Confidence** MED · **Flag** ⚑
- **Expected:** `flb.mode: one-arm-one-nic` is used only when the Supervisor deployment is Simplified.
- **From the network:** `cfg`.
- **Remediation:** Choose `two-arm` or `one-arm` for a standard multi-zone Supervisor.
- **Source:** "Architecture of vSphere Supervisor with Foundation Load Balancer" ("supported only in a Simplified Supervisor deployment").
- **⚑ Confirm:** the config schema has no field recording "Simplified Supervisor" yet, so this cannot be implemented as written — same class of gap as `SUP-MGT-003` / `VDS-DHCP-001`. Add the field or drop the row.

#### `LB-FLB-004` · HA mode has its DRS/host-anti-affinity prerequisites
**Topologies** `vds-flb` · **Category** inventory · **Severity** warning · **Confidence** MED · **Flag** ⚑
- **Expected:** Where `flb.ha: true`, the vSphere cluster has DRS (Fully Automated) and HA enabled, so the soft host anti-affinity rule between the two FLB VMs and leader election can take effect.
- **From the network:** `api` (vCenter cluster settings) — same surface as the Supervisor cluster-readiness checks.
- **Remediation:** Enable DRS (Fully Automated) and HA on the cluster before enabling FLB's active-passive mode.
- **Source:** "Maintaining Foundation Load Balancer" / "Requirements for Deploying vSphere Supervisor with Foundation Load Balancer".
- **⚑ Confirm:** whether this tool can or should claim the anti-affinity rule itself exists, versus only the cluster prerequisites that let it be created.

#### `LB-FLB-005` · VIP range is not already allocated
**Topologies** `vds-flb` · **Category** ippool · **Severity** blocker · **Confidence** LOW · **Flag** ⚑
- **Expected:** No address in the declared VIP range is already in use on the Virtual Server Network.
- **From the network:** `net` (`COM-ADR-001`) + `api` if/when FLB exposes an allocation query through vCenter.
- **Remediation:** Choose a free range.
- **Source:** inferred from ALB's equivalent row (`LB-VIP-004`); FLB's own VIP allocation/IPAM mechanism is not described in the fetched architecture page.
- **⚑ Whole-row low confidence:** the fetched documentation does not describe FLB's VIP allocation model at all. Confirm before building on this row.

---

## Layer: Supervisor vs VKS

Added after the matrix was first written, and it changes how the whole document
should be read. See [ADR-0012](ADR/0012-supervisor-vks-layers.md).

**The overwhelming majority of these rows are Supervisor enablement
prerequisites, not VKS workload-cluster ones.** There is no VKS without a
Supervisor, so most of what an operator means by "is this ready for VKS" is
really "can the Supervisor be enabled here". Every `COM-*`, `SUP-*`, `NSX-*`,
`VPC-*`, `VDS-*` and `LB-*` row above is Supervisor-layer unless stated
otherwise.

The genuinely VKS-layer requirements are a much smaller set and are **not yet
written up as rows**. They are, at minimum:

- TKr / content library availability and reachability from the workload network
- workload node address-range sizing per cluster, including upgrade headroom
- per-cluster LoadBalancer-Service VIP consumption
- namespace-level network policy and storage class prerequisites

**⚑ This is a known gap in the matrix, not a claim that the list is short.**
Adding these rows is outstanding work. Checks already carry a `Layer` field and
`--layer supervisor|vks|both` already filters, so the mechanism is in place and
waiting for content.

Individual rows above have **not** been re-annotated with an explicit layer tag.
That is docs debt tracked in [CONTRIBUTING.md](CONTRIBUTING.md).

---

## Open questions for the reviewer

These are decisions, not doc lookups. They change the tool's shape.

1. **Is `nsx-alb` in scope?** The brief omits it, the README includes it. Currently included. (See the contradiction note at the top.)
2. **Is `vds-haproxy` in scope?** Depends on `LB-HAP-000`.
3. **Should duplicate-IP detection be invasive?** See `COM-ADR-001`.
4. **Should preflight check the *deployment* account's vCenter privileges**, not just the tool's own read access? See `COM-API-001`. It is a common failure and it is not a networking one — in or out?
5. **Two rows are blocked on config schema gaps**: `SUP-MGT-003` and `VDS-DHCP-001` need declared DHCP scopes, which the schema does not have. Add the fields or drop the rows.
6. **Version and port matrices must be data, not code** (`COM-VER-001`, `COM-VER-002`, `COM-FW-007`, `LB-ALB-002`). Where does that data come from — shipped with a release, or supplied by the user?
7. **Numeric thresholds**: how many should be config-tunable versus fixed once confirmed? The current bias is tunable, so the tool never presents an unsourced number as a product requirement.
