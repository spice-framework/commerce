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
