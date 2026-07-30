# ADR-0005 — Credentials never get written to any file

**Status:** Accepted · **Date:** 2026-07-29

## Context

This tool runs inside customer environments, gets handed to field engineers, and
produces files — reports, baselines — that get emailed, attached to tickets and
committed to repositories. It needs vCenter, NSX and ALB credentials to run the
API checks.

Every one of those files is a plausible place for a password to end up, and the
most common way that happens is not someone writing it deliberately. It is a
`%v` in a log line or an error message.

## Decision

Four rules, each with a mechanism rather than a convention:

1. **Never in the config YAML.** Credentials live in a separate file or in
   `VKSINSPECT_*` environment variables. The config loader additionally scans
   the parsed document for credential-shaped keys and refuses the file.
2. **Never written out.** `creds.Credential` implements `MarshalJSON` so that it
   returns an error. Trying to write a credential to a file fails loudly rather
   than succeeding quietly.
3. **Never printed.** `Credential` implements `String()` and `GoString()` to
   redact, so `%v`, `%s`, `%+v`, `%#v` and `%q` are all safe. Tested.
4. **File permissions enforced.** A credentials file readable by group or other
   is refused with an error telling the user to `chmod 0600`. Refusing rather
   than warning: a warning scrolls past on a shared jump host.

The config refers to credentials by name (`credentialRef: vcenter`). A name is
not a secret, so it appears in files freely.

## Consequences

- No credential can reach a report or a baseline without deliberately defeating
  three separate mechanisms.
- Pipelines can supply secrets through the environment without rewriting files.
- **Cost:** refusing a 0644 credentials file will annoy someone, on a laptop,
  once. Accepted.
- **Cost:** `assertNoSecrets` in the config loader almost never fires, because
  `yaml.KnownFields(true)` rejects unknown keys first. It guards against a future
  struct field reintroducing a credential field; it is not an active defence.
  Written down as such, rather than presented as protection it does not give.
- **Not covered:** nothing stops a *check author* putting a credential into
  `Result.Evidence` as plain text. Only convention and review prevent that. If
  the number of credentialed checks grows, scanning the written-out results is
  the obvious next step.
- **What to do:** give this tool a read-only account. It writes nothing
  (ADR-0007), so an administrator account only hands it access it cannot use and
  should not have.
