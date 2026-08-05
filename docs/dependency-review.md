# Dependency review

Commerce is an application, not a library. Its selected dependencies therefore
model a realistic deployment while remaining explicit and independently
auditable.

## Spice Framework

- Module: `github.com/spice-framework/spice`
- Selection: the immutable pseudo-version recorded in `go.mod`
- License: Apache-2.0
- Purpose: public runtime contracts, annotation descriptors, and generated-code
  testing support.
- Runtime: generated ordinary Go directly calls selected providers. The
  compiler does not participate in the running application.
- Security: no module download occurs during normal generation or verification;
  the committed vendor tree is checked against a fresh deterministic render.

## Spice toolchain

- Module: `github.com/spice-framework/toolchain`
- Selection: the independently pinned immutable pseudo-version in `go.mod`
- License: Apache-2.0
- Purpose: the Spice CLI, compiler, generator, verifier, development loop, and
  official annotation tool.
- Runtime: toolchain packages are selected only through Go `tool` directives
  and do not participate in the generated application's runtime graph.
- Security: tools run from the exact Go module graph with offline generation
  and verification; compatibility checks validate the tool module independently
  from the core module.

## pgx

- Module: `github.com/jackc/pgx/v5`
- Selection: `v5.10.0`, through Spice's opt-in PostgreSQL starter
- License: MIT
- Purpose: PostgreSQL `database/sql` driver and connection behavior.
- Cancellation and cleanup: callers own contexts; the application owns the
  pool and closes it through generated lifecycle cleanup.
- Security: TLS is verified by default. A local `sslmode=disable` URL is
  rejected unless `SPICE_COMMERCE_DATABASE_ALLOW_INSECURE=true` is explicitly
  set. Hosted acceptance proves real PostgreSQL persistence and cleanup.

The remaining selected modules are pgx transitive support dependencies or Go
`x` modules selected by Spice. `govulncheck` and `gosec` are mandatory in the
repository gate. Dependency updates must preserve the zero-network default,
caller cancellation, deterministic configuration, secret redaction, and the
real PostgreSQL acceptance result.
