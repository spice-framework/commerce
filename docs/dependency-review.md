# Dependency review

Commerce is an application, not a library. Its selected dependencies therefore
model a realistic deployment while remaining explicit and independently
auditable.

## Spice Framework

- Module: `github.com/spice-framework/spice`
- Selection: the immutable pseudo-version recorded in `go.mod`
- License: Apache-2.0
- Purpose: public runtime contracts, isolated PostgreSQL and SMTP starters,
  annotation descriptors, generated-code testing support, CLI, and annotation
  tool.
- Runtime: generated ordinary Go directly calls selected providers. The
  compiler and CLI are tool dependencies and do not participate in the running
  application.
- Security: no module download occurs during normal generation or verification;
  the committed vendor tree is checked against a fresh deterministic render.

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
