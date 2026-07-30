# Contributing

How to add to vks-inspector without breaking the properties the rest of it
depends on. Read [ADR/](ADR/) for why any of these rules exist; this file is the
operational summary.

**Looking for something to work on?** The backlog lives in
[REQUIREMENTS-MATRIX.md](REQUIREMENTS-MATRIX.md), not in a file of its own —
each section opens with a generated summary table whose **Status** column says
what this build does about every row. Grep it:

```bash
grep -n '| ready |'        docs/REQUIREMENTS-MATRIX.md  # settled; only the work is missing
grep -n '| vantage |'      docs/REQUIREMENTS-MATRIX.md  # decide the vantage story first
grep -n '| NSX client |'   docs/REQUIREMENTS-MATRIX.md  # unlocked by one client
grep -n 'implemented\.\*'  docs/REQUIREMENTS-MATRIX.md  # per-section rollups
```

Those tables are generated from the registry by `make matrix` and verified by
`make test`, so they cannot drift from the code. See
[Status keys](REQUIREMENTS-MATRIX.md#status-keys) for what each value means.

**Where the coverage is.** 25 of 97 rows are implemented. The binding
constraint is not engineering time — 45 rows are blocked on confirming the
requirement itself, and no amount of code moves those. Of what is actionable:

- **11 rows need no new infrastructure**, though 6 of those are `vantage` rows
  that need a design decision before anyone writes code.
- **An NSX client unlocks 7 rows** — the largest single win available.
- An ALB client unlocks 5. A HAProxy Data Plane client unlocks 2, on a topology
  being phased out (`LB-HAP-000`) — weigh that before spending time there.

---

## The rule that governs every check

> Write every check so the same check unit runs in **preflight** mode (declared
> intent) and **verify** mode (live environment), and so every result serialises
> into a baseline artifact a later **drift** run can diff.

A check that only answers "does this config make sense" is not reusable and is
the wrong shape. If a proposed check cannot fill its row in the mode table in
[CHECK-TAXONOMY.md](CHECK-TAXONOMY.md), that is a design conversation — not an
exception to grant.

---

## Layout

```
cmd/vksinspect/          CLI. One file per subcommand. Thin — logic belongs in checks.
internal/
  buildinfo/             Link-time version stamps
  config/                EnvironmentSpec + topology axes + structural validation
  prompt/                Interactive question flow; assembles a config.Config
  netx/                  Address arithmetic — overlap, containment, range sizing
  creds/                 Credential resolution. Redacts, refuses to serialise.
  results/               Result / Report / exit codes / baseline. Leaf package.
  checks/                The Check interface, Meta, RunContext, capabilities
    reference/           Reference implementation — copy this shape
    configval/           Class (c): pure config arithmetic
    network/             Class (a): network-only probes             [stub]
    vcenter/ nsx/ alb/   Class (b): credentialed                    [stub]
    flb/                 Class (b): vCenter-credentialed, no dedicated controller
                          (flb.version-supported implemented; rest [stub])
    all/                 Explicit registration of everything above
  registry/              Check storage + selection with reasons
  engine/                Runs selected checks, assembles a Report. Mode-agnostic.
  renderers/             terminal / json / junit
  probes/                The only place allowed to open a socket    [stub]
  clients/               Read-only vCenter / NSX / ALB clients      [stub]
```

Import direction is one-way: `results` ← `checks` ← `registry` ← `engine` ←
`cmd`. `checks` must never import its own subpackages — that is what
`checks/all` is for.

---

## Adding a check

1. **Find or add its row in [REQUIREMENTS-MATRIX.md](REQUIREMENTS-MATRIX.md).**
   **If the row is flagged `⚑`, stop.** A flagged row is an open question, and
   building on it launders a guess into an assertion that someone under time
   pressure will believe.
2. Put it in the package matching its [taxonomy class](CHECK-TAXONOMY.md).
3. Copy the shape of `internal/checks/reference`.
4. Declare `Modes` honestly. Supporting only one mode needs a comment justifying
   it.
5. Declare `Layer` — the default is `supervisor`, so set it explicitly if it is
   not.
6. Declare `Applies` against the topology **axis** the requirement actually
   depends on, not the list of combinations it happens to apply to today.
7. Declare `Needs` capabilities. A missing capability makes the engine skip with
   a reason. Never fail because *we* lacked access.
8. Register it in `internal/checks/all/all.go`. **If it is not listed there, it
   does not exist.**
9. Fill `Observed.Data` with something machine-comparable. Prose alone is
   invisible to drift.
10. Follow the fan-out idiom (below).
11. Write the test with the check, not after it.

### The fan-out idiom

**One passing row summarising what was examined, or one failing row per problem**
— each with a `Target` naming the specific finding.

A single result saying "3 overlaps" cannot be triaged, cannot be individually
severity-overridden, and cannot be diffed by drift when one of the three is
fixed. The passing case collapses to one row because "47 pairs were disjoint" is
not 47 findings.

### If there was nothing to check, return `skip`

Never `pass`. An empty `externalCIDRs` list, no declared ranges, no supplied
scale — all of these mean the check had nothing to compare against. A green tick
for work that was not done is worse than admitting it was not done.

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

## Result semantics

| Status | Means |
|---|---|
| `pass` | The expected condition holds |
| `fail` | It demonstrably does not |
| `skip` | Not applicable, or we lacked a capability. **Always with a reason.** |
| `unknown` | Ran, could not determine. A filtered port. **Never assert a failure you did not observe.** |
| `error` | The *check* malfunctioned. A tool fault. Always exit 3. |

Severity (`blocker` / `warning` / `info`) is separate, and is policy rather than
the check's decision — `config.Policy.SeverityOverrides` can change it at
runtime.

---

## Testing

- **Table tests** for pure logic — pattern: `internal/results/exitcode_test.go`
- **Golden files** for output — pattern: `internal/renderers/golden_test.go`.
  Regenerate with `make golden` and **review the diff; it is the contract.**
- Class (a) checks are unit-tested with `probes.Fake` — no network in CI, ever
- Class (b) checks are tested with recorded fixtures captured from a lab and
  scrubbed by a committed script
- Anything needing a live lab is `//go:build integration` and never runs in CI

See [test-coverage.md](test-coverage.md) for the full tiering and the current
gaps.

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

## Known debt

Recorded so absences are decisions rather than oversights.

- Matrix rows are not re-annotated with topology axes
  ([ADR-0011](ADR/0011-topology-axes.md)) or layer tags
  ([ADR-0012](ADR/0012-supervisor-vks-layers.md)).
- VKS-layer requirement rows are not written up at all — see the Layer section
  of the matrix.
- No test drives the interactive prompt flow; it is exercised manually. This is
  the largest untested surface in the tool.
- `results.WriteBaseline`'s ordering guarantee is load-bearing for drift and has
  no round-trip test.
- ~~Nothing verifies that a check's cited requirement IDs actually exist in the
  matrix.~~ **Closed.** `internal/docs` fails the build on an invented ID, and
  on summary tables that have drifted from the registry.
- The `blockedBy` map in `internal/docs/matrix_test.go` is hand-maintained. It
  has to be — "this needs an NSX client" is a judgement about work not done,
  not a fact derivable from work done. A missing entry degrades to `—` rather
  than to a wrong claim.

## Open questions

These change what gets built and are not ours to decide unilaterally.

1. ~~Is `vds+haproxy` in scope?~~ **Resolved:** yes, operator-confirmed. HAProxy
   is not removed in VCF 9 — it is fully supported on vCenter 8.x and being
   phased out starting with 9.x. See `LB-HAP-000`, implemented by
   `hap.version-supported` (warns, never blocks).
2. Should duplicate-IP detection be `--invasive`? See `COM-ADR-001`.
3. Should preflight check the *deployment* account's vCenter privileges, not
   just the tool's own read access? See `COM-API-001`.
4. Two matrix rows need config fields that do not exist (declared DHCP scopes):
   `SUP-MGT-003`, `VDS-DHCP-001`. Add the fields or drop the rows.
5. Where do version and port matrices come from — shipped with a release, or
   supplied by the user? They must be data, never code — with one narrow
   exception now settled in
   [ADR-0015](ADR/0015-flagged-rows-and-version-constants.md): a single
   existence or support-lifecycle boundary for one named product may be a
   constant. `flb.version-supported` and `hap.version-supported` are the two,
   and a third that does not fit that shape is evidence the carve-out was too
   generous rather than precedent for widening it.
6. Is a verify-mode result that is *better* than declared a pass or a drift?
7. What exit code does `drift` use?
8. `externalCIDRs` semantics: a site declaring `10.0.0.0/8` as must-not-collide
   while deploying management into `10.50.0.0/24` gets a blocker. Correct by the
   letter, arguably not what they meant. Should deployment networks be
   auto-excluded from their own external comparison?
9. `LICENSE` is a placeholder.
