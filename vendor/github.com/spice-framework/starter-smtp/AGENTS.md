# Starter SMTP implementation contract

This repository owns the independently versioned secure SMTP integration for
Spice. Work directly on local `main` in bounded commits. Fetch before editing
and immediately before pushing; never overwrite unexpected remote work.

Go 1.26.5 is mandatory. Every product change must preserve verified TLS,
caller cancellation, bounded retries, payload-free diagnostics, instance
ownership, and the public `mail.Sender` contract. Add positive and failure-path
tests, update public documentation, run `make verify` on the exact commit tree,
and push only a green commit.

The normal gate is offline after dependencies are cached. Docker-backed
Mailpit acceptance is an additional release/hosted integration gate; it may
not replace the deterministic in-process protocol suite. Never commit live
credentials or production private keys. Test certificates must be visibly
fixture-only.

Release-parity work must preserve the exact `spice-dev` tool version authorized
by the root `go.mod`, invoke its full package path, and run both central and
retained rehearsals with workspace and network resolution disabled in vendor
mode. The protected central workflow is the production path once its user-owned
key, reviewed public anchor, and release environments are configured. The
retained repository builder remains only a parity oracle; unsigned parity must
never manufacture signatures or key material.
