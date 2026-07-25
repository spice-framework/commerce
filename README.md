# Commerce reference application

The commerce example is Spice's production-shaped reference application. It
uses four explicit application modules:

- `inventory` owns stock and compensated reservations;
- `payments` owns authorization policy and records approvals;
- `orders` declares and uses only the inventory and payment default APIs;
- `platform` owns the safely configured HTTP server lifecycle.

`bootstrap.Commerce` is a compile-time-only application marker. Spice generates
ordinary direct construction and lifecycle code in
`internal/spicegen/commerce`; no runtime scan, reflection, or service locator is
used.

From the repository root:

```text
go run ./cmd/spice generate --check --target Commerce ./examples/commerce/bootstrap ./examples/commerce/inventory ./examples/commerce/orders ./examples/commerce/payments ./examples/commerce/platform
go run ./examples/commerce -check
go run ./examples/commerce
```

The server binds `127.0.0.1:8081` by default. Set
`SPICE_COMMERCE_ADDRESS=127.0.0.1:0` for an ephemeral test listener. The command
explicitly opts into the environment source; reusable generated constructors
read no environment or files on their own.
