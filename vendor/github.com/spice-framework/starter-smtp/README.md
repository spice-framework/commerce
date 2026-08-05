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

The current development line requires Go 1.26.5 and Spice
`v0.0.0-20260805162230-a0bbb964bf6b`. Version selection remains ordinary Go
module selection; this repository does not introduce a BOM or dependency
resolver. The public manifest reports Spice annotation API compatibility and
constructor activation metadata.

## Verification

```text
make check
make verify
```

The local gate enforces formatting, module/vendor consistency, vet, allowlisted
lint, nil-safety, gosec, govulncheck, shuffled/race tests, an 85% coverage
floor, and offline compilation. Hosted release evidence additionally delivers
a real message through a pinned Mailpit container requiring authenticated
STARTTLS and verifies the captured MIME through Mailpit's API.

See [`docs/dependency-review.md`](docs/dependency-review.md) for the transport,
security, cancellation, maintenance, and observability review, and
[`docs/support.md`](docs/support.md) for the explicit support matrix.
