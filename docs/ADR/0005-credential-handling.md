# ADR-0005 — Credentials never touch the config or any artifact

**Status:** Accepted · **Date:** 2026-07-29

## Context

This tool runs inside customer environments, is handed to field engineers, and
produces artifacts — reports, baselines — that get emailed, attached to
tickets and committed to repositories. It needs vCenter, NSX and ALB
credentials to do class (b) checks.

Every one of those artifacts is a plausible place for a password to end up, and
the most common leak path is not a deliberate write but a `%v` in a log line or
an error message.

## Decision

Four rules, each with a mechanism rather than a convention:

1. **Never in the config YAML.** Credentials live in a separate file or in
   `VKSINSPECT_*` environment variables. The config loader additionally scans
   the parsed document for credential-shaped keys and refuses the file.
2. **Never serialised.** `creds.Credential` implements `MarshalJSON` returning
   an error. Trying to put a credential in an artifact fails loudly rather than
   succeeding quietly.
3. **Never printed.** `Credential` implements `String()` and `GoString()` to
   redact, so `%v`, `%s`, `%+v`, `%#v` and `%q` are all safe. Tested.
4. **File permissions enforced.** A credentials file readable by group or other
   is refused with an error telling the user to `chmod 0600`. Refusing rather
   than warning: a warning scrolls past on a shared jump host.

The config references credentials by name (`credentialRef: vcenter`). References
are not secrets and appear freely in artifacts.

## Consequences

- No credential can reach a report or a baseline without deliberately defeating
  three separate mechanisms.
- Pipelines inject secrets via environment without rewriting files.
- **Cost:** refusing a 0644 credentials file will annoy someone, on a laptop,
  once. Accepted.
- **Cost:** `assertNoSecrets` in the config loader is nearly unreachable in
  practice, because `yaml.KnownFields(true)` rejects unknown keys first. It is a
  guard against a future struct field reintroducing a credential field, not an
  active defence. Documented as such rather than presented as protection it does
  not provide.
- **Not covered:** the tool does not attempt to prevent a *check author* putting
  a credential into `Result.Evidence` as a plain string. Only convention and
  review prevent that. If credentialed checks proliferate, a scanner over
  serialised results is the obvious next control.
- **Recommended posture:** the account given to this tool should be read-only.
  It performs no writes (ADR-0007), so an administrative account grants access
  it cannot use and should not have.
