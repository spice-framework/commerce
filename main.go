package main

import (
	"os"
)

// @Application
// @management.Enable(expose=["health", "liveness", "readiness", "info", "metrics", "configprops", "modules"])
// @observability.Logging
func main() {
	os.Exit(spiceMain(os.Args[1:]))
}
