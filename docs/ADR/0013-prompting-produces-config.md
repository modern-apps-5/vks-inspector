# ADR-0013 — Prompting produces a config; it is not an alternative to one

**Status:** Accepted · **Date:** 2026-07-29

## Context

The phase-1 scaffold required a fully-written YAML config before it would do
anything. That is the wrong front door. The intended use is: hand someone the
binary, they point it at a vCenter, it asks what it needs to know, and it tells
them whether the environment is ready.

The naive fix — make everything interactive — breaks the other three modes.
`verify`, `snapshot` and `drift` run in pipelines and cannot prompt. If
interactive answers lived only in the process that collected them, a preflight
run and a later verify run would be grading against different, unrecorded
intent, and drift would have nothing stable to diff.

## Decision

**The prompt flow assembles a `config.Config` — the same type, validated through
the same `config.Validate`, as one read from a file.** `--save-config` writes it
out. A saved config makes every subsequent run non-interactive.

```
vksinspect check --vcenter vc.corp.local --save-config lab01.yaml   # asks
vksinspect check --config lab01.yaml                                # does not
```

Rules that follow, and must not be quietly broken:

1. **Never re-ask what is already known.** Config, then flags, then prompts.
   A partially-filled config plus three questions is a supported workflow, not
   an all-or-nothing choice.
2. **Never ask what can be discovered.** Anything readable from vCenter is
   reported to the operator, not asked. See ADR-0014.
3. **`--non-interactive` makes a missing value an error naming the config
   field** — never a silent default. A silently-wrong default produces a
   confident wrong verdict, which is the failure mode this tool exists to
   prevent.
4. **Never prompt for a password.** Credentials come from the environment or
   the credentials file (ADR-0005). The prompt package has no code path that
   reads one.
5. **Unverified defaults are stated as such at the point of asking.** The NTP
   skew prompt says the 30-second default is a field heuristic and not a
   documented product limit; the management-range prompt says the
   five-consecutive-addresses convention is unconfirmed for VCF 9. An operator
   pressing Enter should know they are accepting a guess.
6. **`--save-config` refuses to overwrite.** Someone who has just answered
   twenty questions must not be able to destroy a colleague's config with a
   mistyped filename.

Section headings print lazily, only when a question under them is actually
asked. Eager printing produced empty headings for whole sections already
answered by config, and noise in an interactive tool trains the operator to stop
reading.

## Consequences

- One command serves both the field engineer's first run and the pipeline.
- The saved config is the audit trail: what was declared, in a form that
  `verify` and `drift` can grade against later.
- **Cost:** two sources of truth for the question set — the prompt flow in
  `internal/prompt/elicit.go` and the struct in `internal/config`. A new config
  field that nobody adds a prompt for is silently unreachable interactively.
  Nothing currently detects that.
- **Cost:** the prompt flow is ordered imperative code, not data. Reordering
  questions means editing control flow. Acceptable at this size; if it grows
  much past its current length it wants to become a declarative question list.
- **Not done:** no test drives the prompt flow end to end. It is exercised
  manually. This is a real gap and is listed in unit-test-coverage.md.
