module github.com/StevenBuglione/spice/examples/commerce

go 1.26.0

toolchain go1.26.5

tool github.com/StevenBuglione/spice/cmd/spice-annotation-core

require github.com/StevenBuglione/spice v0.0.0

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.10.0 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)

replace github.com/StevenBuglione/spice => ../..
