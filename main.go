package main

import (
	"os"

	spiceapp "github.com/StevenBuglione/spice/internal/spicegen/commerce"
)

// @import { Application } from "github.com/StevenBuglione/spice/annotation/core"
// @import { Enable } from "github.com/StevenBuglione/spice/annotation/management"
// @import { Logging } from "github.com/StevenBuglione/spice/annotation/observability"

// @Application
// @Enable(expose=["health", "liveness", "readiness", "info", "metrics", "configprops", "modules"], access="loopback")
// @Logging
func main() {
	os.Exit(spiceapp.Main(os.Args[1:]))
}
