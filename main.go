package main

import (
	"os"
)

// @spice.import { Application } from "github.com/StevenBuglione/spice/annotation/core"
// @spice.import { Enable } from "github.com/StevenBuglione/spice/annotation/management"
// @spice.import { Logging } from "github.com/StevenBuglione/spice/annotation/observability"

// @Application
// @Enable(expose=["health", "liveness", "readiness", "info", "metrics", "configprops", "modules"])
// @Logging
func main() {
	os.Exit(spiceMain(os.Args[1:]))
}
