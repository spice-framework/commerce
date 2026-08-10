# Spice core and toolchain compatibility

Commerce is an application and compiler-tool consumer, so source compatibility
alone is insufficient. The repository must prove that the selected Spice core,
annotation tool, CLI, generated target, and application packages agree.

[`../spice-compatibility.json`](../spice-compatibility.json) is the strict,
machine-readable contract. Each boundary is an explicit `(core, toolchain)`
pair because runtime/descriptor compatibility and compiler/generator
compatibility are independently versioned. The direct core requirement and
Go-managed toolchain requirement in `go.mod` form the provisional minimum
pair until the first preview release defines a published floor. The current
pair is a pinned forward-compatibility signal, not a moving dependency.
Branch names, aliases such as `latest`, malformed versions, undeclared tools,
missing requirements, and mismatched MVS selection fail closed.

The enforced code-style contract raises the pair to core
`v0.1.0-preview.1.0.20260807031220-45e4f9d3e12d` and toolchain
`v0.1.0-preview.2.0.20260810170203-57cf59f2d895`. That Toolchain revision owns
the strict `spicestyle` command and typed package-state exception schema.
Minimum and current therefore coincide and `-line=all` performs one combined
verification run. The schema never infers a toolchain version from a core-only
version change.

For each boundary the compatibility runner:

1. resolves the exact core and toolchain versions through Go's authenticated
   module tooling;
2. creates an isolated alternate modfile while preserving every other direct
   application and starter requirement, plus an isolated application mirror
   with a boundary-specific vendor tree for Spice compiler execution;
3. asserts both exact MVS-selected modules and verifies that all three
   authorized tool packages are `main` packages from the selected toolchain
   version;
4. discovers every Commerce product and generated package, then runs `go vet`
   and `go test -race -shuffle=on -count=1`;
5. executes `go tool github.com/spice-framework/toolchain/cmd/spice` to run
   `verify`, `generate --check --target Commerce`, and `build --target
   Commerce`;
6. repeats all acceptance operations with `GOPROXY=off`; and
7. compares repository-wide SHA-256 state before and after, including
   handwritten code, generated Go and artifacts, `.spice` ownership metadata,
   `go.mod`, `go.sum`, and vendor.

Run both boundaries locally with:

```text
make compatibility
```

`make verify` remains the definitive local gate and always includes both.
Hosted CI exposes minimum and current pair jobs independently for review.
Raising either coordinate requires an intentional `go.mod` update, matching
compatibility metadata and documentation, fresh generated evidence if
required, and green minimum/current jobs. Starter versions are not advanced by
this proof.
