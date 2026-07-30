# ADR-0004 — Formatting lives in one place; the UI just reads the JSON

**Status:** Accepted · **Date:** 2026-07-29

## Context

Output goes to three places with incompatible needs: a field engineer's
terminal, a CI system's JUnit collector, and — later — a local web UI. A tool
that prints from inside its checks can serve exactly one of them.

The specific risk with the UI phase is that it gets special access into the
checks: a "UI mode" on a check, its own output format, a second code path. That
ends with the UI and the CLI disagreeing about what the environment looks
like.

## Decision

Checks never print. The engine never prints. `renderers.Renderer` is the only
formatting layer:

```go
type Renderer interface {
    Name() string
    Render(w io.Writer, rep *results.Report) error
}
```

Three of them: terminal, JSON, JUnit XML. **The future UI reads the JSON
renderer's output like anything else would, and gets no special treatment.**
There is no UI mode on a check and there will not be one.

A renderer must give the **same bytes out for the same `Report` in**. No
timestamps of its own, no map iteration order, no colour unless told.
`TestRenderersAreDeterministic` enforces it.

The JSON renderer never drops skipped results, whatever the options say.
Anything reading it that cannot tell "passed" from "never ran" will report a
half-inspected environment as healthy.

## Consequences

- Adding a format is one file and one line in `renderers.New`.
- Golden-file testing works precisely because the output is stable, and those
  files become the output people depend on.
- The UI phase cannot damage the check layer, because there is nothing for it to
  reach into.
- **Cost:** the terminal renderer cannot show progress as checks finish — it gets
  a completed `Report`. On a run with slow network probes that means silence,
  then output. If that becomes a real complaint, the fix is a separate progress
  channel on the engine, **not** letting checks print.
- **Cost:** how severity maps to JUnit is a judgement call. A failed warning
  becomes a `<failure>`, because CI has no idea of severity and quietly passing
  warnings would make the CI view disagree with the exit code. No real collector
  has confirmed it.
