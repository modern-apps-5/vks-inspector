# Test coverage

How vks-inspector is tested, and — more importantly — which checks *can* be
tested without a lab, which need recorded fixtures, and which are honestly
untestable outside a live environment.

This document was rewritten against the check taxonomy. The previous version was
a flat checklist of desired checks with no notion of who can run them, what they
assert, or how they are verified. That list is not lost: every item in it became
a requirement row in [REQUIREMENTS-MATRIX.md](REQUIREMENTS-MATRIX.md), which is
now the authoritative "what do we check" document. **This file is about how we
know the code works** — a different question, and the reason the two were split.

---

## The three tiers

| Tier | Needs | Runs in CI | Command | Build tag |
|---|---|---|---|---|
| **1 — Unit** | Nothing | Yes, always | `make test` | none |
| **2 — Fixture** | Recorded API responses committed to the repo | Yes, always | `make test` | none |
| **3 — Integration** | A live vSphere / NSX / ALB lab | **No, never** | `make test-integration` | `integration` |

Tier 3 is excluded from CI by build tag, not by a skip at runtime. A test that
compiles into the default build and skips itself when it cannot find a lab looks
like coverage in the test count and provides none. `//go:build integration`
means it is not there at all unless asked for.

**Coverage percentage is not a goal here.** It would be trivially inflated by
testing the stub packages. What matters is that every *class (a)* and *class (c)*
check has real assertions against real logic, and that every *class (b)* check
has a fixture proving it parses a real API response correctly.

---

## Tier 1 — Unit tested, no external dependencies

Everything that is arithmetic, parsing, formatting or dispatch.

### Currently covered

| Area | File | What it pins |
|---|---|---|
| Exit-code contract | `internal/results/exitcode_test.go` | The full precedence table, including the load-bearing case that an *indeterminate blocker* is exit 2, not exit 1 |
| Result roll-up | `internal/results/exitcode_test.go` | Summary counts per status and severity |
| Renderer output | `internal/renderers/golden_test.go` | Byte-exact terminal, JSON and JUnit output against golden files |
| Renderer purity | `internal/renderers/golden_test.go` | Same report renders identically 20 times — no map-order or clock leakage |
| JSON completeness | `internal/renderers/golden_test.go` | JSON never omits skipped results whatever the options say |
| Config loading | `internal/config/load_test.go` | The shipped `config/example.yaml` parses; typo'd keys are rejected, not ignored; digest is content-stable and formatting-insensitive |
| Credential safety | `internal/creds/creds_test.go` | Redaction under `%v %s %+v %#v %q`; refusal to marshal; env-over-file precedence; refusal to read a 0644 file |
| Registry selection | `internal/registry/registry_test.go` | Mode, topology, `--only`, `--skip`; every check accounted for; registration guards |
| Engine behaviour | `internal/engine/engine_test.go` | End-to-end run; every mode; panics and silent checks become tool errors; skips carry reasons; severity overrides recorded |
| Reference check | `internal/checks/reference/reference_test.go` | Identical behaviour in all three modes; machine-comparable observations; injected clock |

### The two patterns, already established

**Table test** — `internal/results/exitcode_test.go`. Named case per behaviour,
one assertion, no shared mutable state, failure message that reads as a
sentence. Every class (c) check gets one of these.

**Golden file** — `internal/renderers/golden_test.go`. Output compared byte for
byte against a committed file; `make golden` regenerates; the diff is the
review. The golden files *are* the output contract — a field engineer reads the
terminal under time pressure and a CI job parses the JSON without reading
anything, so an unexplained change to either is a regression.

### What must be unit-tested as checks land

Every class (c) check, without exception. They are pure functions over a config
struct; there is no excuse for one to be untested and no reason for one to be
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

For class (a) checks, unit testing means substituting `probes.Fake` for
`probes.System`. That seam exists for exactly this reason: the entire
network-check suite is testable on a laptop with no network. Cases that must be
covered per probe:

- Success
- Explicit failure (NXDOMAIN, connection refused)
- **Timeout / silence** — must produce `unknown`, never `fail`
- Partial success (2 of 3 resolvers answer)
- Permission denied on a raw socket — must produce a skip with a reason

---

## Tier 2 — Recorded API fixtures

Class (b) checks are tested against **real API responses captured from a lab and
committed to the repo** under `internal/clients/<component>/testdata/`.

This is what makes credentialed checks CI-testable without credentials. It is
also the only honest way to test parsing: a hand-written fixture tests the
parser against the author's belief about the API, which is precisely the belief
most likely to be wrong.

**Rules for fixtures:**

1. **Captured, never hand-written.** Record from a live lab.
2. **Scrubbed before commit** — hostnames, IPs, serial numbers, certificate
   material, session tokens. The scrubber must be a committed script, not a
   manual pass, so it is repeatable and reviewable.
3. **Labelled with what produced them** — product version, and the date. A
   fixture from an unknown version is untrustworthy the moment behaviour changes.
4. **Both shapes captured** — the healthy response *and* the failure response.
   A parser tested only against success is half tested, and the failure path is
   the one that runs when a customer is watching.
5. **Version-diverse where it matters.** Where a check spans product versions,
   fixtures from each.

**What fixture tests assert:** that the client parses the response correctly,
and that the check maps the parsed state to the right status, severity,
observation and remediation. They do **not** assert anything about the network —
that is tier 3.

**Known limitation, stated rather than papered over:** a fixture proves the code
handles a response *shape*. It cannot prove the shape is still current. Fixtures
go stale silently, and a green CI on a stale fixture is a false signal. Mitigation
is a periodic re-capture against the lab, tracked as recurring work — not a test
we can write.

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

**What integration tests must not do:** modify the lab. The read-only constraint
is not a CI convenience, it is the product promise. An integration test that
creates a portgroup to test portgroup detection has broken the tool's core
guarantee and will eventually be run against production by someone who did not
read this file.

---

## Coverage by requirement class

| Class | Requirements | Tier 1 | Tier 2 | Tier 3 |
|---|---|---|---|---|
| (c) config validation | ~16 | **All** | — | — |
| (a) network-only | ~24 | All logic, via `probes.Fake` | — | Probe implementations |
| (b) credentialed | ~48 | Check logic, via fake clients | **All** response parsing | Live client behaviour |

Rough counts against the 88 rows in the matrix. Rows appearing in multiple
classes are counted in each.

---

## What is deliberately not tested

Stated so that absences are decisions rather than oversights.

- **The stub packages.** `internal/checks/{network,vcenter,nsx,alb,configval}`
  return `nil` and have no tests. Testing them would report coverage that does
  not exist.
- **Cobra wiring.** Flag parsing is the framework's problem. The behaviour that
  matters — exit codes — is tested at the `results` layer where it lives.
- **`serve`, `snapshot`, `drift`.** Not implemented. `results.WriteBaseline` and
  `ReadBaseline` should get round-trip tests as soon as `snapshot` is real; the
  ordering guarantee in `WriteBaseline` is load-bearing for drift and untested
  today. **This is a known gap.**

---

## Gaps in the current test suite

Honest list, as of the phase-1 scaffold:

1. **No round-trip test for the baseline artifact.** `WriteBaseline` sorts
   results for byte-comparability and nothing verifies it. Should be written
   before `snapshot` lands.
2. **No test that the JUnit XML actually validates** against a schema or is
   accepted by a real CI collector. The mapping is opinionated (a failed warning
   is a `<failure>`) and unverified against any consumer.
3. **No test that every registered check's `RequirementIDs` exist in the
   matrix.** The registry panics on an *empty* list but cannot detect an
   invented ID. This becomes checkable once the matrix has a machine-readable
   form — see the `explain` TODO.
4. **No fuzz testing of the config parser.** Worth adding once the schema
   settles; a malformed config should never panic.
5. **`internal/config.assertNoSecrets` is effectively unreachable** in normal
   use, because `yaml.KnownFields(true)` rejects unknown keys first. It is a
   belt-and-braces guard against a future struct field reintroducing a
   credential field, and the test acknowledges this rather than asserting a
   behaviour that does not fire.

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
