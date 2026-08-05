# Spice core and tool compatibility

Commerce is an application and compiler-tool consumer, so source compatibility
alone is insufficient. The repository must prove that the selected Spice core,
annotation tool, CLI, generated target, and application packages agree.

[`../spice-compatibility.json`](../spice-compatibility.json) is the strict,
machine-readable contract. The exact direct Spice requirement in `go.mod` is
the provisional minimum until the first preview release defines a published
floor. The current boundary is a pinned forward-compatibility signal, not a
moving runtime dependency. Branch names, aliases such as `latest`, malformed
versions, undeclared tools, indirect minimum requirements, and mismatched MVS
selection fail closed.

For each boundary the compatibility runner:

1. resolves the exact Spice version through Go's authenticated module tooling;
2. creates an isolated alternate modfile while preserving every other direct
   application and starter requirement, plus an isolated application mirror
   with a boundary-specific vendor tree for Spice compiler execution;
3. asserts the exact MVS-selected core and verifies that both authorized tool
   packages are `main` packages from that same Spice module version;
4. discovers every Commerce product and generated package, then runs `go vet`
   and `go test -race -shuffle=on -count=1`;
5. executes the selected `go tool` to run `spice verify`, `spice generate
   --check --target Commerce`, and `spice build --target Commerce`;
6. repeats all acceptance operations with `GOPROXY=off`; and
7. compares repository-wide SHA-256 state before and after, including
   handwritten code, generated Go and artifacts, `.spice` ownership metadata,
   `go.mod`, `go.sum`, and vendor.

Run both boundaries locally with:

```text
make compatibility
```

`make verify` remains the definitive local gate and always includes both.
Hosted CI exposes minimum and current jobs independently for review. Raising
the floor requires an intentional direct `go.mod` update, matching
compatibility metadata and documentation, fresh generated evidence if
required, and green minimum/current jobs. Starter versions are not advanced by
this proof.
