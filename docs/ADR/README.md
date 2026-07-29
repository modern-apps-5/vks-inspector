# Architecture Decision Records

Short records of decisions that are expensive to reverse. One file per decision,
numbered, never edited after acceptance — a decision that changes gets a new ADR
that supersedes the old one, and the old one stays so the reasoning trail
survives.

Format: Context (what forced the decision) → Decision (what we did) →
Consequences (what this costs us, including what it makes harder).

The Consequences section is not a formality. An ADR that lists only benefits is
advertising, not a record.

| # | Decision | Status |
|---|---|---|
| [0001](0001-go-single-binary.md) | Go, single static binary | Accepted |
| [0002](0002-mode-parametric-checks.md) | Checks are mode-parametric, not preflight-specific | Accepted |
| [0003](0003-structured-observations.md) | Observations are structured values, not prose | Accepted |
| [0004](0004-pluggable-renderers.md) | Renderers are pluggable; the UI is a JSON consumer | Accepted |
| [0005](0005-credential-handling.md) | Credentials never touch the config or any artifact | Accepted |
| [0006](0006-exit-code-contract.md) | Exit codes are a contract, and indeterminate ≠ failed | Accepted |
| [0007](0007-read-only-by-default.md) | Read-only by default; invasive probes are gated | Accepted |
| [0008](0008-requirements-matrix-authority.md) | The requirements matrix is the source of truth | Accepted |
| [0009](0009-baseline-artifact.md) | A baseline is a Report, not a second format | Accepted |
| [0010](0010-explicit-check-registration.md) | Explicit registration, no `init()` magic | Accepted |
