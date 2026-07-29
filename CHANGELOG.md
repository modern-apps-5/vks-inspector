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
`loadBalancer` (`nsx-lb` | `alb` | `haproxy`). Unsupported combinations are
rejected rather than assumed workable. Supported-but-unverified combinations
(HAProxy, VPC-based networking) pass with the caveat carried into the report.

**Supervisor / VKS layer.** Every check declares whether it is a Supervisor
enablement prerequisite or a VKS workload-cluster one. `--layer
supervisor|vks|both` filters a run. Most checks are Supervisor-layer — there is
no VKS without a Supervisor.

**Checks (5).** All are pure config arithmetic; no network probe or credentialed
check is implemented yet.

| Check | Requirements |
|---|---|
| `cidr.overlap` | `COM-CID-001` |
| `cidr.external-collision` | `COM-CID-002` |
| `cidr.infra-collision` | `COM-CID-003` |
| `range.containment` | `COM-CID-005`, `LB-VIP-001` |
| `meta.topology-recognised` | `MET-001` |

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

**Documentation** — `docs/REQUIREMENTS-MATRIX.md` (89 rows, 46 flagged as
unverified), `docs/CHECK-TAXONOMY.md`, `docs/test-coverage.md`, and 14 ADRs.

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

### Fixed

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
- A network's own static range was reported as colliding with its own subnet.
  Containment there is required, not a conflict; the check now excludes
  parent/child pairs and `range.containment` asserts the containment separately.
- A run in which every check was skipped reported "no blockers, no warnings".
  Technically true and misleading — it now states that the run inspected nothing
  and is not evidence of readiness.
- Interactive section headings printed even when no question under them was
  asked, producing empty headings for sections already answered by config.

### Known limitations

- No network probes and no credentialed checks. The vCenter, NSX and ALB clients
  are stubs, so every credentialed check reports as a skip with a reason.
- 46 of 89 requirement rows are flagged as unconfirmed against current VCF 9 /
  VKS documentation. Checks are not built on flagged rows.
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
