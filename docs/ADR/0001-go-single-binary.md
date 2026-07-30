# ADR-0001 — Go, single static binary

**Status:** Accepted · **Date:** 2026-07-29

## Context

The tool runs from a jump host inside a customer's management network, or from a
field engineer's laptop. Neither is a place where an interpreter, a package
manager, a virtualenv or an internet connection can be assumed. Frequently the
environment is a hardened bastion where installing anything at all requires a
change request.

Later phases add a built-in web UI, which means whatever we ship has to be able
to carry static assets along with it.

Python was the obvious alternative and was considered. It loses on the only thing
that matters here: getting the tool onto the machine. Shipping a Python tool into
a customer's
locked-down bastion means shipping an interpreter, a dependency tree, and a
support burden for whichever of those the customer's hardening breaks. PyInstaller
and friends move the problem rather than solving it.

## Decision

Go, compiled `CGO_ENABLED=0`, one static binary per platform. No runtime
dependencies. UI assets will be embedded with `go:embed` when that phase lands.

Cross-compiled for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64 and
windows/amd64 — the platforms a field engineer actually has.

## Consequences

- `scp` one file and run it. No install step, no interpreter, no dependency
  resolution on a host with no internet.
- Go's standard library covers DNS, TCP, TLS and HTTP without third-party
  packages, which keeps the dependency surface small in a tool that will be
  security-reviewed by customers.
- **Cost:** raw sockets for ICMP and ARP need access the tool often will not
  have. That is not a Go problem, but Go gives us no way round it either. What
  happens instead — report a skip with a reason, never a quiet pass — is part of
  the design rather than an afterthought. See ADR-0007.
- **Cost:** the vSphere and NSX SDK ecosystem in Go is real but thinner than
  Python's `pyvmomi`. Some API surfaces will be hand-rolled REST clients.
- **Cost:** contributors who know Python and not Go are excluded from
  contributing checks. Mitigated by making the `Check` interface small enough
  that a new check is a self-contained file with an obvious template
  (`internal/checks/reference`).
