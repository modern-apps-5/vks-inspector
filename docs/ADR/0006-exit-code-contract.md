# ADR-0006 — Exit codes are fixed, and "could not tell" is not "failed"

**Status:** Accepted · **Date:** 2026-07-29

## Context

The tool is going into pipelines. For automation, the exit code *is* the
interface, and quietly changing one breaks a customer's gate in a way they will
struggle to diagnose.

There is a subtler problem too. A network probe has three outcomes, not two: a
port can accept, refuse, or say nothing at all. Silence means a firewall, and it
is *not* proof the service is down. A tool that calls silence a failure blocks
deployments over a filtered ICMP path. A tool that calls it success passes an
environment it never actually checked.

## Decision

Four codes, fixed:

| Code | Meaning |
|---|---|
| `0` | All checks passed (or were legitimately skipped) |
| `1` | At least one blocker-severity check failed |
| `2` | Warnings failed, or some checks could not tell |
| `3` | Tool error — says nothing about the environment |

Two design consequences:

**Status and severity are separate things.** `Status` is what was seen (`pass`,
`fail`, `skip`, `unknown`, `error`). `Severity` is how much it matters
(`blocker`, `warning`, `info`). A check never decides its own importance — that
is policy, and `config.Policy.SeverityOverrides` can change it at runtime.

**`StatusUnknown` never produces exit 1**, even for a blocker. We did not see a
failure, so we do not report one. It produces exit 2, so a pipeline still
notices. This is the most heavily tested line in the codebase.

`ExitToolError` outranks everything else. Exit 3 must never be read as "the
environment is fine" — it means the tool could not do its job. Commands that are
not built yet exit 3 rather than 0, so a pipeline that calls a future command by
mistake fails loudly instead of recording a pass that never happened.

## Consequences

- Pipelines can gate on blockers (`-eq 1`) while tolerating warnings, or gate on
  anything non-zero.
- `results.ExitCode` is the single implementation; every command path funnels
  through `exitWith`.
- **Cost:** exit 2 covers both "a warning failed" and "we could not tell". Both
  mean "look at this", which is what the code is for, but anyone who needs to
  tell them apart has to parse the JSON. Accepted — a fifth code costs more than
  it buys.
- **Cost:** `--fail-on` (treat warnings as blockers) does not exist yet. Someone
  will want it. It must not change what the existing codes mean.
- **Unresolved:** what exit code `drift` uses. Reusing 1 and 2 by severity would
  mix up "a check failed" with "something changed", which are different
  questions. Recorded in `cmd/vksinspect/drift.go`.
