# Architecture Decision Records

Short records of decisions that would be expensive to reverse. One file per
decision, numbered. Once accepted, the decision itself is not rewritten — a
decision that changes gets a new record that replaces the old one, and the old
one stays so you can follow the reasoning.

Each one is: Context (what forced the decision) → Decision (what we did) →
Consequences (what it costs us, including what it makes harder).

The Consequences section is not a formality. A record that lists only benefits is
advertising.

| # | Decision | Status |
|---|---|---|
| [0001](0001-go-single-binary.md) | Go, single static binary | Accepted |
| [0002](0002-mode-parametric-checks.md) | One check works in every mode | Accepted |
| [0003](0003-structured-observations.md) | Results carry data, not just sentences | Accepted |
| [0004](0004-pluggable-renderers.md) | Formatting lives in one place; the UI just reads the JSON | Accepted |
| [0005](0005-credential-handling.md) | Credentials never get written to any file | Accepted |
| [0006](0006-exit-code-contract.md) | Exit codes are fixed, and "could not tell" is not "failed" | Accepted |
| [0007](0007-read-only-by-default.md) | Read-only by default; invasive probes are gated | Accepted |
| [0008](0008-requirements-matrix-authority.md) | The requirements matrix is the master list | Accepted |
| [0009](0009-baseline-artifact.md) | A baseline is just a Report, not a second format | Accepted |
| [0010](0010-explicit-check-registration.md) | Explicit registration, no `init()` magic | Accepted |
| [0011](0011-topology-axes.md) | Topology is two separate settings, not one list | Accepted |
| [0012](0012-supervisor-vks-layers.md) | Requirements are tagged Supervisor or VKS | Accepted |
| [0013](0013-prompting-produces-config.md) | Prompting produces a config, it is not an alternative to one | Accepted |
| [0014](0014-vcenter-first-discovery.md) | vCenter is the starting point; find the rest from it | Accepted, partly implemented |
| [0015](0015-flagged-rows-and-version-constants.md) | What a flag blocks, and when a version may be a constant | Accepted, refines 0008 |
