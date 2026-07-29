# ADR-0004 — Renderers are pluggable; the UI is a JSON consumer

**Status:** Accepted · **Date:** 2026-07-29

## Context

Output goes to three places with incompatible needs: a field engineer's
terminal, a CI system's JUnit collector, and — later — a local web UI. A tool
that prints from inside its checks can serve exactly one of them.

The specific risk with the UI phase is that it grows privileged access into the
check layer: a "UI mode" on checks, a special serialisation, a second code path.
That ends with the UI and the CLI disagreeing about what the environment looks
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

Three implementations: terminal, JSON, JUnit XML. **The future UI is a consumer
of the JSON renderer's output and gets no special support.** There is no UI mode
for a check and there will not be one.

Renderers must be **pure**: same `Report` in, same bytes out. No timestamps of
their own, no map iteration order, no colour unless told. This is enforced by
`TestRenderersAreDeterministic`.

The JSON renderer never filters skipped results, regardless of options. A
machine consumer that cannot distinguish "passed" from "never ran" will report a
half-inspected environment as healthy.

## Consequences

- Adding a format is one file and one line in `renderers.New`.
- Golden-file testing is possible because renderers are pure, and the golden
  files become the output contract.
- The UI phase cannot corrupt the check layer, because there is nothing for it
  to reach into.
- **Cost:** the terminal renderer cannot stream progress as checks complete —
  it receives a finished `Report`. For a run with slow network probes this means
  silence followed by output. If that becomes a real complaint, the fix is a
  separate progress channel on the engine, **not** letting checks print.
- **Cost:** the JUnit severity mapping is opinionated (a failed warning is a
  `<failure>`, because CI has no severity concept and silently passing warnings
  would make the CI view disagree with the exit code) and is currently unverified
  against any real collector.
