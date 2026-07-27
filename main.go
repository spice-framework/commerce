package main

import (
	"os"
)

// @import { Application } from "github.com/StevenBuglione/spice/annotation/core"
// @import { Enable } from "github.com/StevenBuglione/spice/annotation/management"
// @import { Logging } from "github.com/StevenBuglione/spice/annotation/observability"

// @Application
// @Enable(expose=["health", "liveness", "readiness", "info", "metrics", "configprops", "modules"])
// @Logging
func main() {
	os.Exit(spiceMain(os.Args[1:]))
}
