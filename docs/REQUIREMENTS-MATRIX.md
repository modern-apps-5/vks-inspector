# VKS networking requirements matrix

The authoritative list of networking prerequisites this tool grades against.
Every check in the codebase must cite one or more IDs from this file; a check
that traces to nothing here is not allowed to ship.

---

## Read this before you trust a single row

**Provenance.** This matrix was written from model knowledge with a May 2026
training cutoff. **No product documentation was fetched while writing it.** That
is a deliberate consequence of how it was commissioned — the reviewer confirms
rows against product docs and a lab — but it means the correct default posture
toward any row is suspicion, not trust.

**No URLs are cited, on purpose.** Fabricated documentation links are worse than
absent ones: they look authoritative and cost a reviewer real time to disprove.
The `Source` field names the *document area to look in*, not a URL. Treat every
one as "go and find this", not "this was read".

**Confidence levels.**

| Level | Meaning |
|---|---|
| `HIGH` | Generic networking fact, or a VMware behaviour stable across many releases. Still worth a spot-check; unlikely to be wrong. |
| `MED` | Believed correct for the TKGS / vSphere-with-Tanzu generation. Whether it survived unchanged into VCF 9 / VKS is genuinely unknown. |
| `LOW` | Reconstructed or inferred. Assume it is wrong until proven otherwise. |

**Flags.** `⚑` marks a row that must be confirmed before any check is built on
it. **46 of 89 rows are flagged.** Concentrations of doubt, worst first:

1. **Every VPC row (`VPC-*`)** — VCF 9 VPC-based Supervisor networking is the
   single least-reliable section. The object *names* may be wrong, not just the
   values. Do not implement anything here from this document.
2. **Every numeric threshold** — MTU minimums, "5 consecutive IPs", clock-skew
   tolerance, minimum prefix sizes, pool-sizing ratios. Numbers are exactly what
   changes between releases and exactly what this document is least able to
   supply. Where a number appears without a flag, it is a generic networking
   constant, not a product requirement.
3. **The port matrix (`COM-FW-*`)** — deliberately *not* enumerated. See
   `COM-FW-007`.
4. **HAProxy (`LB-HAP-*`)** — believed removed in the VCF 9 generation. The
   whole topology may be moot.
5. **Version-compatibility rows (`COM-VER-*`, `LB-ALB-002`)** — these depend on
   an external interoperability matrix that changes independently of any release
   and must never be hardcoded.

**What a flag does NOT mean.** An unflagged row is not verified — it is merely
one where the failure mode of being wrong is low (a generic networking fact). No
row in this file has been confirmed against a product document.

**Contradiction with the brief, flagged rather than silently resolved.** The
brief listed four topologies; the original README listed four *different* ones,
including `NSX + AVI`, which the brief omitted. This matrix covers the **union**.
`NSX + ALB` is a real supported shape and dropping it because the brief did not
name it would be a silent scope decision.

---

## Topology keys

The code models topology as two orthogonal axes
([ADR-0011](ADR/0011-topology-axes.md)); the rows below still use the older flat
names in their **Topologies** field. The mapping is:

| Row says | Means |
|---|---|
| `nsx` | `networking=nsx`, `loadBalancer=nsx-lb` |
| `nsx-alb` | `networking=nsx`, `loadBalancer=alb` |
| `vds-alb` | `networking=vds`, `loadBalancer=alb` |
| `vds-haproxy` ⚑ | `networking=vds`, `loadBalancer=haproxy` *(believed removed in VCF 9)* |
| `nsx-vpc` ⚑ | `networking=nsx-vpc`, either load balancer *(lowest confidence section)* |
| `all` | every combination |

**⚑ Docs debt:** re-expressing every row in terms of axes is outstanding. Where a
row says "nsx, nsx-alb" it almost always means "networking=nsx, any load
balancer", and a row saying "vds-alb, vds-haproxy" almost always means
"networking=vds". Worth doing when the rows are confirmed, since many will change
anyway.

## Verifiability keys

| Key | Meaning |
|---|---|
| `net` | Verifiable from a host on the network with no credentials — taxonomy class (a) |
| `api` | Requires vCenter / NSX / ALB API credentials — class (b) |
| `cfg` | Pure arithmetic on the declared config, no I/O — class (c) |
| `net+api` | Needs both to be answered properly; usually `net` gives a partial answer |
| `INVASIVE` | Sends traffic that may disturb the network; gated behind `--invasive` |

Severity: `blocker` (deployment fails or is unsupported) · `warning` (degrades
or bites later) · `info` (recorded, never gates).

---

# Meta

#### `MET-001` · Declared topology is supported by this build
**Topologies** all · **Category** meta · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** `topology:` names a shape this build knows how to grade.
- **From the network:** `cfg`.
- **Remediation:** Set a supported topology. If the environment uses a shape this build does not know, no other pass in the report means anything.
- **Source:** This tool. Not a product requirement.

---

# Common — DNS

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
- **From the network:** `net` — must be run from a host on each segment; a pass from the jump host says nothing about the workload network. The report records the vantage point for exactly this reason.
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
- **⚑ Confirm:** **The threshold is the problem.** The previous test-coverage document asserted 30 seconds. No documented product threshold is known to this document, and 30s appears to be a field heuristic. It is configurable rather than hardcoded so the tool does not present an unsourced number as a product requirement. Confirm whether a documented tolerance exists; if not, keep it configurable and say so in the output.

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

#### `COM-FW-001` · vCenter management port reachable
**Topologies** all · **Category** firewall · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** TCP 443 to vCenter accepts a connection from the management segment.
- **From the network:** `net` — tri-state. "Refused" proves reachability with nothing listening; a silent drop proves nothing and must report as indeterminate, never as a failure.
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

#### `COM-MTU-001` · Underlay MTU meets the overlay minimum
**Topologies** `nsx`, `nsx-alb`, `nsx-vpc` · **Category** mtu · **Severity** blocker · **Confidence** MED ⚑ · **Flag** ⚑
- **Expected:** Every hop carrying Geneve traffic supports at least the required MTU end to end.
- **From the network:** `net` + `INVASIVE` for true path verification; `api` for configured values.
- **Remediation:** Raise MTU on the physical underlay, the VDS and the NSX uplink profile together. Raising one and not the others produces intermittent, size-dependent failures that look like application bugs.
- **Source:** NSX installation prerequisites, transport network MTU requirements.
- **⚑ Confirm the number.** 1600 is the long-standing Geneve minimum and 9000 is commonly recommended in practice, but the VCF 9 requirement is not confirmed here. `nsx.overlayMTU` is configurable so the tool asserts the site's declared value rather than a number this document invented.

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

#### `COM-RTE-001` · Declared gateways respond
**Topologies** all · **Category** routing · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** Each declared gateway address is live and is inside its declared subnet.
- **From the network:** `net` (ICMP or ARP) + `cfg` for containment.
- **Remediation:** Correct the gateway address, or bring up the SVI.
- **Source:** Generic.

#### `COM-RTE-002` · Routable ranges are actually routed
**Topologies** all · **Category** routing · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** Ranges declared `routable: true` are reachable from outside their own segment and are not blackholed.
- **From the network:** `net` — and the vantage point is the whole point. A pass from inside the segment says nothing.
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

#### `COM-VER-001` · vCenter and ESXi versions are supported
**Topologies** all · **Category** inventory · **Severity** blocker · **Confidence** LOW ⚑ · **Flag** ⚑
- **Expected:** vCenter and host builds are within the supported set for the VKS release being deployed.
- **From the network:** `api`.
- **Remediation:** Upgrade to a supported build.
- **Source:** Product interoperability matrix.
- **⚑ Confirm — and note the design constraint:** the interoperability matrix changes independently of any release, so **this must never be hardcoded**. It has to be supplied as data the user can update, or the check must report the observed versions as `info` and let a human judge. Reporting a stale hardcoded matrix as authoritative would be worse than not checking.

#### `COM-VER-002` · NSX version is supported
**Topologies** `nsx`, `nsx-alb`, `nsx-vpc` · **Category** inventory · **Severity** blocker · **Confidence** LOW ⚑ · **Flag** ⚑
- **Expected:** The NSX build is within the supported set for the VKS release.
- **From the network:** `api`.
- **Remediation:** Upgrade to a supported build.
- **Source:** Product interoperability matrix.
- **⚑ Confirm:** As `COM-VER-001`. Same data-not-code constraint.

---

# Supervisor management network

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
- **⚑ Confirm what is actually required, per load balancer.** The requirements differ between ALB Service Engines and HAProxy, and enabling promiscuous mode unnecessarily is a real security cost. Do not implement a blanket assertion.

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
- **⚑ Confirm:** Which cloud type is required per topology — a vCenter cloud for `vds-alb` versus an NSX-T cloud for `nsx-alb`. Asserting the wrong one produces a confident false blocker.

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
- **Note:** this is a separate row from `LB-ALB-001` on purpose. Vantage point is the difference between a preflight that catches this and one that produces a green report and a failed deployment.

#### `LB-VIP-001` · VIP range sits inside its frontend subnet
**Topologies** `vds-alb`, `nsx-alb` · **Category** cidr · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** The declared VIP range is contained within the frontend network's CIDR.
- **From the network:** `cfg`.
- **Remediation:** Correct the VIP range or the frontend CIDR.
- **Source:** Arithmetic.

#### `LB-VIP-002` · VIP range does not overlap other allocations
**Topologies** `vds-alb`, `nsx-alb` · **Category** cidr · **Severity** blocker · **Confidence** HIGH · **Flag** —
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

#### `LB-VIP-005` · SE data network reaches the workload network
**Topologies** `vds-alb`, `nsx-alb` · **Category** routing · **Severity** blocker · **Confidence** HIGH · **Flag** —
- **Expected:** The Service Engine data network can route to the workload network where the backend pods and nodes live.
- **From the network:** `net`, run from the SE data segment.
- **Remediation:** Fix routing between the SE data and workload segments.
- **Source:** ALB Service Engine network design.

#### `LB-VIP-006` · VIP range is large enough
**Topologies** `vds-alb`, `nsx-alb` · **Category** ippool · **Severity** warning · **Confidence** MED · **Flag** ⚑
- **Expected:** The VIP range covers the expected load balancer service count plus headroom, plus whatever the Supervisor itself consumes.
- **From the network:** `cfg`.
- **Remediation:** Widen the range before enabling.
- **Source:** ALB and Supervisor address planning.
- **⚑ Confirm the Supervisor's own baseline VIP consumption** before deriving the total from `scale.expectedLoadBalancerServices` alone.

---

# Load balancer — HAProxy (`vds-haproxy`)

> **⚑ Whole-section flag.** HAProxy as a Supervisor load balancer is believed to
> be **deprecated or removed in the VCF 9 generation**. If it is gone, this
> entire section and the `vds-haproxy` topology should be deleted rather than
> maintained — carrying a dead topology forward makes the tool look more capable
> than it is. It is retained for now only because the brief asked for it and
> because older environments may still need preflighting. **Confirm support
> status first; that answer makes every other row here moot or not.**

#### `LB-HAP-000` · HAProxy topology is supported at all
**Topologies** `vds-haproxy` · **Category** inventory · **Severity** blocker · **Confidence** LOW ⚑ · **Flag** ⚑
- **Expected:** The HAProxy load balancer is a supported choice for the VKS version being deployed.
- **From the network:** `cfg` against a version-support table.
- **Remediation:** Migrate to NSX ALB.
- **Source:** VKS load balancer support statement for the target version.
- **⚑ Confirm.** This is the row that decides the fate of the section.

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
That is docs debt tracked in CLAUDE.md.

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
