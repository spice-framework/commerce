# Spice Commerce Implementation Contract

This repository is the standalone production-shaped reference application for
the Spice Framework. It proves that a modular application can consume an
immutable Spice module without a local replacement or monorepo-only behavior.

## Delivery model

- Work directly on local `main` in single-writer mode.
- Fetch and inspect `origin/main` before work and immediately before pushing.
- Use bounded, reviewable commits and never overwrite unexpected remote work.
- Preserve generated source and `.spice/*.manifest.json`; never hand-edit them.
- Pin Spice core and toolchain independently to immutable released or
  pseudo-versions.
- Do not add a local `replace` to the committed module graph.

## Required verification

Go 1.26.5 is exact and mandatory. Every commit must pass `make verify`, which
is implemented by the cross-platform Go program in `internal/qualitygate` and
also runs directly on Windows as `go run ./internal/qualitygate`.

The gate checks identity, formatting, module and vendor reproducibility, vet,
the allowlisted linter policy, NilAway, gosec, govulncheck, shuffled and race
tests, an 85% business-source coverage floor, vendor-offline tests, current
Spice generation, ordinary builds, and an executable zero-network smoke path.
It also verifies immutable minimum and pinned current paired Spice
core/toolchain boundaries without changing handwritten source, generated
output, module files, or vendor.

PostgreSQL integration tests additionally run against PostgreSQL 18 in hosted
Linux jobs. Local verification may use them when the explicit test URL is
present; it never starts or downloads services.

## Application invariants

- Handwritten source remains ordinary valid Go.
- Annotations remain declaration comments resolved through explicit imports.
- Generated code mirrors source ownership under `internal/spicegen/commerce`
  and remains inspectable, committed, deterministic Go.
- Interfaces and cleanup behavior remain explicit in generated direct calls;
  no runtime reflection, package scanning, or global service locator is added.
- In-memory SQL and test mail remain the zero-network defaults.
- PostgreSQL and SMTP retain secure defaults and require explicit opt-ins where
  local insecure transport is supported.
- Tests cover success, validation, ambiguity, cancellation, rollback, cleanup,
  and deterministic behavior appropriate to each change.
