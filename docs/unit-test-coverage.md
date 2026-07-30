# Test coverage

How vks-inspector is tested and, more to the point, which checks *can* be tested
without a lab, which need recorded API responses, and which honestly cannot be
tested outside a live environment.

This file was rewritten around [the three kinds of check](check-types.md). The
previous version was a flat checklist of checks we wanted, with nothing about who
could run them, what they claimed, or how anyone knew they worked. That list is
not lost — every item in it became a requirement row in
[REQUIREMENTS-MATRIX.md](REQUIREMENTS-MATRIX.md), which is now the master list of
what we check. **This file is about how we know the code works**, which is a
different question, and the reason the two were split.

---

## The three tiers

| Tier | Needs | Runs in CI | Command | Build tag |
|---|---|---|---|---|
| **1 — Unit** | Nothing | Yes, always | `make test` | none |
| **2 — Fixture** | Recorded API responses committed to the repo | Yes, always | `make test` | none |
| **3 — Integration** | A live vSphere / NSX / ALB lab | **No, never** | `make test-integration` | `integration` |

Tier 3 is kept out of CI by a build tag, not by skipping at runtime. A test that
compiles into the normal build and skips itself when it cannot find a lab counts
towards the test total and proves nothing. `//go:build integration` means it is
not there at all unless you ask for it.

**A coverage percentage is not the goal.** It would be trivially inflated by
testing the stub packages. What matters is that every network check and config
check is tested against real logic, and that every API check has a recorded
response proving it parses the real thing correctly.

---

## Tier 1 — Unit tested, no external dependencies

Everything that is arithmetic, parsing, formatting or dispatch.

### Currently covered

| Area | File | What it checks |
|---|---|---|
| Exit codes | `internal/results/exitcode_test.go` | The full precedence table, including the case everything else rests on: a blocker that *could not tell* is exit 2, not exit 1 |
| Result roll-up | `internal/results/exitcode_test.go` | Summary counts per status and severity |
| Renderer output | `internal/renderers/golden_test.go` | Byte-exact terminal, JSON and JUnit output against golden files |
| Renderer stability | `internal/renderers/golden_test.go` | The same report renders identically 20 times — no map ordering or clock values leaking in |
| JSON completeness | `internal/renderers/golden_test.go` | JSON never omits skipped results whatever the options say |
| Config loading | `internal/config/load_test.go` | The example config parses; mistyped keys are rejected rather than ignored; the digest depends on content, not formatting |
| Credential safety | `internal/creds/creds_test.go` | Redaction under `%v %s %+v %#v %q`; refusal to marshal; env-over-file precedence; refusal to read a 0644 file |
| Registry selection | `internal/registry/registry_test.go` | Mode, topology, `--only`, `--skip`; every check accounted for; registration guards |
| Engine behaviour | `internal/engine/engine_test.go` | End-to-end run; every mode; panics and silent checks become tool errors; skips carry reasons; severity overrides recorded |
| Reference check | `internal/checks/reference/reference_test.go` | Behaves the same in all three modes; observations come out as comparable data; the clock is injected; unsupported topology combinations are rejected |
| Address arithmetic | `internal/netx/netx_test.go` | Overlap, containment, range sizing — including the off-by-one family: adjacency, single-address touch, /31, /32, IPv4-vs-IPv6, host bits set, IPv6 counts exceeding int64 |
| Prompts | `internal/prompt/prompt_test.go` | Optional vs required lists; examples shown in labels; validators clean up input and ask again; non-interactive errors instead of guessing; section headings only print when used |
| Answer cleanup | `internal/prompt/elicit_internal_test.go` | A bare address becomes `/32`; a CIDR with host bits set is refused and the corrected form is named; strict vs lenient CIDR |
| Network checks | `internal/checks/network/network_test.go` | All eight, via `probes.Fake`: DNS timeout vs NXDOMAIN, resolving to the wrong address, querying each resolver separately, PTR case matching, severity raised by config, the three TCP outcomes, unsynchronised NTP sources, and skew in both directions |
| vCenter checks | `internal/checks/vcenter/vcenter_test.go` | All five against vcsim, end to end through real SOAP |
| Address-plan checks | `internal/checks/configval/cidr_test.go` | All four checks that exist: overlap, external collision, infra collision, containment. Also one row per problem, and skip rather than pass when there is nothing to compare |
| FLB version boundary | `internal/checks/flb/flb_test.go` | Blocks below vCenter 9.0, passes at and above; unreachable vCenter and unparseable versions report `unknown` rather than a verdict; applicability is `flb`-only |
| HAProxy version boundary | `internal/checks/alb/alb_test.go` | Passes on vCenter 8.x, warns on 9.x; **checks the severity stays `warning`** so being phased out can never fail a working deployment on its own |
| Matrix matches the code | `internal/docs/matrix_test.go` | Every requirement ID a check names exists in the matrix, and the generated summary tables match the registry. Both were confirmed to fire by breaking each on purpose |

### The two patterns, already established

**Table test** — `internal/results/exitcode_test.go`. One named case per
behaviour, one thing checked, no shared state between cases, and a failure
message that reads as a sentence. Every config check gets one of these.

**Golden file** — `internal/renderers/golden_test.go`. Output is compared byte for byte
against a committed file, `make golden` regenerates it, and the diff is the
review. Those files *are* the output people depend on: a field engineer reads the
terminal under time pressure, and a CI job parses the JSON without reading
anything at all. An unexplained change to either is a regression.

### What must be unit-tested as checks land

Every config check, without exception. They are pure functions over a config
struct, so there is no excuse for one to be untested and no reason for one to be
slow.

The highest-value cases are the nasty ones, and they should be written before
the implementations:

- CIDR overlap where one range **contains** another rather than partially
  intersecting it
- Adjacent-but-not-overlapping ranges (`10.0.0.0/24` and `10.0.1.0/24`) — the
  classic off-by-one
- IPv6, and mixed IPv4/IPv6 configs
- A `/31` and a `/32`
- Ranges whose start is greater than their end
- A range that is syntactically valid and semantically absurd (`0.0.0.0/0`)
- Empty and absent optional blocks — a missing `frontend` under `vds-alb` must
  produce a clear result, not a nil dereference
- Sizing checks with **no declared scale** — must skip, never pass

For network checks, unit testing means swapping `probes.System` for
`probes.Fake`. That swap exists for exactly this reason: the whole network-check
suite can be tested on a laptop with no network. Every probe needs these cases
covered:

- Success
- Explicit failure (NXDOMAIN, connection refused)
- **Timeout / silence** — must produce `unknown`, never `fail`
- Partial success (2 of 3 resolvers answer)
- Permission denied on a raw socket — must produce a skip with a reason

---

## Tier 2 — API simulator, and recorded fixtures where it falls short

API checks are tested against **vcsim**, govmomi's vSphere API simulator, which
speaks the real SOAP protocol and returns real managed-object shapes.

That is better than the hand-written responses originally planned. A hand-written
response tests the parser against *what the author believed* the API returns,
which is the belief most likely to be wrong. vcsim tests it against a separate
implementation. Writing these tests immediately caught four bugs that
hand-written responses would have baked in rather than exposed:

1. finder paths are datacenter-relative, so inventory lookups silently found
   nothing;
2. a DVS may be reported as the base type or the VMware subclass, and
   unmarshalling into the wrong one panics inside the property collector;
3. `MaxMtu` of 0 means "not populated", not "MTU is zero" — reporting it as a
   real value would fail an environment whose MTU was never read;
4. portgroup `name` is not always set (only `config.name`), and an empty name
   silently matched an empty lookup string — one test was green while proving
   nothing.

**What vcsim does NOT prove:** that a real vCenter behaves the same way, that
VCF 9 returns these shapes, or anything at all about authentication. vcsim
accepts any credentials, so it cannot test a login being rejected. An earlier
test claimed to prove bad credentials were refused, and only passed because the
transport was misconfigured. Those questions need tier 3.

Recorded responses are still the right tool where the simulator does not model
something at all — there is no equivalent for NSX or ALB — and they live under
`internal/clients/<component>/testdata/`.

This is what lets credentialed checks be tested in CI without credentials. It is
also the only honest way to test parsing, for the same reason as above: a
hand-written response only tests the parser against what the author believed.

**Rules for recorded responses:**

1. **Recorded, never hand-written.** Capture them from a live lab.
2. **Scrubbed before commit** — hostnames, IPs, serial numbers, certificate
   material, session tokens. The scrubber must be a committed script, not a
   manual pass, so it is repeatable and reviewable.
3. **Labelled with what produced them** — product version and date. A response
   from an unknown version stops being trustworthy the moment behaviour changes.
4. **Capture both** — the healthy response *and* the failure response. A parser
   tested only against success is half tested, and the failure path is the one
   that runs while a customer is watching.
5. **Cover more than one version where it matters.** If a check spans product
   versions, record a response from each.

**What these tests prove:** that the client parses the response correctly, and
that the check turns the parsed state into the right status, severity,
observation and fix. They prove **nothing** about the network — that is tier 3.

**A limitation worth stating rather than hiding:** a recorded response proves the
code handles that *shape* of response. It cannot prove the shape is still
current. Recorded responses go stale quietly, and green CI on a stale one is a
false signal. The only fix is re-capturing against the lab periodically, tracked
as recurring work rather than as a test anyone can write.

---

## Tier 3 — Integration, live lab, excluded from CI

Build tag `integration`. Never runs in CI. Requires a real environment and
credentials in the environment.

```
make test-integration
```

**What genuinely requires a lab:**

| Area | Why a fake is not enough |
|---|---|
| Path MTU discovery | Requires a real path with a real clamping hop. **Invasive.** |
| Duplicate IP / ARP detection | Requires real L2 adjacency and real occupied addresses |
| ICMP and raw-socket probes | Privilege behaviour differs per OS and per container runtime |
| Real DNS resolver behaviour | Timeouts, truncation, TCP fallback, split-horizon |
| NTP offset measurement | Requires real time sources and real clock skew |
| TLS chain validation against real certs | Real chains, real intermediates, real expiry |
| Live vCenter / NSX / ALB clients | Session handling, pagination, API-version negotiation, auth expiry |
| Fixture capture | The lab run *is* how tier 2 fixtures are produced |

**What integration tests must not do:** change anything in the lab. Read-only is
not a CI convenience, it is what the tool promises. An integration test that
creates a portgroup in order to test portgroup detection has broken that promise,
and will eventually be run against production by someone who never read this
file.

---

## Coverage by kind of check

| Kind | Requirements | Tier 1 | Tier 2 | Tier 3 |
|---|---|---|---|---|
| Config checks | ~16 | **All** | — | — |
| Network checks | ~24 | All logic, via `probes.Fake` | — | The probes themselves |
| API checks | ~48 | Check logic, via fake clients | **All** response parsing | Live client behaviour |

Rough counts against the 97 rows in the matrix. A row that needs more than one
kind is counted under each.

These count *requirements*, not checks that exist. 25 rows have a check today.
The per-section summary tables in the matrix are the place to look for which
ones, and they are generated rather than kept up by hand.

---

## What is deliberately not tested

Written down so that what is missing is a decision rather than an oversight.

- **The stub packages.** `internal/checks/{network,vcenter,nsx,alb,configval}`
  return `nil` and have no tests. Testing them would report coverage that does
  not exist.
- **Cobra wiring.** Flag parsing is the framework's problem. The behaviour that
  matters — exit codes — is tested at the `results` layer where it lives.
- **`serve`, `snapshot`, `drift`.** Not built yet. `results.WriteBaseline` and
  `ReadBaseline` should get round-trip tests as soon as `snapshot` is real. Drift
  depends on `WriteBaseline` writing things in a stable order, and nothing tests
  that today. **This is a known gap.**

---

## Gaps in the current test suite

An honest list, as of the phase-1 scaffold:

1. **No round-trip test for the baseline file.** `WriteBaseline` sorts results so
   two files can be compared byte for byte, and nothing checks that it does.
   Should be written before `snapshot` lands.
2. **Nothing checks the JUnit XML** against a schema, or against a real CI
   collector. The mapping involves a judgement call (a failed warning becomes a
   `<failure>`) and no consumer has confirmed it.
3. **No fuzz testing of the config parser.** Worth adding once the format
   settles; a malformed config should never panic.
4. **`--save-config` round-tripping is untested.** The saved file gets re-read
   during manual testing, but no test checks that
   prompt → config → YAML → load → the same config holds.
5. **`Elicit` itself is untested.** The individual prompts are covered, but
   nothing runs a full sequence of questions and checks the `config.Config` that
   comes out — including that a value already given in a config or a flag is
   never asked for again. The FLB questions added most recently are untested for
   the same reason.
6. **`internal/config.assertNoSecrets` never actually fires** in normal use,
   because `yaml.KnownFields(true)` rejects unknown keys first. It is a
   belt-and-braces guard in case a future struct field reintroduces a credential
   field, and the test says so rather than pretending to cover behaviour that
   cannot happen.

---

## Running

```
make test              # tiers 1 and 2 — this is what CI runs
make test-race         # same, with the race detector
make cover             # coverage.html
make golden            # regenerate renderer golden files; review the diff
make test-integration  # tier 3 — needs a live lab, never in CI
make check             # fmt + vet + test
```
