# Changelog

All notable changes to vks-inspector are recorded here.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
This project has not cut a release yet; everything below is unreleased.

## [Unreleased]

### Added

**Command surface** — `check`, `verify`, `snapshot`, `drift`, `explain`,
`serve`. Only `check` is implemented; `explain` works for registered checks.
Unimplemented commands exit 3 (tool error) rather than 0, so a pipeline calling
one by mistake fails loudly instead of recording a spurious pass.

**`--defaults`** accepts each prompt's illustrative example on Enter, so the
whole flow can be walked without inventing answers. **For exercising the CLI
only.** The answers describe no real environment, yet the checks still run and
may report PASS — so a run that used it is marked in three places: a banner
before the questions, a caveat line in the report next to the verdict, and a
label written into the saved config so a file reused weeks later still declares
its own provenance. Optional lists are never auto-filled: fabricating the
"existing networks" list would turn a truthful skip into a false pass.

**Interactive flow.** `vksinspect check --vcenter <fqdn>` prompts for what it
cannot determine, then runs. `--save-config` writes the answers out; the saved
file makes every later run non-interactive. `--non-interactive` turns a missing
value into an error naming the config field rather than applying a silent
default.

**Declarative config** (`vksinspect/v1alpha1`, kind `EnvironmentSpec`). One
document describes intended topology and addressing and drives every mode.
Contains no credentials and no run options. Unknown keys are rejected rather
than ignored.

**Topology as orthogonal axes** — `networking` (`vds` | `nsx` | `nsx-vpc`) ×
`loadBalancer` (`nsx-lb` | `alb` | `haproxy` | `flb`). Unsupported combinations
are rejected rather than assumed workable. Supported-but-caveated combinations
(HAProxy, FLB, VPC-based networking) pass with the caveat carried into the
report.

**Foundation Load Balancer (`flb`)**, the VCF 9.1 load balancer, as a third
`loadBalancer` value paired with `vds` networking. It differs structurally from
ALB and HAProxy and the schema reflects that: FLB runs as VM(s) inside
vCenter's own Supervisor folder and resource pool rather than as a separately
managed controller, so the `flb:` config block carries **no endpoint and no
`credentialRef`** — there is nothing to log into, and the tool's existing
vCenter access covers it. The block declares the arm mode
(`two-arm` | `one-arm` | `one-arm-one-nic`), size, HA, and its virtual-server
and transit networks; those networks feed the existing overlap and containment
arithmetic like any other declared range. Interactive prompting covers the same
ground and never asks for a controller address.

**Load balancer version boundaries are checked, not just documented.** FLB and
HAProxy sit at opposite ends of the same vCenter version line, and both now
read the live version through the existing vCenter client.
`flb.version-supported` **blocks** below vCenter 9.0 — FLB does not exist
there, so the topology cannot work. `hap.version-supported` **warns and never
blocks** from vCenter 9.x, where HAProxy is being phased out: it still works,
it just should not be a new design, and failing a working deployment over a
deprecation would be worse than the deprecation. Both report `unknown` rather
than a verdict when the version cannot be read — an unreachable vCenter is
`vc.api-reachable`'s finding, not theirs.

These two hardcode a version constant, which
[ADR-0008](docs/ADR/0008-requirements-matrix-authority.md) rule 4 otherwise
forbids. The distinction: a product *existence* boundary is not the shifting
patch-level interoperability grid that rule protects against (`COM-VER-001`).
Written up as **[ADR-0015](docs/ADR/0015-flagged-rows-and-version-constants.md)**,
which also settles what a `⚑` flag does and does not block — four shipped checks
cite a flagged row, and all four are legitimate because they parameterise the
doubted dimension rather than asserting it.

**Coverage is generated and enforced.** Each section of
`docs/REQUIREMENTS-MATRIX.md` now opens with a summary table whose **Status**
column says what this build does about every row — implemented (naming the
check), `ready`, `vantage`, blocked on a named client, or `confirm first`. The
tables are generated from the check registry by `make matrix` and **verified by
`make test`**, so a coverage claim cannot drift from the code the way a
hand-maintained one does. Two guards ship with it, both closing debt items that
`docs/CONTRIBUTING.md` had recorded as open: a check citing a requirement ID
that exists in no row now fails the build, and so do stale tables.

**Supervisor / VKS layer.** Every check declares whether it is a Supervisor
enablement prerequisite or a VKS workload-cluster one. `--layer
supervisor|vks|both` filters a run. Most checks are Supervisor-layer — there is
no VKS without a Supervisor.

**vCenter client and discovery.** `--vcenter` now actually connects. The client
is read-only — the only write is the session itself, which is closed on exit so
the tool does not leave sessions accumulating on a customer's vCenter. It reads
version, datacenters, clusters, hosts, distributed switches, portgroups, and any
registered NSX Manager. Discovered values pre-fill the question flow and are
reported rather than asked; a discovered value never overwrites a declared one.

**`--insecure-skip-tls-verify`**, and a prompt for it during credential entry.
Self-signed management-plane certificates are the norm in labs, and refusing to
connect to them made every credentialed check unreachable there. When
verification is off, `tls.chain` reports **`SKIP` with a reason** for that
endpoint rather than a pass — honouring what ADR-0005 promised: an unverified
connection cannot evidence a valid chain. `tls.expiry` still reports and states
that it covers expiry only.

**`--force`** lets `--save-config` overwrite. An interactive run asks;
a non-interactive one without `--force` still refuses.

**Credential prompting.** If no credentials are found, the tool asks — username
plainly, password with terminal echo disabled — and offers to save them to
`~/.vksinspect/credentials.yaml` at mode 0600. Saving is opt-in; writing
someone's password to disk without asking is not a decision this tool makes for
them. Refuses to read a password from a non-terminal, where it would be
invisible to the person running the command. The secret still never reaches the
config, a report or a baseline. This supersedes ADR-0013's "never prompt for a
password" — the rules that protect the secret are unchanged.

**Probe memoisation and concurrency.** Several checks legitimately need the same
observation (`dns.forward` and `dns.resolver-agreement` resolve the same names;
`tls.chain` and `tls.expiry` inspect the same certificates), and fan-outs now
run with bounded concurrency. A run against unreachable endpoints went from over
two minutes to twenty seconds.

**Network probes.** DNS (forward, reverse, cross-resolver agreement), TCP
tri-state connect, TLS chain inspection and expiry, and real SNTP. Confined to
`internal/probes` behind an interface, so the whole class-(a) suite is testable
with a fake — no network, no lab, runs in CI.

**Checks (20).** Five config-only, eight network, seven requiring vCenter
credentials. See the README for a one-line description of each.

| Check | Requirements | Needs |
|---|---|---|
| `cidr.overlap` | `COM-CID-001` | — |
| `cidr.external-collision` | `COM-CID-002` | — |
| `cidr.infra-collision` | `COM-CID-003` | — |
| `range.containment` | `COM-CID-005`, `LB-VIP-001` | — |
| `meta.topology-recognised` | `MET-001` | — |
| `vc.api-reachable` | `COM-API-001` | vCenter |
| `vc.cluster-exists` | `INV-VC-001` | vCenter |
| `vc.vds-exists` | `INV-VC-002` | vCenter |
| `vc.vds-mtu` | `COM-MTU-003` | vCenter |
| `vc.portgroup-exists` | `VDS-PG-001/002/003` | vCenter |
| `flb.version-supported` | `LB-FLB-000` | vCenter |
| `hap.version-supported` | `LB-HAP-000` | vCenter |
| `dns.forward` | `COM-DNS-001` | network |
| `dns.reverse` | `COM-DNS-002` | network |
| `dns.resolver-agreement` | `COM-DNS-005` | network |
| `tcp.port-open` | `COM-FW-001/002` | network |
| `tls.chain` | `COM-CRT-001/002` | network |
| `tls.expiry` | `COM-CRT-003` | network |
| `ntp.reachable` | `COM-NTP-001` | network |
| `ntp.skew` | `COM-NTP-002` | network |

**Renderers** — human terminal, JSON, JUnit XML. Renderers are pure: the same
report produces byte-identical output every time. JSON never omits skipped
results regardless of options, so a machine consumer can always distinguish
"passed" from "never ran".

**Exit code contract** — `0` all passed, `1` a blocker failed, `2` warnings or
indeterminate only, `3` tool error. An indeterminate result never produces `1`
even for a blocker-severity check: a filtered port is a firewall, not proof a
service is down, and the tool does not assert a failure it did not observe.

**Credential handling.** Credentials come from a `0600` file or `VKSINSPECT_*`
environment variables, never from the config. They redact under every format
verb, refuse to serialise into any artifact, and a group- or world-readable
credentials file is refused rather than warned about.

**Baseline artifact format.** A baseline is a `results.Report` with
`kind: vksinspect.baseline/v1` — one artifact type, not two. Results are sorted
for byte-comparability and the config digest distinguishes "the environment
changed" from "the declared intent changed". Written now so `snapshot` and
`drift` have a fixed target.

**Documentation** — `docs/REQUIREMENTS-MATRIX.md` (97 rows, 49 flagged as
unverified, 25 implemented), `docs/CHECK-TAXONOMY.md`, `docs/test-coverage.md`,
and 15 ADRs.

### Changed

- **README** rewritten. It previously described a tool that did not exist. It
  now states what works, what does not, and that most of what is checked are
  Supervisor prerequisites rather than VKS ones.
- **`docs/test-coverage.md`** rewritten against the check taxonomy. The previous
  flat checklist of desired checks became requirement rows in the matrix; this
  file now covers how the code is verified.
- **Contributing pointer** corrected from GitLab to GitHub — the previous README
  contradicted itself by naming GitLab while linking to github.com.
- **`config/example.yaml`** addressing reworked. The original declared a pod
  CIDR inside a range it also declared as must-not-collide, so the shipped
  example failed its own checks.
- **HAProxy's status corrected from guess to fact.** The matrix had flagged the
  whole topology as "believed deprecated or removed in the VCF 9 generation"
  and suggested deleting it. Operator-confirmed: it is *not* removed — fully
  supported on vCenter 8.x, phased out starting on 9.x. `LB-HAP-000` is
  rewritten, no longer flagged, and implemented. The Data Plane API rows below
  it stay flagged on their own separate merits.
- **The backlog moved into the requirements matrix** rather than living in a
  file beside it. A separate coverage document duplicates every row's state and
  drifts from the code within a release — the same failure mode as any
  hand-maintained index. Status now sits on the row it describes, generated
  from the registry. `docs/CONTRIBUTING.md` keeps only what is genuinely not a
  per-row fact: which single client unlocks the most rows, and the one design
  decision blocking six otherwise-ready rows.
- **Two ID shapes in the matrix are now handled everywhere.** Rows are mostly
  three-segment (`COM-DNS-001`) but four are two-segment (`MET-001`,
  `NSX-T0-00x`). Any tooling that assumed three silently dropped those four and
  under-reported coverage.
- **Failure messages state the fault, not the rule.** A blocker headed "No two
  declared ranges overlap" made the reader diff the heading against the
  observation to find out what had happened. Headings are now generated from
  the finding — "The workload-primary network sits entirely inside the
  Kubernetes service CIDR".
- **Overlaps are classified.** Containment, partial overlap and an identical
  range have different causes and different fixes; reporting all three as
  "overlaps" left the reader to compare prefixes in their head. The finding now
  names the relationship and the extent, in addresses.
- **Results carry an `impact`**, separate from `remediation`. "Why it matters"
  and "what to do" are different questions, and merging them buried the first.
  The consequence is stated in terms of what stops working, and differs by what
  collided — a cluster-internal range swallowing a real host is not the same
  failure as two routable networks colliding.
- **Remediation names which side to move** and why that one, rather than saying
  "re-plan the address space".

### Fixed

- **`--insecure-skip-tls-verify` never reached the checks.** It was applied only
  to the credential handed to the client, so the certificate checks never
  learned verification was disabled and went on asserting a chain nobody had
  verified. The flag now lands on the credential set the checks see, and is also
  carried on the run context so it applies before any credential exists.
- **An IP typed into the `fqdn` field was not recognised as an address.**
  Operators routinely do it, and the consequences are identical to using the
  `ip` field: nothing to resolve, and a certificate validated against an
  address. Detection now keys on the host actually used.
- **`dns.forward` "resolved" IP literals and counted them as passes.** Resolving
  an address always succeeds and proves nothing; a config whose only endpoint
  was an IP produced "1 name resolved correctly" out of thin air. IP literals
  are no longer DNS targets, and the check skips when no real name remains.
- **The IP-versus-hostname diagnosis missed certificates carrying only a common
  name** — which is the case it exists for. It now falls back to the CN.
- **A wrong stored password was a dead end**: the tool loaded it, failed, and
  never asked again. Authentication failures now offer a retry, and `--relogin`
  forces a fresh prompt.
- **`--non-interactive` refused to run a config missing any optional field.** A
  saved config lacking only a management IP range would not run at all. Absence
  is now reported and flows through to checks that skip with a reason.
- **Reports did not say what access a run lacked.** The coverage line now names
  it — "no vcenter access — 4 check(s) could not run, so nothing above covers
  it" — because "how are these checks OK if the vCenter one is not?" is the
  first question a reader asks.

- **Thin passes read as broad ones.** `dns.forward` reported "1 name resolved
  correctly" without mentioning that the vCenter had been declared by IP and so
  was never name-checked. It now names every endpoint it could not cover. Same
  for `tls.expiry`, which now says when a chain was unverified rather than
  sitting next to a `tls.chain` failure looking like a clean bill of health.
- **A chain failure caused by connecting via IP is now diagnosed precisely.**
  A certificate is validated against the address connected to, so reaching
  `10.47.0.200` when the certificate is issued for `vc01.gpu.set.lab` fails SAN
  matching even when the certificate is fine. The finding now names both the
  mismatch and the hostname to use instead of offering generic advice.
- **`--save-config` could not overwrite**, making a re-run of the wizard
  needlessly painful. It now asks interactively, and `--force` skips the question.

- **The report read as a readiness verdict for work it had not done.** A run of
  pure config arithmetic printed "4 passed, 0 failed / all checks passed" with
  nothing to indicate that no packet had been sent and no API called. Reports
  now carry a `coverage` block and state, next to the verdict, when nothing in
  the run contacted the environment. The all-clear text changed from "all checks
  passed" to "the declared configuration is internally consistent / the
  environment itself has not been inspected", and `ExitCodeText(0)` from "all
  checks passed" to "no check failed" — a run that skipped most of its checks
  never had full coverage.
- **`--vcenter` was accepted and silently never used.** Supplying an endpoint
  the tool cannot yet contact now warns at the point it is supplied.

- **An "optional" prompt refused an empty answer.** The external-networks
  question said "leave empty to skip" and then rejected empty as `required`,
  stranding the operator with no way past it. `AskListOptional` is now a
  distinct call from `AskList` so the promise and the behaviour cannot drift
  apart again.
- **Prompts showed no expected format.** Every free-text question now carries an
  `(e.g. …)` example, and a `required` message names the shape it wants instead
  of only saying "required".
- Prompt answers are now normalised as well as validated: a bare address is
  accepted where a single host is a legitimate answer and stored as `/32`. A
  prefix with host bits set is still refused — `192.168.200.5/24` is genuinely
  ambiguous — but the error names the masked form so the fix is one edit away.
- **A saved config re-prompted for a question that had already been answered.**
  An empty `externalCIDRs` was indistinguishable from an unanswered one, so
  every re-run asked again — defeating the point of `--save-config`. `nil` now
  means "never asked" and `[]` means "asked, answer was none", and the empty
  list survives a YAML round trip.
- **A separate `--probe-timeout`.** The per-check timeout (60s) was being used
  to bound individual probes, so one DNS lookup could block for a minute.
  Probes now default to 5s; `--timeout` still bounds a whole check.
- **`--credentials` pointing at a file that does not exist yet was fatal** —
  which is exactly the state before the first save. Absent is now treated as
  empty, with a note naming the file so a typo'd path is still visible.
- A network's own static range was reported as colliding with its own subnet.
  Containment there is required, not a conflict; the check now excludes
  parent/child pairs and `range.containment` asserts the containment separately.
- A run in which every check was skipped reported "no blockers, no warnings".
  Technically true and misleading — it now states that the run inspected nothing
  and is not evidence of readiness.
- Interactive section headings printed even when no question under them was
  asked, producing empty headings for sections already answered by config.

### Known limitations

- No NSX or ALB client, so those checks report as skips with a reason. Building
  the NSX client unlocks 7 requirement rows — the largest single coverage win
  available. FLB needs no client of its own; it is read through vCenter.
- No ICMP, duplicate-IP or path-MTU probes — all need raw sockets, and the
  privilege-degradation path is unwritten.
- Probes run only from wherever the tool is invoked. Several requirements need a
  specific vantage (the workload segment, outside the segment) and the tool
  cannot yet be told it is standing somewhere else. **Six otherwise-ready rows
  are blocked on this** and are marked `vantage` in the matrix rather than
  implemented — writing them as ordinary local probes would produce exactly the
  false green this tool exists to prevent.
- **FLB coverage is one row deep.** The version boundary is checked; VM
  placement and health, the arm-mode network requirements, and VIP allocation
  are not. Four of the six `LB-FLB-*` rows are flagged — the published
  architecture describes intent and topology but not the vCenter object model a
  check would have to read.
- vCenter behaviour is verified against **vcsim**, govmomi's API simulator. That
  genuinely exercises request construction and response parsing, but it is a
  model, not a real vCenter, and proves nothing about VCF 9 or real
  authentication. Live-lab integration tests remain necessary and unwritten.
- **25 of 97 requirement rows are implemented; 49 are flagged** as unconfirmed
  against current VCF 9 / VKS documentation. The binding constraint on coverage
  is not engineering time — 45 uncovered rows are blocked on confirming the
  requirement itself, and no amount of code moves those.
- Checks are not built on flagged rows, with four deliberate exceptions where
  the check **parameterises the uncertain dimension instead of asserting it**:
  `ntp.skew` takes its tolerance from config rather than inventing a threshold,
  `dns.reverse` defers severity to `services.dns.requireReverse`,
  `dns.resolver-agreement` stays a warning exactly as its flag instructs, and
  `vc.api-reachable` checks only the tool's own access and says so. The rule
  that applies: a row flagged on a dimension the check does not assert is not
  blocked by that flag. Documented in the matrix under *Status keys*.
- VKS-layer requirement rows are not yet written; the matrix says so explicitly.
- The interactive prompt flow is now covered (`internal/prompt`), but the
  end-to-end `--save-config` round trip is not.
- **A passing run is not a validated environment.**

### Security

- Read-only against the target environment. No writes, no configuration changes.
- No outbound internet calls at runtime. The tool probes only what the config
  declares.
- Probes that could disturb a production network sit behind `--invasive` and are
  skipped *visibly* by default, so "not run" is never mistaken for "passed".

[Unreleased]: https://github.com/modern-apps-5/vks-inspector/commits/main
