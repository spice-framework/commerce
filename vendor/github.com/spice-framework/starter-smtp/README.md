# Spice SMTP Starter

`starter-smtp` is the independently versioned secure SMTP transport for the
[Spice Framework](https://github.com/spice-framework/spice). It implements the
public `mail.Sender` interface without a global client, runtime reflection, or
hidden network activity during construction.

## Install

```text
go get github.com/spice-framework/starter-smtp@<version>
```

```go
import (
	"context"
	"crypto/tls"

	spicesmtp "github.com/spice-framework/starter-smtp"
	"github.com/spice-framework/spice/mail"
)

func sender() (mail.Sender, error) {
	return spicesmtp.New(spicesmtp.Config{
		Address:    "smtp.example.com:587",
		ServerName: "smtp.example.com",
		Mode:       spicesmtp.TLSModeStartTLS,
		TLSConfig:  &tls.Config{MinVersion: tls.VersionTLS12},
	})
}

func deliver(ctx context.Context, sender mail.Sender, message mail.Message) error {
	return sender.Send(ctx, message)
}
```

TLS is mandatory. STARTTLS fails closed when the server does not advertise it;
implicit TLS is an explicit alternative. Certificate verification cannot be
disabled, authentication occurs only after TLS, retries are bounded to safe
pre-DATA failures, and ambiguous post-DATA failures are never replayed.

## Compatibility

The current development line requires Go 1.26.5. Its verified Spice minimum is
`v0.0.0-20260805222830-a2ecd56df246` and its current endpoint is
`v0.0.0-20260806053623-2ec6f862073f`. The repository-owned
[`spice-compatibility.json`](spice-compatibility.json) is the machine-readable
source of these boundaries. The current endpoint is a pinned forward-
compatibility signal, not a moving runtime dependency. The complete gate uses
isolated alternate modfiles, asserts the exact MVS-selected Spice version, and
runs vet plus shuffled race tests over every product package at both
boundaries. It also proves that `go.mod`, `go.sum`, product Go files, and vendor
contents remain unchanged. Version selection remains ordinary Go module
selection; this repository does not introduce a BOM or dependency resolver.
The public manifest reports Spice annotation API compatibility and constructor
activation metadata.

## Verification

```text
make check
make compatibility
make lint
make release-parity
make security
make verify
make verify-release
```

The local gate enforces formatting, module/vendor consistency, vet, allowlisted
lint, nil-safety, gosec, govulncheck, shuffled/race tests, an 85% coverage
floor, offline compilation, and minimum/current Spice compatibility. Hosted
release evidence additionally delivers a real message through a pinned Mailpit
container requiring authenticated STARTTLS and verifies the captured MIME
through Mailpit's API. CI exposes separate minimum/current compatibility jobs
for review, while `make verify` remains the definitive local gate and always
checks both.

Release parity runs the exact `spice-dev` tool authorized by `go.mod` and the
retained repository builder twice each, entirely from `vendor` with network and
workspace resolution disabled. It compares archive entries after normalizing
only the builders' documented root-directory spelling, requires equivalent
SBOM package and dependency facts, verifies both checksum files, and forbids
rehearsal signatures on Windows and Linux.

See [`docs/dependency-review.md`](docs/dependency-review.md) for the transport,
security, cancellation, maintenance, and observability review, and
[`docs/support.md`](docs/support.md) for the explicit support matrix.

## Releases

The repository builds deterministic source-only releases with an SPDX 2.3
SBOM, SHA-256 checksums, and Ed25519 signatures. See the exact artifact and
clean-tag ceremony in [`docs/releasing.md`](docs/releasing.md).
The retained repository builder and signed production workflow remain the
release authority while the centrally rendered unsigned candidate is held to
the dual-builder parity contract.
