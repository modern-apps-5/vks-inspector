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

**Most of what it checks are Supervisor enablement prerequisites**, not VKS
cluster ones. There is no VKS without a Supervisor, so the majority of "is this
ready for VKS" questions are really "can the Supervisor be enabled here"
questions. Use `--layer supervisor|vks|both` to narrow. See
[ADR-0012](docs/ADR/0012-supervisor-vks-layers.md).

Single static binary. No runtime dependencies. Read-only. No outbound internet
calls.

> **Status: early.** The interactive flow, the config pipeline, the address-plan
> checks and the vCenter client all work end to end. **No network probes yet**
> (DNS, TCP, TLS, NTP), and **no NSX or ALB client**, so those checks report as
> skips with a reason.
>
> Implemented: five config-only checks (CIDR overlap, external collision, infra
> collision, range containment, topology) and five vCenter checks (API reachable,
> cluster exists, VDS exists, VDS MTU, portgroups exist). The remaining checks are
> blocked on review of [docs/REQUIREMENTS-MATRIX.md](docs/REQUIREMENTS-MATRIX.md),
> 46 of whose 91 rows are flagged as unconfirmed against current VCF 9 / VKS
> documentation.
>
> **Do not read a passing run as a validated environment.** The tool says so
> itself: a run that contacts nothing prints `NOTHING IN THIS RUN CONTACTED YOUR
> ENVIRONMENT` above its own verdict.

---

## Quick start

```bash
make build

# First run: asks what it cannot discover, prompts for credentials, saves both
./vksinspect check --vcenter vcenter.corp.local --save-config lab01.yaml

# Every run after that: no questions asked
./vksinspect check --config lab01.yaml
./vksinspect check --config lab01.yaml --format json
./vksinspect check --config lab01.yaml --layer supervisor   # narrow the scope
./vksinspect check --vcenter vc.corp.local --defaults        # accept every suggested answer on Enter

./vksinspect explain            # what this build checks
./vksinspect explain dns.reverse
```

**Running a config you already have** — just point `--config` at it. Nothing
else is required; anything the file does not answer is prompted for, and a
complete file asks nothing:

```bash
./vksinspect check --config /path/to/lab01.yaml
```

`--save-config` asks before overwriting an existing file; `--force` overwrites
without asking, and is required in a non-interactive run.

`--non-interactive` no longer refuses to run a config that is missing an
optional field. Anything unanswered is listed, and the checks that need it
report as skipped — the same way the tool handles every other absence.

Add `--non-interactive` in CI so a missing value is an error naming the field
rather than a prompt that hangs a pipeline.

## Credentials

Read-only accounts are enough — the tool performs no writes.

Supply them however suits you; the tool asks only if it finds none:

```bash
# environment (takes precedence over everything)
export VKSINSPECT_VCENTER_USERNAME='readonly@vsphere.local'
export VKSINSPECT_VCENTER_PASSWORD='...'

# or a file
./vksinspect check --config lab01.yaml --credentials ~/.vksinspect/credentials.yaml

# or just run it — it prompts, hides the password, and offers to save
./vksinspect check --vcenter vcenter.corp.local
```

Saved credentials go to `~/.vksinspect/credentials.yaml` at mode `0600` (the
tool refuses to read anything looser). They are **never** written to the
environment config, a report, or a baseline. Saving is opt-in.

**A wrong stored password is not a dead end.** If the server rejects the
credentials the tool offers to re-enter them and retries. `--relogin` forces the
prompt without waiting for a failure.

**Self-signed certificates.** Lab vCenters usually have them, and verification
failure otherwise blocks every credentialed check. The tool asks, or use the flag:

```bash
./vksinspect check --config lab01.yaml --insecure-skip-tls-verify
```

When verification is off, `tls.chain` reports **`SKIP` with a reason** for that
endpoint rather than a pass — an unverified connection cannot evidence a valid
chain, so claiming otherwise would be a lie. `tls.expiry` still reports, because
the expiry date is readable regardless of trust, and says it covers expiry only.

**Declaring an endpoint by IP has consequences the tool now spells out.** A
certificate is validated against the address you connect to, so connecting to
`10.47.0.200` when the certificate is issued for `vc01.gpu.set.lab` fails SAN
matching even if the certificate is perfectly good. `tls.chain` names the
mismatch and the hostname to use; `dns.forward` discloses that the endpoint was
not name-checked at all.

The interactive flow and the config file are the same thing: prompting
*produces* a config, it is not an alternative to one. That is what keeps
`verify`, `snapshot` and `drift` — which run in pipelines and cannot prompt —
grading against the same declared intent. See
[ADR-0013](docs/ADR/0013-prompting-produces-config.md).

---

## Commands

| Command | Status | What it does |
|---|---|---|
| `vksinspect check` | **phase 1** | Preflight against the intended config |
| `vksinspect verify` | phase 2 | Post-deploy, actual vs declared |
| `vksinspect snapshot` | phase 3 | Capture current state as a baseline |
| `vksinspect drift` | phase 4 | Re-run against a stored baseline |
| `vksinspect explain` | partial | Why a requirement exists, and how to satisfy it |
| `vksinspect serve` | phase 5 | Local web UI |

Unimplemented commands exit 3 (tool error), never 0, so a pipeline calling one
by mistake fails loudly rather than recording a spurious pass.

## Exit codes

Contractual — this goes into pipelines.

| Code | Meaning |
|---|---|
| `0` | All checks passed |
| `1` | At least one blocker-severity check failed |
| `2` | Warnings failed, or results were indeterminate |
| `3` | Tool error — says nothing about the environment |

An **indeterminate** result never produces exit 1, even for a blocker. A
filtered port is a firewall, not proof that a service is down; the tool does not
assert a failure it did not observe.

## Supported topologies

Topology is two independent axes, because they are two independent decisions and
requirements attach to whichever one they actually depend on.

| `networking` | | `loadBalancer` | |
|---|---|---|---|
| `vds` | vSphere Distributed Switch | `nsx-lb` | the NSX built-in load balancer |
| `nsx` | NSX | `alb` | NSX Advanced Load Balancer (Avi) |
| `nsx-vpc` | NSX VPC-based (VCF 9) ⚑ | `haproxy` | HAProxy ⚑ *legacy* |

Supported combinations: `vds+alb`, `vds+haproxy` ⚑, `nsx+nsx-lb`, `nsx+alb`,
`nsx-vpc+nsx-lb` ⚑, `nsx-vpc+alb` ⚑. Anything else is rejected rather than
assumed workable. ⚑ marks a combination that works but whose requirement
coverage is unverified — see the matrix.

## What it checks

18 checks today. Each traces to a row in
[REQUIREMENTS-MATRIX.md](docs/REQUIREMENTS-MATRIX.md); `vksinspect explain <id>`
prints the detail.

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
| `dns.forward` | Queries **each declared resolver separately** for every endpoint name; fails if an answer points somewhere other than the declared IP |
| `dns.reverse` | PTR lookup per endpoint, compared case-insensitively to the forward name; warning unless `requireReverse: true` |
| `dns.resolver-agreement` | Asks all resolvers the same name and reports disagreement (split-horizon) |
| `tcp.port-open` | TCP connect per endpoint, **tri-state**: open passes, refused fails, silence is indeterminate — a firewall is not a dead service |
| `tls.chain` | Handshakes, then verifies the chain explicitly so an invalid cert is inspected rather than hidden behind "connect failed"; honours a pinned thumbprint |
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

**Not implemented yet:** NSX and ALB clients, ICMP/gateway probes, duplicate-IP
detection, path-MTU discovery, and every check tracing to a flagged matrix row.
Those report as skips with a reason.

## Configuration

One declarative YAML document describes the intended topology and addressing. It
drives every mode — preflight, verify, snapshot and the future UI — so that all
of them grade against the same stated intent. Write it by hand, or let
`--save-config` produce it from the interactive flow.

See [config/example.yaml](config/example.yaml).

**Credentials are never in it.** They come from a separate 0600 file or from
`VKSINSPECT_*` environment variables, are redacted in all output, and refuse to
serialise into any artifact. See
[config/credentials.example.yaml](config/credentials.example.yaml) and
[ADR-0005](docs/ADR/0005-credential-handling.md).

## Safety

- **Read-only.** No writes, no configuration changes, ever.
- **Non-invasive by default.** Probes that could disturb a production network —
  path-MTU discovery is the main one — are gated behind `--invasive` and are
  skipped *visibly*, so "not run" is never mistaken for "passed".
- **Offline.** No outbound internet calls at runtime. The tool probes only what
  the config declares.
- The account it is given should be read-only. It performs no writes, so an
  administrative account grants access it cannot use.

## Documentation

| Document | What it is for |
|---|---|
| [REQUIREMENTS-MATRIX.md](docs/REQUIREMENTS-MATRIX.md) | The authoritative requirement list, with explicit confidence and flags. **Start here.** |
| [CHECK-TAXONOMY.md](docs/CHECK-TAXONOMY.md) | The three check classes and what each may assume |
| [test-coverage.md](docs/test-coverage.md) | Testing tiers; what needs a live lab |
| [CONTRIBUTING.md](docs/CONTRIBUTING.md) | Conventions, known debt, open questions |
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

## Building

```bash
make build             # single static binary for this platform
make release           # cross-compile linux/darwin/windows, amd64 + arm64
make test              # unit and fixture tests — what CI runs
make test-integration  # needs a live lab; never runs in CI
make check             # fmt + vet + test
```

## Licence

Not yet chosen — see [LICENSE](LICENSE).
