# Contributing

How to add to vks-inspector without breaking what the rest of it relies on. Read
[ADR/](ADR/) for why these rules exist; this file is the short version.

**Looking for something to work on?** The backlog lives in
[REQUIREMENTS-MATRIX.md](REQUIREMENTS-MATRIX.md) rather than in a file of its
own. Each section opens with a generated table whose **Status** column says what
this build does about every row. Grep it:

```bash
grep -n '| ready |'         docs/REQUIREMENTS-MATRIX.md  # settled; only the work is missing
grep -n '| run location |'  docs/REQUIREMENTS-MATRIX.md  # needs a decision about where it runs
grep -n '| NSX client |'    docs/REQUIREMENTS-MATRIX.md  # unlocked by one client
grep -n 'implemented\.\*'   docs/REQUIREMENTS-MATRIX.md  # per-section totals
```

`make matrix` generates those tables from the registry and `make test` checks
them, so they cannot fall out of date. See
[Status keys](REQUIREMENTS-MATRIX.md#status-keys) for what each value means.

**Where the coverage is.** 25 of 97 rows are done. What holds the rest up is
mostly not engineering time: 45 rows are waiting on someone confirming the
requirement itself, and no amount of code moves those. Of the rest:

- **11 rows need nothing new built first** — though 6 of those only mean
  something when run from a specific network, which needs a design decision
  before anyone writes code.
- **An NSX client unlocks 7 rows** — the biggest single win available.
- An ALB client unlocks 5. A HAProxy Data Plane client unlocks 2, on a topology
  that is being phased out (`LB-HAP-000`) — weigh that before spending time
  there.

---

## The one rule for every check

> Write each check so the same code runs in **preflight** mode (against what you
> declared) and in **verify** mode (against the live environment), and so every
> result can be written to a baseline file that a later **drift** run can
> compare against.

A check that only answers "does this config make sense" cannot be reused and is
the wrong shape. If a proposed check cannot fill its row in the mode table in
[check-types.md](check-types.md), it needs a design discussion rather than an
exception.

---

## Layout

```
cmd/vksinspect/          CLI. One file per subcommand. Thin — logic belongs in checks.
internal/
  buildinfo/             Version stamps set at link time
  config/                EnvironmentSpec + the two topology settings + shape checks
  prompt/                Interactive questions; assembles a config.Config
  netx/                  Address arithmetic — overlap, containment, range sizing
  creds/                 Finds credentials. Redacts them, refuses to write them out.
  results/               Result / Report / exit codes / baseline. Imports nothing.
  checks/                The Check interface, Meta, RunContext, access it needs
    reference/           Reference implementation — copy this shape
    configval/           Config checks: pure arithmetic
    network/             Network checks: probes, no credentials       [stub]
    vcenter/ nsx/ alb/   API checks: need credentials                 [stub]
    flb/                 API checks: uses vCenter, no controller of its own
                          (flb.version-supported implemented; rest [stub])
    all/                 Registers everything above, by hand
  registry/              Holds the checks; selects them and says why
  engine/                Runs the selected checks and builds a Report. Same in every mode.
  renderers/             terminal / json / junit
  probes/                The only place allowed to open a socket    [stub]
  clients/               Read-only vCenter / NSX / ALB clients      [stub]
```

Imports only go one way: `results` ← `checks` ← `registry` ← `engine` ← `cmd`.
`checks` must never import its own subpackages — that is what `checks/all` is
for.

---

## Adding a check

1. **Find or add its row in [REQUIREMENTS-MATRIX.md](REQUIREMENTS-MATRIX.md).**
   **If the row is flagged `⚑`, stop.** A flagged row is still an open question,
   and building on it turns a guess into a stated fact that someone under time
   pressure will believe.
2. Put it in the package for [its kind of check](check-types.md).
3. Copy the shape of `internal/checks/reference`.
4. Set `Modes` honestly. If it only supports one mode, add a comment saying why.
5. Set `Layer` — the default is `supervisor`, so set it explicitly if it is not.
6. Set `Applies` against the topology **setting** the requirement actually
   depends on, not the list of combinations it happens to apply to today.
7. Set `Needs` for the access it requires. Without that access the engine skips
   the check and says why. Never fail because *we* could not get in.
8. Register it in `internal/checks/all/all.go`. **If it is not listed there, it
   does not exist.**
9. Put something comparable in `Observed.Data`. A sentence alone is invisible to
   drift.
10. Follow the reporting pattern below.
11. Write the test alongside the check, not afterwards.

### Reporting more than one finding

**One passing row summarising what was looked at, or one failing row per
problem** — each with a `Target` naming the specific finding.

A single result saying "3 overlaps" cannot be triaged, cannot have its severity
overridden one at a time, and cannot be compared by drift when one of the three
gets fixed. The passing case collapses to one row because "47 pairs did not
overlap" is not 47 findings.

### If there was nothing to check, return `skip`

Never `pass`. An empty `externalCIDRs` list, no declared ranges, no scale
supplied — each of these means the check had nothing to compare against. A green
tick for work that was never done is worse than saying it was not done.

---

## Things a check must never do

- print, or write to stdout/stderr
- return a bare bool
- call `os.Exit`
- call `time.Now()` — use `rc.Now()`, or golden tests become impossible
- open a socket directly — go through `rc.Probes`
- write to the target environment
- put a credential in `Result.Evidence`
- put jittery values (RTTs) in `Observed.Data` — every drift run would report a
  change. RTTs belong in `Evidence`.

---

## What each status means

| Status | Means |
|---|---|
| `pass` | What was expected is what was found |
| `fail` | It demonstrably is not |
| `skip` | Does not apply, or we did not have the access. **Always with a reason.** |
| `unknown` | It ran but could not tell — a filtered port, say. **Never report a failure you did not see.** |
| `error` | The *check* broke. A fault in the tool. Always exit 3. |

Severity (`blocker` / `warning` / `info`) is separate. It is a policy decision,
not the check's, and `config.Policy.SeverityOverrides` can change it at runtime.

---

## Testing

- **Table tests** for pure logic — pattern: `internal/results/exitcode_test.go`
- **Golden files** for output — pattern: `internal/renderers/golden_test.go`.
  Regenerate with `make golden` and **review the diff. That output is what
  people and pipelines depend on.**
- Network checks are unit-tested with `probes.Fake` — no network in CI, ever
- API checks are tested against API responses recorded from a lab and scrubbed
  by a script kept in the repo
- Anything needing a live lab is `//go:build integration` and never runs in CI

See [unit-test-coverage.md](unit-test-coverage.md) for the full picture and the
current gaps.

```bash
make check             # fmt + vet + test
make test              # unit and fixture tests — what CI runs
make golden            # regenerate renderer golden files
make test-integration  # needs a live lab; never in CI
```

---

## Commits

Conventional commits (`feat(scope):`, `fix:`, `docs:`, `chore:`). Logical
chunks, not one blob. Explain *why* in the body — the diff already says what.

---

## Known gaps

Written down so that what is missing is a decision rather than an oversight.

- Matrix rows have not been re-annotated with the two topology settings
  ([ADR-0011](ADR/0011-topology-axes.md)) or layer tags
  ([ADR-0012](ADR/0012-supervisor-vks-layers.md)).
- VKS-layer requirement rows have not been written up at all — see the Layer
  section of the matrix.
- No test exercises the interactive prompts; they are only tested by hand. This
  is the largest untested part of the tool.
- Drift relies on `results.WriteBaseline` writing things in a stable order, and
  nothing tests that it does.
- ~~Nothing checks that the requirement IDs a check names actually exist in the
  matrix.~~ **Closed.** `internal/docs` fails the build on a made-up ID, and on
  summary tables that no longer match the registry.
- The `blockedBy` map in `internal/docs/matrix_test.go` is maintained by hand.
  It has to be — "this needs an NSX client" is a judgement about work not done,
  which cannot be derived from work that is done. A missing entry falls back to
  `—` rather than to a wrong claim.

## Open questions

These change what gets built, and they are not ours to decide alone.

1. ~~Is `vds+haproxy` in scope?~~ **Resolved:** yes, operator-confirmed. HAProxy
   is not removed in VCF 9 — it is fully supported on vCenter 8.x and being
   phased out starting with 9.x. See `LB-HAP-000`, implemented by
   `hap.version-supported` (warns, never blocks).
2. Should duplicate-IP detection be `--invasive`? See `COM-ADR-001`.
3. Should preflight check the *deployment* account's vCenter privileges, not
   just the tool's own read access? See `COM-API-001`.
4. Two matrix rows need config fields that do not exist (declared DHCP scopes):
   `SUP-MGT-003`, `VDS-DHCP-001`. Add the fields or drop the rows.
5. Where do the version and port tables come from — shipped with a release, or
   supplied by the user? They have to be data, never code, with one narrow
   exception now settled in
   [ADR-0015](ADR/0015-flagged-rows-and-version-constants.md): the version at
   which one named product starts or stops being supported may be a constant.
   `flb.version-supported` and `hap.version-supported` are the two. A third one
   that does not fit that shape would mean the exception was written too widely,
   not that it should be widened further.
6. In verify mode, if the live environment is *better* than what was declared,
   is that a pass or a drift?
7. What exit code does `drift` use?
8. What should `externalCIDRs` mean exactly? A site that declares `10.0.0.0/8`
   as must-not-collide, then deploys management into `10.50.0.0/24`, gets a
   blocker. That is right by the letter and arguably not what they meant. Should
   the deployment's own networks be excluded from that comparison automatically?
9. `LICENSE` is a placeholder.
