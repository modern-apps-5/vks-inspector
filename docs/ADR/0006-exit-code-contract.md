# ADR-0006 — Exit codes are a contract, and indeterminate ≠ failed

**Status:** Accepted · **Date:** 2026-07-29

## Context

The tool is going into pipelines. Exit codes are its real API for automation,
and changing one silently breaks a customer's gate in a way that is very hard
for them to diagnose.

There is also a subtler problem. Network probes have three outcomes, not two: a
port can accept, refuse, or say nothing at all. A silent drop is a firewall, and
it is *not* proof that the service is down. A tool that reports silence as
failure will block deployments over a filtered ICMP path; a tool that reports it
as success will pass an environment it never actually checked.

## Decision

Four codes, fixed:

| Code | Meaning |
|---|---|
| `0` | All checks passed (or were legitimately skipped) |
| `1` | At least one blocker-severity check failed |
| `2` | Warnings failed, or results were indeterminate |
| `3` | Tool error — says nothing about the environment |

Two design consequences:

**Status is separated from Severity.** `Status` is what was observed (`pass`,
`fail`, `skip`, `unknown`, `error`); `Severity` is how much it matters
(`blocker`, `warning`, `info`). A check never decides its own importance —
that is policy, and `config.Policy.SeverityOverrides` can change it at runtime.

**`StatusUnknown` never produces exit 1**, even on a blocker-severity check. We
did not observe a failure, so we do not assert one. It produces exit 2 so a
pipeline still notices. This is the single most-tested line in the codebase.

`ExitToolError` outranks everything. A caller must not read exit 3 as "the
environment is fine" — it means the tool could not do its job. Stub subcommands
exit 3 rather than 0, so a pipeline calling a future command by mistake fails
loudly instead of recording a spurious pass.

## Consequences

- Pipelines can gate on blockers (`-eq 1`) while tolerating warnings, or gate on
  anything non-zero.
- `results.ExitCode` is the single implementation; every command path funnels
  through `exitWith`.
- **Cost:** exit 2 conflates "a warning failed" with "we could not tell". Both
  mean "look at this", which is what the exit code is for, but a caller wanting
  to distinguish them must parse the JSON. Accepted: adding a fifth code costs
  more than it buys.
- **Cost:** `--fail-on` (treat warnings as blockers) is not implemented. It will
  be wanted. It must not change the meaning of the existing codes.
- **Unresolved:** what exit code `drift` uses. Reusing 1/2 by severity conflates
  "a check failed" with "something changed", which are different questions.
  Recorded in `cmd/vksinspect/drift.go`.
