# vks-inspector

Establishes ground truth about the networking underlying a VMware vSphere
Kubernetes Service (VKS) environment.

Point it at a declarative description of the environment you intend to build and
it tells you, before you start, which networking prerequisites hold and which do
not — with the requirement each finding traces to, what was expected, what was
actually observed, and what to do about it.

Single static binary. No runtime dependencies. Read-only. No outbound internet
calls.

> **Status: phase 1 scaffold.** The skeleton compiles, runs and produces output
> through one reference check. **No real networking checks are implemented yet** —
> they are blocked on review of
> [docs/REQUIREMENTS-MATRIX.md](docs/REQUIREMENTS-MATRIX.md), 46 of whose 88 rows
> are flagged as unconfirmed against current VCF 9 / VKS documentation. Do not
> read a passing run as a validated environment.

---

## Quick start

```bash
make build
./vksinspect check --config config/example.yaml
./vksinspect check --config config/example.yaml --format json
./vksinspect explain
```

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

| Key | Topology |
|---|---|
| `nsx` | Supervisor on NSX networking |
| `nsx-alb` | Supervisor on NSX networking with NSX Advanced Load Balancer |
| `vds-alb` | Supervisor on vSphere (VDS) networking with NSX Advanced Load Balancer |
| `vds-haproxy` | Supervisor on vSphere (VDS) networking with HAProxy — **legacy, believed removed in VCF 9** |
| `nsx-vpc` | VPC-based NSX networking (VCF 9) — **lowest-confidence requirement coverage** |

## Configuration

One declarative YAML document describes the intended topology and addressing. It
drives every mode — preflight, verify, snapshot and the future UI — so that all
of them grade against the same stated intent.

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
| [ADR/](docs/ADR/) | One record per significant architectural decision |
| [CLAUDE.md](CLAUDE.md) | Working agreement: scope, conventions, open questions |

## Contributing

Test coverage is driven by field feedback. **Open a GitHub issue** with a feature
request for a check that is missing — ideally naming the requirement it should
trace to, or describing the failure you hit so a requirement row can be written
for it.

Before adding a check, read the "Adding a check" section of
[CLAUDE.md](CLAUDE.md). The short version: every check cites a requirement ID,
runs in every mode it can, returns structured results, and never prints.

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
