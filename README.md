# vks-inspector

Establishes ground truth about the networking underlying a VMware vSphere
Kubernetes Service (VKS) environment.

Point it at a vCenter. It asks what it needs to know, interrogates the
environment, and tells you whether it is ready — with the requirement each
finding traces to, what was expected, what was actually observed, and what to do
about it.

```
vksinspect check --vcenter vcenter.corp.local
```

Single static binary. No runtime dependencies. Read-only. No outbound internet
calls.

**Most of what it checks are Supervisor enablement prerequisites**, not VKS
cluster ones. There is no VKS without a Supervisor, so the majority of "is this
ready for VKS" questions are really "can the Supervisor be enabled here"
questions. See [ADR-0012](docs/ADR/0012-supervisor-vks-layers.md).

> **Status: early.** 20 checks implemented — address-plan arithmetic, network
> probes, and vCenter inventory. **No NSX or ALB controller / HAProxy Data
> Plane API checks yet** (their vCenter-version checks are implemented; the
> controller/appliance-level checks are not), and no ICMP, duplicate-IP or
> path-MTU probes. Those report as skips with a reason.
>
> Those 20 checks cover **25 of the 97 rows** in
> [docs/REQUIREMENTS-MATRIX.md](docs/REQUIREMENTS-MATRIX.md); 49 of those rows
> are flagged as unconfirmed against current VCF 9 / VKS documentation. No
> check *asserts* a flagged claim — four cite a flagged row but deliberately
> parameterise the uncertain part (the clock-skew tolerance, for instance,
> comes from your config rather than a number this tool invented).
>
> Each section of the matrix opens with a summary table showing exactly which
> rows are implemented and what blocks the rest — generated from the check
> registry, so it cannot drift from the code.
>
> **Do not read a passing run as a validated environment.** The tool says so
> itself: a run that contacts nothing prints `NOTHING IN THIS RUN CONTACTED YOUR
> ENVIRONMENT` above its own verdict, and the coverage line names any access it
> lacked.

---

## Contents

- [Install](#install)
- [First run](#first-run)
- [Running a config you already have](#running-a-config-you-already-have)
- [Credentials](#credentials)
- [TLS and self-signed certificates](#tls-and-self-signed-certificates)
- [What it checks](#what-it-checks)
- [Selecting what runs](#selecting-what-runs)
- [Output formats](#output-formats)
- [Exit codes](#exit-codes)
- [Reading a report](#reading-a-report)
- [Command reference](#command-reference)
- [Flag reference](#flag-reference)
- [Config file reference](#config-file-reference)
- [Topologies](#topologies)
- [Common problems](#common-problems)
- [Safety](#safety)
- [Building and testing](#building-and-testing)
- [Documentation](#documentation)

---

## Install

Needs Go 1.25 or newer to build. There are no published binaries yet.

```bash
git clone https://github.com/modern-apps-5/vks-inspector
cd vks-inspector
make build          # produces ./vksinspect
```

Cross-compile for other platforms:

```bash
make release        # dist/ for linux+darwin (amd64/arm64) and windows/amd64
```

Verify the build without touching any real environment:

```bash
./vksinspect check --config config/example.yaml
```

That grades the shipped example and exits 0. Nothing is contacted.

---

## First run

Give it a vCenter. It asks for what it cannot discover, prompts for credentials,
and saves both so later runs ask nothing.

```bash
./vksinspect check --vcenter vcenter.corp.local --save-config lab01.yaml
```

What happens, in order:

1. **Connects to vCenter** and discovers the datacenter, cluster, hosts,
   distributed switches, portgroups and any registered NSX Manager. Discovered
   values are reported, not asked, and never overwrite something you declared.
2. **Asks for credentials** if it has none — username plainly, password with echo
   off — and offers to save them.
3. **Asks for the address plan**: topology, management and workload CIDRs, pod and
   service CIDRs, ingress/egress, expected scale. Each prompt shows an example.
4. **Runs the checks** and prints a report.
5. **Writes `lab01.yaml`** with everything you answered. No credentials go in it.

Press Enter through everything with `--defaults` — see
[Flag reference](#flag-reference) for why that is for testing only.

---

## Running a config you already have

Point `--config` at it. Nothing else is required.

```bash
./vksinspect check --config lab01.yaml
```

A complete config asks nothing. A partial one prompts only for the gaps. To
guarantee no prompting at all — in CI, or a cron job:

```bash
./vksinspect check --config lab01.yaml --non-interactive
```

`--non-interactive` will not invent answers. Anything unanswered is listed on
stderr and the checks that need it report as skipped, so the run still completes
and still tells you what it could not cover.

To overwrite an existing config file when re-running the wizard:

```bash
./vksinspect check --vcenter vcenter.corp.local --save-config lab01.yaml --force
```

Without `--force` an interactive run asks before overwriting, and a
non-interactive one refuses.

---

## Credentials

A **read-only** account is enough. The tool performs no writes, so an
administrative account grants access it cannot use.

Three ways, in precedence order:

**1. Environment** — wins over everything. Best for CI.

```bash
export VKSINSPECT_VCENTER_USERNAME='readonly@vsphere.local'
export VKSINSPECT_VCENTER_PASSWORD='...'
./vksinspect check --config lab01.yaml
```

The pattern is `VKSINSPECT_<REF>_USERNAME` / `_PASSWORD` / `_TOKEN` /
`_CACERTFILE`, where `<REF>` is the `credentialRef` from the config, uppercased —
`VCENTER`, `NSX`, `ALB`, `HAPROXY`.

**2. A file.**

```bash
./vksinspect check --config lab01.yaml --credentials ~/.vksinspect/credentials.yaml
```

Must be mode `0600`; the tool refuses to read anything looser. A path that does
not exist yet is fine — that is the state before the first save. See
[config/credentials.example.yaml](config/credentials.example.yaml).

**3. Just run it.** With no credentials found, it asks and offers to save to
`~/.vksinspect/credentials.yaml` at mode `0600`. Saving is opt-in.

### Changing a stored password

If the server rejects the credentials, the tool offers to re-enter them and
retries. To force a fresh prompt without waiting for a failure:

```bash
./vksinspect check --config lab01.yaml --relogin
```

Credentials are **never** written to the environment config, a report, or a
baseline. They redact under every format verb and refuse to serialise into any
artifact. See [ADR-0005](docs/ADR/0005-credential-handling.md).

---

## TLS and self-signed certificates

Lab vCenters are usually self-signed, and verification failure otherwise blocks
every credentialed check. The tool asks during credential entry, or:

```bash
./vksinspect check --config lab01.yaml --insecure-skip-tls-verify
```

**This does not turn a failure into a pass.** With verification off, `tls.chain`
reports **`SKIP` with a reason** for that endpoint — an unverified connection
cannot evidence a valid chain, so claiming otherwise would be a lie.
`tls.expiry` still reports, because an expiry date is readable regardless of
trust, and says it covers expiry only.

### Declare endpoints by name, not by IP

A certificate is validated against the address you connect to. Declaring
`10.47.0.200` when the certificate is issued for `vc01.gpu.set.lab` fails name
matching **even if the certificate is perfectly good**, and there is no name for
DNS checks to resolve.

The tool detects this — including an address typed into the `fqdn` field — names
the mismatch, and tells you which hostname to use instead. Using the FQDN means
DNS gets checked, the certificate validates properly, and you will not need
`--insecure-skip-tls-verify` at all.

---

## What it checks

20 checks. Each traces to a row in
[REQUIREMENTS-MATRIX.md](docs/REQUIREMENTS-MATRIX.md); `vksinspect explain <id>`
prints the detail for any of them.

**Config only** — arithmetic on your declared addressing. No network, no credentials.

| Check | What it does |
|---|---|
| `cidr.overlap` | Compares every declared range pairwise; reports each overlapping pair separately |
| `cidr.external-collision` | Compares declared ranges against `externalCIDRs`; skips (never passes) if that list is empty |
| `cidr.infra-collision` | Flags a pod/service CIDR that swallows a vCenter, DNS or NTP address the cluster must reach |
| `range.containment` | Checks each static IP range falls inside its own subnet, and says which end escapes |
| `meta.topology-recognised` | Confirms the networking × load-balancer combination is one this build can grade |

**Network** — probes from wherever you run it. No credentials. The report records the vantage host.

| Check | What it does |
|---|---|
| `dns.forward` | Queries **each declared resolver separately** for every endpoint name; fails if an answer points somewhere other than the declared IP. Skips IP literals — "resolving" an address proves nothing |
| `dns.reverse` | PTR lookup per endpoint, compared case-insensitively to the forward name; warning unless `requireReverse: true` |
| `dns.resolver-agreement` | Asks all resolvers the same name and reports disagreement (split-horizon) |
| `tcp.port-open` | TCP connect per endpoint, **tri-state**: open passes, refused fails, silence is indeterminate — a firewall is not a dead service |
| `tls.chain` | Handshakes, then verifies explicitly so an invalid cert is inspected rather than hidden behind "connect failed"; honours a pinned thumbprint; diagnoses IP-vs-hostname mismatches |
| `tls.expiry` | Flags any certificate expiring within 90 days; already-expired escalates to blocker |
| `ntp.reachable` | Real SNTP query on 123/udp — not ping, not curl; fails a source that answers but reports itself unsynchronised |
| `ntp.skew` | Measures clock offset against each source; skips if you declared no tolerance rather than inventing one |

**vCenter** — needs credentials. Read-only; the session is closed on exit.

| Check | What it does |
|---|---|
| `vc.api-reachable` | Logs in and reads `about`; also catches an ESXi host given where a vCenter was meant |
| `vc.cluster-exists` | Looks up the declared datacenter and cluster, and **lists the ones that do exist** when it misses |
| `vc.vds-exists` | Same for the distributed switch |
| `vc.vds-mtu` | Compares VDS MTU against the requirement your config declares; reports `unknown` if vCenter returns no MTU |
| `vc.portgroup-exists` | One result per declared portgroup: existence, backing switch, and VLAN match (a trunk yields `unknown`, not a false pass) |
| `flb.version-supported` | `loadBalancer: flb` only. Blocks if vCenter is older than 9.0 — Foundation Load Balancer does not exist before that |
| `hap.version-supported` | `loadBalancer: haproxy` only. Warns (never blocks) once vCenter reaches the 9.x generation, where HAProxy is being phased out; fully supported on 8.x |

**Not implemented:** NSX and ALB controller checks, the HAProxy Data Plane API
checks, and the rest of FLB's checks (FLB has no separate controller client —
it is configured through vCenter, not one of its own), ICMP/gateway,
duplicate-IP detection, path-MTU discovery, and every check tracing to a
flagged matrix row.

---

## Selecting what runs

```bash
# by check ID
./vksinspect check --config lab01.yaml --only dns.forward

# by namespace — dns, tcp, tls, ntp, cidr, range, vc, meta
./vksinspect check --config lab01.yaml --only dns,tls

# by category — cidr, routing, mtu, dns, ntp, firewall, certs,
#               reachability, ippool, inventory, meta, storage
./vksinspect check --config lab01.yaml --only certs

# exclude instead
./vksinspect check --config lab01.yaml --skip ntp

# Supervisor prerequisites only, or VKS-cluster ones only
./vksinspect check --config lab01.yaml --layer supervisor
```

Excluded checks are still **reported as skips with a reason** — a report always
accounts for every check the build knows about.

You can also skip permanently from the config, which records the decision in
every baseline:

```yaml
policy:
  skip: [ntp.skew]
  severityOverrides:
    COM-NTP-002: warning
```

---

## Output formats

```bash
./vksinspect check --config lab01.yaml -f terminal    # default, human
./vksinspect check --config lab01.yaml -f json        # machine, and the baseline format
./vksinspect check --config lab01.yaml -f junit       # CI collectors
./vksinspect check --config lab01.yaml -f json -o report.json
```

Useful with terminal output:

| Flag | Effect |
|---|---|
| `--show-skipped` | Show skipped checks (JSON always includes them) |
| `-v`, `--verbose` | Include evidence detail — resolved addresses, RTTs, thumbprints |
| `--no-color` | Disable ANSI colour (also honours `NO_COLOR`) |

JSON **never** omits skipped results regardless of options: a consumer must be
able to tell "passed" from "never ran".

---

## Exit codes

Contractual. This goes into pipelines.

| Code | Meaning |
|---|---|
| `0` | No check failed |
| `1` | At least one blocker-severity check failed |
| `2` | Warnings failed, or results were indeterminate |
| `3` | Tool error — says nothing about the environment |

An **indeterminate** result never produces exit 1, even for a blocker. A filtered
port is a firewall, not proof a service is down; the tool does not assert a
failure it did not observe.

Unimplemented commands exit 3, never 0, so a pipeline calling one by mistake
fails loudly rather than recording a spurious pass.

```bash
./vksinspect check --config lab01.yaml --non-interactive
case $? in
  0) echo "ready" ;;
  1) echo "blockers — do not deploy" ;;
  2) echo "review warnings" ;;
  3) echo "the tool failed; this says nothing about the environment" ;;
esac
```

---

## Reading a report

```
vksinspect  b040440
  mode      preflight
  topology  nsx+nsx-lb
  vantage   jumphost.corp.local          ← which host probed. A pass from your
  probes    read-only (non-invasive)       laptop is not a pass from the
                                           management segment.

BLOCK The workload-primary network sits entirely inside the Kubernetes service CIDR
      check     cidr.overlap  (COM-CID-001)     ← matrix row; `explain` it
      problem   networks.workload[0].cidr (10.96.0.0/23) sits entirely inside
                kubernetes.serviceCIDR (10.96.0.0/22); overlapping range
                10.96.0.0-10.96.1.255 — 512 addresses
      expected  10.96.0.0/23 and 10.96.0.0/22 share no addresses
      impact    Addresses in a pod or service CIDR are claimed by the cluster
                and only exist inside it. Any real host in the overlapping
                range becomes unreachable from pods...
      fix       Change one of the two so they share no addresses. The
                Kubernetes service CIDR is cluster-internal and does not need
                to be routable, so it is usually the easier to move...

checks    13 ran of 20 in this build — 5 config-only, 8 network probe(s), 0 API check(s)
          no vcenter access — 4 check(s) could not run, so nothing above covers it

summary  20 checks: 7 passed, 1 failed, 11 skipped, 1 indeterminate, 0 errors
         1 blocker(s) must be fixed before deployment
         exit code 1 (one or more blockers failed)
```

Each finding answers three questions in the order you ask them:

| Line | Answers |
|---|---|
| heading | **What is wrong** — the fault, not the rule that was broken |
| `problem` | **Exactly what was seen**, with config paths and the extent |
| `expected` | What would have been acceptable |
| `impact` | **What it costs you** if left alone |
| `fix` | **What to change**, and which side is easier to move |

Status labels:

| Label | Means |
|---|---|
| `PASS` | The expected condition holds |
| `BLOCK` | A blocker-severity check failed |
| `WARN` | A warning-severity check failed |
| `UNKWN` | Ran, could not determine — a filtered port, a DNS timeout. **Not a failure.** |
| `SKIP` | Not applicable, or the tool lacked access. Always with a reason. |
| `ERROR` | The check itself malfunctioned. A tool fault, not an environment one. |

**Read the coverage line before the summary.** "7 passed" is a statement about
the checks that ran, not about your environment. The coverage line says how many
ran, of what kind, and what access was missing.

---

## Command reference

| Command | Status | What it does |
|---|---|---|
| `check` | **working** | Preflight against the intended config |
| `explain [id]` | partial | Explain a check or requirement; no argument lists everything |
| `verify` | phase 2 | Post-deploy, actual vs declared |
| `snapshot` | phase 3 | Capture current state as a baseline (`--out`) |
| `drift` | phase 4 | Re-run against a stored baseline (`--baseline`) |
| `serve` | phase 5 | Local web UI (`--addr`) |

```bash
./vksinspect explain                    # topologies + every check in this build
./vksinspect explain dns.reverse        # one check in detail
./vksinspect explain COM-DNS-002        # by requirement ID
./vksinspect --version
```

---

## Flag reference

All flags are global — every mode accepts them, so nothing in the tool can grow a
preflight-only assumption.

**Input**

| Flag | Default | Purpose |
|---|---|---|
| `--vcenter <fqdn>` | — | vCenter endpoint. The entry point; other endpoints are discovered from it |
| `-c`, `--config <file>` | — | Saved environment config. Gaps are prompted for |
| `--credentials <file>` | — | Credentials YAML, mode 0600. `VKSINSPECT_*` overrides it |
| `--topology <n+lb>` | — | e.g. `nsx+alb`, skipping those two prompts |

**Prompting**

| Flag | Default | Purpose |
|---|---|---|
| `--non-interactive` | off | Never prompt. Unanswered questions are listed; dependent checks skip |
| `--defaults` | off | Accept each prompt's example on Enter. **Exercising the CLI only** — the answers describe no real environment, and the run is marked as placeholder in the report, the saved config and the JSON |
| `--relogin` | off | Ignore stored credentials and ask again |
| `--save-config <file>` | — | Write the assembled config for non-interactive re-runs |
| `--force` | off | Let `--save-config` overwrite |

**Selection**

| Flag | Default | Purpose |
|---|---|---|
| `--only <list>` | all | Check IDs, namespaces or categories |
| `--skip <list>` | none | Same vocabulary, excluded |
| `--layer <l>` | `both` | `supervisor`, `vks` or `both` |

**Behaviour**

| Flag | Default | Purpose |
|---|---|---|
| `--insecure-skip-tls-verify` | off | Do not verify management-plane TLS. Certificate checks downgrade to skip-with-reason |
| `--invasive` | off | Permit probes that may disturb the network (path-MTU discovery). Every invasive check is marked in the matrix |
| `--probe-timeout <d>` | `5s` | Bounds one DNS lookup, TCP connect or NTP query |
| `--timeout <d>` | `1m` | Bounds a whole check, which may fan out over many targets |

**Output**

| Flag | Default | Purpose |
|---|---|---|
| `-f`, `--format` | `terminal` | `terminal`, `json`, `junit` |
| `-o`, `--output <file>` | stdout | Write to a file |
| `--show-skipped` | off | Show skipped checks in human output |
| `-v`, `--verbose` | off | Include evidence detail |
| `--no-color` | off | Disable ANSI colour |

---

## Config file reference

One declarative YAML document describes intended topology and addressing. It
drives every mode, so preflight, verify, snapshot and the future UI all grade
against the same stated intent. Write it by hand or let `--save-config` produce
it.

Full annotated example: [config/example.yaml](config/example.yaml).

```yaml
apiVersion: vksinspect/v1alpha1
kind: EnvironmentSpec

metadata:
  name: lab-nsx-01            # names the environment in reports and baselines

topology:                     # two independent axes — see Topologies below
  networking: nsx             # vds | nsx | nsx-vpc
  loadBalancer: nsx-lb        # nsx-lb | alb | haproxy | flb

infrastructure:
  vcenter:
    fqdn: vcenter.lab.example.com   # prefer the FQDN over an IP
    ip: 192.0.2.10                  # optional; checked against DNS if both given
    port: 443
    credentialRef: vcenter          # a NAME, never a secret
    # expectedThumbprint: "AA:BB:.."  pin the cert instead of only validating it
  nsxManager: { fqdn: nsx.lab.example.com, credentialRef: nsx }
  esxiHosts:
    - { fqdn: esx01.lab.example.com, ip: 192.0.2.21 }
  registry: { fqdn: harbor.lab.example.com, port: 443 }
  # proxy: { https: "http://proxy:3128", noProxy: ["10.0.0.0/8"] }

services:
  dns:
    servers: [192.0.2.53, 192.0.2.54]   # each is queried separately
    searchDomains: [lab.example.com]
    requireReverse: true                # makes a PTR mismatch a blocker
    additionalNames: [supervisor.lab.example.com]
  ntp:
    servers: [192.0.2.123]
    maxSkewSeconds: 30                  # your tolerance; not a product limit

vsphere:
  datacenter: DC1
  cluster: Compute-01
  distributedSwitch: DSwitch-01
  storagePolicy: vsan-default
  contentLibrary: tkr-library

networks:
  management:                  # Supervisor control plane VMs
    name: management
    portGroup: PG-Mgmt         # required for VDS topologies
    vlan: 100
    cidr: 192.0.2.0/24
    gateway: 192.0.2.1
    mtu: 1500
    routable: true
    ranges:
      - { start: 192.0.2.30, end: 192.0.2.34, purpose: supervisor-control-plane }
  workload:
    - { name: workload-primary, cidr: 10.20.0.0/16, gateway: 10.20.0.1, routable: true }
  # frontend: {...}            # VIP network; required for ALB and HAProxy
                               # (FLB declares its own vipNetwork/transitNetwork instead — see below)

kubernetes:
  podCIDRs: [100.96.0.0/16]    # cluster-internal
  serviceCIDR: 100.64.0.0/18
  ingressCIDR: 198.51.100.0/24 # NSX topologies: LB VIPs
  egressCIDR: 203.0.113.0/24   # NSX topologies: SNAT addresses
  apiServerVIP: 192.0.2.30
  externalCIDRs:               # what the plan must not collide with.
    - 10.0.0.0/8               # An EMPTY list makes collision detection a
    - 172.16.0.0/12            # reported SKIP, not a pass.

nsx:                           # networking: nsx or nsx-vpc
  tier0Gateway: T0-Edge-01
  edgeCluster: EdgeCluster-01
  transportZone: TZ-Overlay
  overlayMTU: 1700             # your required underlay MTU
  # vpc: {...}                 # nsx-vpc only; object model unverified

# alb: {...}                   # loadBalancer: alb
# haproxy: {...}               # loadBalancer: haproxy
# flb: {...}                   # loadBalancer: flb; no controller endpoint — configured through vCenter

scale:                         # drives pool-sizing checks
  supervisorControlPlaneNodes: 3
  expectedNamespaces: 20
  expectedWorkloadClusters: 5
  expectedNodesPerCluster: 6
  expectedLoadBalancerServices: 30
  growthHeadroomPercent: 30

policy:
  severityOverrides: {}        # by check ID or requirement ID
  skip: []
  allowInvasive: false
```

Notes that bite:

- **Unknown keys are rejected**, not ignored. A typo'd `serviceCidr` is an error,
  not a silently-empty field.
- **`externalCIDRs: []` means "asked, none"** and is preserved. An absent key
  means "never asked" and gets prompted for.
- **No credentials, ever.** The loader refuses a config containing
  credential-shaped keys.

---

## Topologies

Two independent axes, because they are two independent decisions, and
requirements attach to whichever one they actually depend on
([ADR-0011](docs/ADR/0011-topology-axes.md)).

| `networking` | | `loadBalancer` | |
|---|---|---|---|
| `vds` | vSphere Distributed Switch | `nsx-lb` | NSX built-in load balancer |
| `nsx` | NSX | `alb` | NSX Advanced Load Balancer (Avi) |
| `nsx-vpc` | NSX VPC-based (VCF 9) ⚑ | `haproxy` | HAProxy *(8.x; phased out on 9.x)* |
| | | `flb` | Foundation Load Balancer *(9.0+ only)* |

Supported combinations: `vds+alb`, `vds+haproxy`, `vds+flb`, `nsx+nsx-lb`,
`nsx+alb`, `nsx-vpc+nsx-lb` ⚑, `nsx-vpc+alb` ⚑. Anything else is rejected
rather than assumed workable.

`flb` differs from `alb`/`haproxy` in one structural way worth knowing: FLB has
no separate controller/appliance to declare credentials for. It runs as VM(s)
inside vCenter's own Supervisor resource pool, so its config block (`flb:`)
carries no `credentialRef` and the tool's vCenter access covers it.

`flb` and `haproxy` are also opposite ends of the same vCenter-version boundary:
FLB does not exist before vCenter 9.0 (`flb.version-supported` blocks below
that), while HAProxy is fully supported on vCenter 8.x and is being phased out
starting with 9.x (`hap.version-supported` warns, never blocks — it still
works, it just should not be a new design).

⚑ works, but its requirement coverage is unverified. `nsx-vpc` in particular has
**no VPC-specific checks at all** yet — every VPC row in the matrix is flagged, so
expect a quieter report than that topology deserves.

---

## Common problems

**`could not connect: certificate is not trusted`**
Self-signed vCenter. Use `--insecure-skip-tls-verify`, or install the issuing CA.
Certificate checks then report skip-with-reason rather than a pass.

**`could not connect: incorrect user name or password`**
The tool offers a retry. Or force a fresh prompt with `--relogin`.

**`0 API check(s)` and several skips**
The tool never reached vCenter. The coverage line names the missing access.
Nothing in that report covers vCenter.

**`tls.chain` fails but the certificate looks fine**
You are probably connecting by IP. The finding names the certificate's actual
hostname — declare that instead.

**A run takes minutes**
Unreachable endpoints cost one `--probe-timeout` each. Lower it, or narrow with
`--only`.

**`<file> already exists`**
`--save-config` will not overwrite silently. Add `--force`.

**`no names declared to resolve`**
Every endpoint was declared by IP, so there is nothing for DNS to check. Declare
FQDNs.

**Everything is `SKIP`**
Check `--layer` and `--only`. A run where nothing executed says so explicitly
rather than reporting a clean bill of health.

---

## Safety

- **Read-only.** No writes, no configuration changes, ever. The only write is the
  vCenter session, which is closed on exit.
- **Non-invasive by default.** Probes that could disturb a production network —
  path-MTU discovery is the main one — sit behind `--invasive` and are skipped
  *visibly*, so "not run" is never mistaken for "passed".
- **Offline.** No outbound internet calls at runtime. The tool probes only what
  the config declares.
- **Bounded probing.** At most 8 concurrent probes, each memoised per run, so the
  tool never looks like a scan to network monitoring.
- Give it a **read-only account**. It performs no writes.

---

## Building and testing

```bash
make build             # single static binary for this platform
make release           # cross-compile linux/darwin/windows, amd64 + arm64
make test              # unit and simulator tests — what CI runs
make test-race         # same, with the race detector
make cover             # coverage.html
make check             # fmt + vet + test
make golden            # regenerate renderer golden files; review the diff
make test-integration  # needs a live lab; never runs in CI
make smoke             # build and run against the shipped example
make clean
```

vCenter behaviour is tested against **vcsim**, govmomi's API simulator, so it runs
in CI without a lab. That verifies request construction and response parsing; it
does not verify real authentication or VCF 9 specifics. See
[docs/unit-test-coverage.md](docs/unit-test-coverage.md).

---

## Documentation

| Document | What it is for |
|---|---|
| [REQUIREMENTS-MATRIX.md](docs/REQUIREMENTS-MATRIX.md) | The authoritative requirement list, with explicit confidence and flags. **Start here.** |
| [CHECK-TAXONOMY.md](docs/CHECK-TAXONOMY.md) | The three check classes and what each may assume |
| [CONTRIBUTING.md](docs/CONTRIBUTING.md) | Conventions, known debt, open questions |
| [unit-test-coverage.md](docs/unit-test-coverage.md) | Testing tiers; what needs a live lab |
| [ADR/](docs/ADR/) | One record per significant architectural decision |
| [CHANGELOG.md](CHANGELOG.md) | What changed |

## Contributing

Test coverage is driven by field feedback. **Open a GitHub issue** with a feature
request for a check that is missing — ideally naming the requirement it should
trace to, or describing the failure you hit so a requirement row can be written
for it.

Before adding a check, read the "Adding a check" section of
[docs/CONTRIBUTING.md](docs/CONTRIBUTING.md). The short version: every check
cites a requirement ID, runs in every mode it can, returns structured results,
and never prints.

## Licence

Not yet chosen — see [LICENSE](LICENSE).
