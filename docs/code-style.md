# Commerce code-style contract

Commerce is a strict consumer of the canonical Spice application standard in
Toolchain `CODE_STYLE.md`. The exact machine-readable policy is committed at
`.spice/style.json`; `make check` and `make verify` run its pinned `spicestyle`
tool offline from the committed vendor tree.

The migration separates every domain DTO, configuration value, interface,
service, controller, listener, transport, repository, and in-memory SQL driver
object into an owning file. Deterministic helpers are private methods on that
owner. Generated code remains under `internal/spicegen` and never substitutes
for the handwritten model.

## Exact interoperability boundaries

- `inventory/errors.go`, `notifications/errors.go`, `orders/errors.go`,
  `payments/errors.go`, and `storage/errors.go` retain the ten public typed
  `error` sentinels used with `errors.Is` across module boundaries.
- `storage/order_sql.go` is the immutable constant-only SQL and migration
  identity carrier.
- `*_bean.go`, `*_topic.go`, Go process entrypoints, tests, and generated
  ownership roots use the standard exact boundary classifications.

Every variable exception fixes the path, symbol, resolved Go type, reason, and
review issue. No wildcard authorizes mutable package state, and declaration
comments cannot suppress a finding. New behavior belongs to its constructed
owner; a new exception is acceptable only when Go or an external identity
contract requires package scope.
