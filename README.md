# vks-inspector

Checks whether the networking under a VMware vSphere Kubernetes Service (VKS)
environment is actually ready.

Point it at a vCenter. It asks for what it needs, tests the environment, and
tells you what it found — for each problem, the requirement it comes from, what
was expected, what it actually saw, and what to change.

```
vksinspect check --vcenter vcenter.corp.local
```

Single static binary. Nothing to install alongside it. Read-only. Never calls
out to the internet.

**Most of what it checks is what the Supervisor needs**, not what a VKS cluster
needs. You cannot have VKS without a Supervisor, so most "is this ready for VKS"
questions are really "can the Supervisor be turned on here" questions. See
[ADR-0012](docs/ADR/0012-supervisor-vks-layers.md).

> **Status: early.** 20 checks so far — address-plan arithmetic, network probes,
> and vCenter inventory. **No NSX, ALB controller or HAProxy Data Plane API
> checks yet** (the vCenter-version checks for those exist; the
> controller-level ones do not), and no ICMP, duplicate-IP or path-MTU probes.
> Anything missing is reported as a skip with a reason.
>
> Those 20 checks cover **25 of the 97 rows** in
> [docs/REQUIREMENTS-MATRIX.md](docs/REQUIREMENTS-MATRIX.md). 49 of those rows
> are marked as unconfirmed against current VCF 9 / VKS documentation, and no
> check states a flagged claim as fact. Four cite a flagged row but take the
> uncertain number from your config instead — the clock-skew tolerance, for
> example, is one you set, not one this tool invented.
>
> Every section of the matrix opens with a table showing which rows are done and
> what is blocking the rest. Those tables are generated from the code, so they
> cannot fall out of date.
>
> **A passing run does not mean the environment is validated.** The tool says so
> itself: a run that contacted nothing prints `NOTHING IN THIS RUN CONTACTED
> YOUR ENVIRONMENT` above its own verdict, and the coverage line names whatever
> access it did not have.

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

Check the build works without touching any real environment:

```bash
./vksinspect check --config config/example.yaml --only cidr,range,meta
```

That runs only the config checks against the example config that ships with the
repo, and exits 0. It contacts nothing, and says so above its own verdict.

Drop the `--only` and it also probes the example's addresses, which are
documentation ranges that answer nothing — so the run takes a minute and exits 2
with everything reported as unknown. That is the tool working correctly; it just
is not a quick build check.

---

## First run

Give it a vCenter. It asks for whatever it cannot find on its own, prompts for
credentials, and saves both, so later runs ask you nothing.

```bash
./vksinspect check --vcenter vcenter.corp.local --save-config lab01.yaml
```

What happens, in order:

1. **Connects to vCenter** and finds the datacenter, cluster, hosts,
   distributed switches, portgroups and any registered NSX Manager. What it
   finds it tells you rather than asking, and it never overwrites something you
   set yourself.
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

`--non-interactive` never invents answers. Anything left unanswered is listed on
stderr, and the checks that needed it are reported as skipped — the run still
finishes, and still tells you what it could not cover.

To overwrite an existing config file when re-running the wizard:

```bash
./vksinspect check --vcenter vcenter.corp.local --save-config lab01.yaml --force
```

Without `--force` an interactive run asks before overwriting, and a
non-interactive one refuses.

---

## Credentials

A **read-only** account is enough. The tool never writes anything, so an
administrator account just hands it access it cannot use.

Three ways to supply credentials. If more than one is present, the first wins:

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
baseline file. They print as `REDACTED` however you format them, and refuse to
be written to a file at all. See
[ADR-0005](docs/ADR/0005-credential-handling.md).

---

## TLS and self-signed certificates

Lab vCenters usually have self-signed certificates, and without this every
check that needs credentials fails at the TLS handshake. The tool offers this
while it asks for credentials, or you can pass it directly:

```bash
./vksinspect check --config lab01.yaml --insecure-skip-tls-verify
```

**This does not turn a failure into a pass.** With verification off,
`tls.chain` reports **`SKIP` with a reason** for that endpoint. A connection
that skipped verification cannot prove the certificate chain is good, so saying
otherwise would be a lie. `tls.expiry` still runs, because the expiry date is
readable either way, and it says it covers expiry only.

### Declare endpoints by name, not by IP

A certificate is checked against the address you connect to. If you declare
`10.47.0.200` but the certificate was issued for `vc01.gpu.set.lab`, the name
will not match **even though the certificate is perfectly good** — and there is
no name left for the DNS checks to look up.

The tool spots this, including an address typed into the `fqdn` field. It names
the mismatch and tells you which hostname to use instead. Use the FQDN and DNS
gets checked, the certificate validates properly, and you will not need
`--insecure-skip-tls-verify` at all.

---

## What it checks

20 checks. Each one points at a row in
[REQUIREMENTS-MATRIX.md](docs/REQUIREMENTS-MATRIX.md); `vksinspect explain <id>`
prints the detail for any of them.

**Config only** — arithmetic on the addressing you declared. No network, no credentials.

| Check | What it does |
|---|---|
| `cidr.overlap` | Compares every declared range against every other; reports each overlapping pair separately |
| `cidr.external-collision` | Compares declared ranges against `externalCIDRs`; skips (never passes) if that list is empty |
| `cidr.infra-collision` | Catches a pod/service CIDR that swallows a vCenter, DNS or NTP address the cluster has to reach |
| `range.containment` | Checks each static IP range sits inside its own subnet, and says which end falls outside |
| `meta.topology-recognised` | Confirms the networking × load-balancer combination is one this build knows how to grade |

**Network** — probes from wherever you run it. No credentials. The report records which host they ran from.

| Check | What it does |
|---|---|
| `dns.forward` | Asks **each declared resolver separately** for every endpoint name; fails if an answer points somewhere other than the declared IP. Skips bare IPs — "resolving" an address proves nothing |
| `dns.reverse` | PTR lookup per endpoint, compared to the forward name ignoring case; a warning unless you set `requireReverse: true` |
| `dns.resolver-agreement` | Asks every resolver the same name and reports it when they disagree (split-horizon DNS) |
| `tcp.port-open` | TCP connect per endpoint, with **three outcomes**: open passes, refused fails, and silence is unknown — a firewall is not a dead service |
| `tls.chain` | Completes the handshake, then verifies separately, so a bad certificate gets inspected rather than hidden behind "connect failed". Honours a pinned thumbprint and explains IP-vs-hostname mismatches |
| `tls.expiry` | Flags any certificate expiring within 90 days; one that has already expired becomes a blocker |
| `ntp.reachable` | A real SNTP query on 123/udp — not ping, not curl. Fails a source that answers but says it is not synchronised itself |
| `ntp.skew` | Measures the clock offset against each source; skips if you declared no tolerance, rather than inventing one |

**vCenter** — needs credentials. Read-only; the session is closed on exit.

| Check | What it does |
|---|---|
| `vc.api-reachable` | Logs in and reads `about`; also catches an ESXi host given where a vCenter was meant |
| `vc.cluster-exists` | Looks up the declared datacenter and cluster, and **lists the ones that do exist** when it cannot find them |
| `vc.vds-exists` | Same for the distributed switch |
| `vc.vds-mtu` | Compares the VDS MTU against the one your config requires; reports `unknown` if vCenter returns no MTU |
| `vc.portgroup-exists` | One result per declared portgroup: does it exist, is it on the right switch, does the VLAN match (a trunk gives `unknown`, not a false pass) |
| `flb.version-supported` | `loadBalancer: flb` only. Blocks if vCenter is older than 9.0 — Foundation Load Balancer does not exist before that |
| `hap.version-supported` | `loadBalancer: haproxy` only. Warns (never blocks) on vCenter 9.x, where HAProxy is being phased out; fully supported on 8.x |

**Not built yet:** NSX and ALB controller checks, the HAProxy Data Plane API
checks, the rest of FLB's checks (FLB has no controller of its own — it is
configured through vCenter), ICMP/gateway, duplicate-IP detection, path-MTU
discovery, and every check that would depend on a flagged matrix row.

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

Excluded checks are still **reported as skips with a reason**. A report always
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
| `-v`, `--verbose` | Show the supporting detail — resolved addresses, round-trip times, thumbprints |
| `--no-color` | Disable ANSI colour (also honours `NO_COLOR`) |

JSON **always** includes skipped results, whatever options you pass. Anything
reading the output has to be able to tell "passed" from "never ran".

---

## Exit codes

These are fixed. Pipelines depend on them.

| Code | Meaning |
|---|---|
| `0` | No check failed |
| `1` | At least one blocker-severity check failed |
| `2` | Warnings failed, or some checks could not tell |
| `3` | Tool error — says nothing about the environment |

A check that **could not tell** never produces exit 1, even a blocker. A
filtered port means a firewall, not a dead service, and the tool does not report
a failure it did not see.

Commands that are not built yet exit 3, never 0, so a pipeline that calls one by
mistake fails loudly instead of recording a pass that never happened.

```bash
./vksinspect check --config lab01.yaml --non-interactive
case $? in
  0) echo "ready" ;;
  1) echo "blockers — do not deploy" ;;
  2) echo "review the warnings" ;;
  3) echo "the tool failed; this says nothing about the environment" ;;
esac
```

---

## Reading a report

```
vksinspect  b040440
  mode      preflight
  topology  nsx+nsx-lb
  ran from  jumphost.corp.local          ← which host did the probing. A pass
  probes    read-only (non-invasive)       from your laptop is not a pass from
                                           the management network.

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

summary  20 checks: 7 passed, 1 failed, 11 skipped, 1 unknown, 0 errors
         1 blocker(s) must be fixed before deployment
         exit code 1 (one or more blockers failed)
```

Each finding answers the questions in the order you would ask them:

| Line | Answers |
|---|---|
| heading | **What is wrong** — the fault itself, not the rule it broke |
| `problem` | **Exactly what was seen**, with config paths and how far it goes |
| `expected` | What would have been fine |
| `impact` | **What it costs you** if you leave it |
| `fix` | **What to change**, and which side is easier to move |

Status labels:

| Label | Means |
|---|---|
| `PASS` | What was expected is what was found |
| `BLOCK` | A blocker-severity check failed |
| `WARN` | A warning-severity check failed |
| `UNKWN` | It ran but could not tell — a filtered port, a DNS timeout. **Not a failure.** |
| `SKIP` | Does not apply, or the tool had no access. Always with a reason. |
| `ERROR` | The check itself broke. A fault in the tool, not in the environment. |

**Read the coverage line before the summary.** "7 passed" says something about
the checks that ran, not about your environment. The coverage line tells you how
many ran, of what kind, and what access was missing.

---

## Command reference

| Command | Status | What it does |
|---|---|---|
| `check` | **working** | Check the environment against the config you intend to deploy |
| `explain [id]` | partial | Explain a check or requirement; no argument lists everything |
| `verify` | phase 2 | After deploying — what is actually there vs what you declared |
| `snapshot` | phase 3 | Save the current state as a baseline file (`--out`) |
| `drift` | phase 4 | Re-run and compare against a saved baseline (`--baseline`) |
| `serve` | phase 5 | Local web UI (`--addr`) |

```bash
./vksinspect explain                    # topologies + every check in this build
./vksinspect explain dns.reverse        # one check in detail
./vksinspect explain COM-DNS-002        # by requirement ID
./vksinspect --version
```

---

## Flag reference

Every command accepts every flag. That is deliberate: it stops any part of the
tool from quietly assuming it is only ever used for preflight.

**Input**

| Flag | Default | Purpose |
|---|---|---|
| `--vcenter <fqdn>` | — | vCenter endpoint. The starting point — everything else is discovered from it |
| `-c`, `--config <file>` | — | Saved environment config. Anything missing is prompted for |
| `--credentials <file>` | — | Credentials YAML, mode 0600. `VKSINSPECT_*` overrides it |
| `--topology <n+lb>` | — | e.g. `nsx+alb`, skipping those two prompts |

**Prompting**

| Flag | Default | Purpose |
|---|---|---|
| `--non-interactive` | off | Never prompt. Unanswered questions are listed, and the checks that needed them skip |
| `--defaults` | off | Take each prompt's example when you press Enter. **For trying out the CLI only** — the answers describe no real environment, and the run is marked as placeholder in the report, the saved config and the JSON |
| `--relogin` | off | Ignore stored credentials and ask again |
| `--save-config <file>` | — | Write the config it assembled, so later runs need no prompting |
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
| `--insecure-skip-tls-verify` | off | Do not verify management-plane TLS. Certificate checks then skip, with a reason |
| `--invasive` | off | Allow probes that may disturb the network (path-MTU discovery). Every one of these is marked in the matrix |
| `--probe-timeout <d>` | `5s` | Time limit for one DNS lookup, TCP connect or NTP query |
| `--timeout <d>` | `1m` | Time limit for a whole check, which may cover many targets |

**Output**

| Flag | Default | Purpose |
|---|---|---|
| `-f`, `--format` | `terminal` | `terminal`, `json`, `junit` |
| `-o`, `--output <file>` | stdout | Write to a file |
| `--show-skipped` | off | Show skipped checks in human output |
| `-v`, `--verbose` | off | Show the supporting detail behind each result |
| `--no-color` | off | Disable ANSI colour |

---

## Config file reference

One YAML file describes the topology and addressing you intend to deploy. Every
command reads it, so `check`, `verify`, `snapshot` and the future UI all grade
against the same thing. Write it by hand, or let `--save-config` produce it.

Full annotated example: [config/example.yaml](config/example.yaml).

```yaml
apiVersion: vksinspect/v1alpha1
kind: EnvironmentSpec

metadata:
  name: lab-nsx-01            # names the environment in reports and baselines

topology:                     # two separate settings — see Topologies below
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

Things to watch for:

- **Unknown keys are rejected**, not ignored. A mistyped `serviceCidr` is an
  error, not a field that quietly ends up empty.
- **`externalCIDRs: []` means "we asked, there are none"** and is kept. Leaving
  the key out means "nobody asked yet", and you get prompted for it.
- **No credentials, ever.** The loader refuses any config that contains keys
  that look like credentials.

---

## Topologies

Two settings you choose separately, because they are two separate decisions.
Each requirement attaches to whichever one it actually depends on
([ADR-0011](docs/ADR/0011-topology-axes.md)).

| `networking` | | `loadBalancer` | |
|---|---|---|---|
| `vds` | vSphere Distributed Switch | `nsx-lb` | NSX built-in load balancer |
| `nsx` | NSX | `alb` | NSX Advanced Load Balancer (Avi) |
| `nsx-vpc` | NSX VPC-based (VCF 9) ⚑ | `haproxy` | HAProxy *(8.x; phased out on 9.x)* |
| | | `flb` | Foundation Load Balancer *(9.0+ only)* |

Supported combinations: `vds+alb`, `vds+haproxy`, `vds+flb`, `nsx+nsx-lb`,
`nsx+alb`, `nsx-vpc+nsx-lb` ⚑, `nsx-vpc+alb` ⚑. Anything else is rejected rather
than assumed to work.

`flb` differs from `alb` and `haproxy` in one way worth knowing: there is no
separate controller or appliance to give credentials for. FLB runs as VMs inside
vCenter's own Supervisor resource pool, so its config block (`flb:`) has no
`credentialRef` — the tool's vCenter access covers it.

`flb` and `haproxy` sit on opposite sides of the same vCenter version line. FLB
does not exist before vCenter 9.0, so `flb.version-supported` blocks below that.
HAProxy is fully supported on vCenter 8.x and is being phased out starting with
9.x, so `hap.version-supported` warns and never blocks — it still works, it just
should not be your choice for something new.

⚑ means the topology works but its requirements have not been confirmed.
`nsx-vpc` in particular has **no VPC-specific checks at all** yet — every VPC row
in the matrix is flagged — so expect a quieter report than that topology
deserves.

---

## Common problems

**`could not connect: certificate is not trusted`**
Self-signed vCenter. Use `--insecure-skip-tls-verify`, or install the issuing CA.
Certificate checks then report skip-with-reason rather than a pass.

**`could not connect: incorrect user name or password`**
The tool offers a retry. Or force a fresh prompt with `--relogin`.

**`0 API check(s)` and several skips**
The tool never reached vCenter. The coverage line names the access it did not
have. Nothing in that report covers vCenter.

**`tls.chain` fails but the certificate looks fine**
You are probably connecting by IP. The finding names the certificate's real
hostname — declare that instead.

**A run takes minutes**
Every unreachable endpoint costs one `--probe-timeout`. Lower it, or narrow the
run with `--only`.

**`<file> already exists`**
`--save-config` will not overwrite quietly. Add `--force`.

**`no names declared to resolve`**
Every endpoint was declared by IP, so there is nothing for DNS to look up.
Declare FQDNs instead.

**Everything is `SKIP`**
Check `--layer` and `--only`. A run where nothing ran says so outright, rather
than reporting a clean bill of health.

---

## Safety

- **Read-only.** It writes nothing and changes nothing, ever. The one exception
  is opening a vCenter session, which is closed on exit.
- **Nothing disruptive by default.** Probes that could disturb a production
  network — path-MTU discovery is the main one — need `--invasive`, and are
  skipped *visibly* so "did not run" is never mistaken for "passed".
- **Offline.** No internet calls while it runs. It only contacts what your
  config declares.
- **Limited probing.** At most 8 probes at a time, and each one is cached for the
  run, so it never looks like a port scan to network monitoring.
- Give it a **read-only account**. It writes nothing, so anything more is access
  it cannot use.

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

vCenter behaviour is tested against **vcsim**, govmomi's API simulator, so it
runs in CI without a lab. That proves the requests are built correctly and the
responses are parsed correctly. It does not prove anything about real
authentication or VCF 9 specifics. See
[docs/unit-test-coverage.md](docs/unit-test-coverage.md).

---

## Documentation

| Document | What it is for |
|---|---|
| [REQUIREMENTS-MATRIX.md](docs/REQUIREMENTS-MATRIX.md) | The master requirement list, with a confidence level and flags on every row. **Start here.** |
| [check-types.md](docs/check-types.md) | The three kinds of check and what each one is allowed to assume |
| [CONTRIBUTING.md](docs/CONTRIBUTING.md) | Conventions, known gaps, open questions |
| [unit-test-coverage.md](docs/unit-test-coverage.md) | How things are tested, and what needs a live lab |
| [ADR/](docs/ADR/) | One record per significant design decision |
| [CHANGELOG.md](CHANGELOG.md) | What changed |

## Contributing

What gets built next comes from field feedback. **Open a GitHub issue** for a
check that is missing — ideally naming the requirement it should point at, or
describing the failure you hit so a requirement row can be written for it.

Before adding a check, read the "Adding a check" section of
[docs/CONTRIBUTING.md](docs/CONTRIBUTING.md). The short version: every check
names a requirement ID, runs in every mode it can, returns results as data, and
never prints anything itself.

## Licence

Not yet chosen — see [LICENSE](LICENSE).
