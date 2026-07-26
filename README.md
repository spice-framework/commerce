# Commerce reference application

The commerce example is Spice's production-shaped reference application. It
uses four explicit application modules:

- `inventory` owns stock and compensated reservations;
- `payments` owns authorization policy and records approvals;
- `orders` declares and uses only the inventory and payment default APIs;
- `platform` owns the safely configured HTTP server lifecycle.

The ordinary `main.go` is the compile-time application marker. Its explicit
`@management.Enable` allowlist exposes health, liveness, readiness, info, and
metrics plus redacted configuration and generated module reports;
`@observability.Logging` installs structured lifecycle and HTTP observers. The
inventory module's `@schedule.FixedDelay` audit demonstrates direct generated,
lifecycle-owned scheduled work. Its `@async.Execute` inventory verification
demonstrates a readiness-gated typed generated submit method, bounded
admission, and graceful drain before provider cleanup. The order lookup's
`@cache.Cacheable` boundary demonstrates configured, bounded, typed response
caching. Its first successful lookup publishes a typed `OrderViewed` event to
the provider-owned `ViewAudit` listener; a cache hit bypasses the controller
and therefore does not publish again. Spice generates ordinary direct
construction, command, lifecycle, scheduling, asynchronous, cache, and event
code beside `main.go`; no runtime scan, reflection, service locator, dummy
module import, or marker execution is used.

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
own. Order lookup caching defaults to 256 entries and a five-minute TTL;
`SPICE_CACHE_COMMERCE_ORDERS_BY_ID_CAPACITY` and
`SPICE_CACHE_COMMERCE_ORDERS_BY_ID_TTL` override those generated typed
properties. Asynchronous execution defaults to 16 concurrent tasks;
`SPICE_ASYNC_MAX_CONCURRENCY` overrides that positive bound.

The generated public API is:

- `POST /orders` with a strict `{"quantity": 2}` JSON body;
- `GET /orders/{id}`;
- deterministic RFC 9457 errors for invalid, unavailable, declined, and missing
  orders;
- `/actuator/health`, `/actuator/health/liveness`,
  `/actuator/health/readiness`, `/actuator/info`, and `/actuator/metrics`.
- `/actuator/configprops`, with generated key/type/module/provenance metadata
  and mandatory secret redaction.
- `/actuator/modules`, with the generated module/API/dependency canvas and
  unassigned-package report.

The generated OpenAPI 3.1 contract is
`examples/commerce/openapi.json`. Route metrics use compiler-owned
method, pattern, symbol, and module labels rather than raw request paths.

The handwritten `main.go` only calls
`os.Exit(spiceMain(os.Args[1:]))`. Generated `spiceMain` returns the exit code
and owns signals; it never exits the process itself. Tests and embedded
processes can instead use `RunCommand`, `NewApplication`,
`NewApplicationWithOptions`, `Start`, `Stop`, or `Run` with caller-owned
writers, loggers, sources, contexts, observers, middleware, error mapping, and
shutdown policy. The ready application exposes
`SubmitServiceVerifySKU(admissionContext, sku)` and `AsyncSnapshot()` as its
typed asynchronous boundary.

`main_test.go` demonstrates `spicetest.NewHTTP`, which constructs this
generated application with explicit test options and exercises its controllers
and management routes through a bounded loopback-only slice.
