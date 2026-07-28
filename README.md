# Commerce reference application

The commerce example is Spice's production-shaped reference application. It
uses six explicit application modules:

- `inventory` owns stock and compensated reservations;
- `payments` owns authorization policy and records approvals;
- `notifications` owns typed receipt composition and its selected mail
  transport;
- `orders` declares and uses explicit inventory, notification, payment, and
  storage APIs;
- `storage` owns typed order persistence and module-owned migrations;
- `platform` owns the safely configured HTTP server lifecycle and depends on
  storage readiness.

The ordinary `main.go` is the compile-time application marker. Its explicit
`@management.Enable` allowlist exposes health, liveness, readiness, info, and
metrics plus redacted configuration and generated module reports;
`@observability.Logging` installs structured lifecycle and HTTP observers. The
inventory module's `@schedule.FixedDelay` audit demonstrates direct generated,
lifecycle-owned scheduled work. Its `@async.Execute` inventory verification
demonstrates a readiness-gated typed generated submit method, bounded
admission, and graceful drain before provider cleanup. The placement route's
`@data.Transactional` boundary passes the generated transaction-owned
`data.Executor` directly into the repository. Generated `@security.Authorize`
guards require exact `orders:write`, `orders:read`, and `orders:notify` scopes
on order routes and fail closed with safe 401/403 problems. The public catalog
route's `@cache.Cacheable` boundary demonstrates configured, bounded, typed
response caching without putting principal-specific data in a shared cache.
Every successful persisted-order lookup publishes a typed `OrderViewed` event
to the provider-owned `ViewAudit` listener. Spice generates ordinary direct
construction, command, lifecycle, scheduling, asynchronous, cache, event,
migration, repository, transaction, authorization, and HTTP code beside
`main.go`; no runtime scan, reflection, service locator, dummy module import,
or marker execution is used.

The payment module exposes two explicit `payments.Processor` candidates.
`Service` is named, qualified as `stripe`, and primary; `OfflineProcessor` is
qualified separately and marked fallback. Both use `@Implements(Processor)`
and Spice emits source-owned Go assertions. The orders constructor requests
`@Qualifier("stripe")`, and the generated file passes the already constructed
Stripe service directly as the interface—there is no runtime lookup.
Notifications applies the same rule to an external framework interface:
`Delivery` uses `@Implements(mail.Sender)` and a generated Go assertion, while
`Notifier` requests the exact interface. Generated code constructs and passes
the concrete delivery directly. `SystemClock` is a second explicit interface
binding, keeping message dates caller-owned and deterministic in tests.

From the repository root:

```text
go run ./cmd/spice generate --check --target Commerce ./examples/commerce/...
go run ./cmd/spice run --target Commerce ./examples/commerce/... -- -check
go run ./cmd/spice run --target Commerce ./examples/commerce/...
```

The server binds `127.0.0.1:8081` by default. Set
`SPICE_COMMERCE_ADDRESS=127.0.0.1:0` for an ephemeral test listener. The command
uses the conventional `SPICE_` environment source. Set
`SPICE_SHUTDOWN_TIMEOUT` to override the typed `10s` shutdown default. Reusable
generated constructors read no environment, files, or process signals on their
own. Public catalog caching defaults to 256 entries and a five-minute TTL;
`SPICE_CACHE_COMMERCE_CATALOG_CAPACITY` and
`SPICE_CACHE_COMMERCE_CATALOG_TTL` override those generated typed
properties. Asynchronous execution defaults to 16 concurrent tasks;
`SPICE_ASYNC_MAX_CONCURRENCY` overrides that positive bound.

Database configuration is typed and secret-redacted. The default
`memory://commerce` URL selects an instance-owned transaction-aware
`database/sql` connector so `spice dev` needs no external service. Set
`SPICE_COMMERCE_DATABASE_URL` to a complete PostgreSQL URL to use the reviewed
pgx starter; local `sslmode=disable` additionally requires the explicit
`SPICE_COMMERCE_DATABASE_ALLOW_INSECURE=true` opt-in. The database opens without
network I/O during construction. Its module-owned migration runs as the first
lifecycle hook, and the HTTP server has an explicit dependency on the database
bean, so traffic cannot start against an unreconciled schema. The
`integration`-tagged storage test proves a committed order survives closing and
reopening the PostgreSQL pool:

```text
SPICE_TEST_POSTGRES_URL=postgres://... go test -tags=integration -run PostgreSQLPersistence ./examples/commerce/storage
```

Mail configuration is typed and instance-owned. `test` is the default
transport: it performs no network I/O and retains a bounded decoded snapshot
for tests. `POST /orders/{id}/receipt` runs only after the order transaction has
committed, creates deterministic plain-text MIME plus a receipt attachment,
and returns only the message ID. Set
`SPICE_COMMERCE_MAIL_TRANSPORT=smtp` with
`SPICE_COMMERCE_MAIL_SMTP_ADDRESS`, optional server name, and paired username
and password to use the secure SMTP starter. SMTP requires verified STARTTLS
by default (or explicit `implicit-tls`), authenticates only after TLS, observes
caller cancellation and a typed timeout, retries only safe pre-DATA transient
failures, and never replays ambiguous delivery. Recipient and credential
configuration is secret-redacted.

For the local `spice dev` walkthrough only, set
`SPICE_COMMERCE_DEVELOPER_TOKEN` to a 16-byte-or-longer bearer token. The
reference platform accepts that token only while the server binds a loopback
address and attaches a fixed developer principal with the three documented
order scopes. The token is disabled by default, compared in constant time, and
secret-redacted. It is not a production authentication mechanism; production
applications compose the OAuth2/OIDC authentication starter ahead of the same
generated authorization guards.

The generated public API is:

- `GET /catalog`, the public cache-safe product;
- `POST /orders` with a strict `{"quantity": 2}` JSON body and
  `orders:write`;
- `GET /orders/{id}` with `orders:read`;
- `POST /orders/{id}/receipt` with `orders:notify`;
- deterministic RFC 9457 errors for invalid, unavailable, declined, and missing
  orders;
- `/actuator/health`, `/actuator/health/liveness`,
  `/actuator/health/readiness`, `/actuator/info`, and `/actuator/metrics`.
- `/actuator/configprops`, with generated key/type/module/provenance metadata
  and mandatory secret redaction.
- `/actuator/modules`, with the generated module/API/dependency canvas and
  unassigned-package report.

The generated OpenAPI 3.1 contract is
`internal/spicegen/commerce/openapi.json`. Route metrics use compiler-owned
method, pattern, symbol, and module labels rather than raw request paths.

The handwritten `main.go` only calls
`os.Exit(spiceMain(os.Args[1:]))`. Generated `spiceMain` returns the exit code
and owns signals; it never exits the process itself. Tests and embedded
processes can instead use `RunCommand`, `NewApplication`,
`NewApplicationWithOptions`, `Start`, `Stop`, `Run`, or the typed `Components`
snapshot with caller-owned writers, loggers, sources, contexts, observers,
middleware, error mapping, and shutdown policy. The ready application exposes
`SubmitServiceVerifySKU(admissionContext, sku)` and `AsyncSnapshot()` as its
typed asynchronous boundary.

`TestCommerceDeveloperProof` in `main_test.go` is the executable vertical
proof. It uses
`spicetest.NewHTTP` to construct the real generated application, authenticates
verified principals in caller middleware, proves allowed/unauthenticated/
insufficient-scope decisions, places an order transactionally, retrieves the
persisted record, delivers its test receipt, exercises public caching, and
inspects management metadata through a bounded loopback-only slice. The
notifications tests inspect the exact decoded message, attachment, envelope,
cancellation, and sanitized delivery failures.

The complete edit/save/restart/HTTP walkthrough and its automated evidence map
are documented in `docs/developer-proof.md`.
