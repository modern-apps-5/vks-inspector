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

# interactive: asks for what it cannot discover, saves the answers
./vksinspect check --vcenter vcenter.corp.local --save-config lab01.yaml

# non-interactive: same checks, no questions. This is the pipeline form.
./vksinspect check --config lab01.yaml
./vksinspect check --config lab01.yaml --format json

./vksinspect explain
```

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
