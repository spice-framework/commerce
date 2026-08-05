package main

import (
	"os"

	spiceapp "github.com/spice-framework/spice/examples/commerce/internal/spicegen/commerce"
	_ "github.com/spice-framework/spice/examples/commerce/inventory"
	_ "github.com/spice-framework/spice/examples/commerce/notifications"
	_ "github.com/spice-framework/spice/examples/commerce/orders"
	_ "github.com/spice-framework/spice/examples/commerce/payments"
	_ "github.com/spice-framework/spice/examples/commerce/platform"
	_ "github.com/spice-framework/spice/examples/commerce/storage"
)

// @import { Application } from "github.com/spice-framework/spice/annotation/core"
// @import { Enable } from "github.com/spice-framework/spice/annotation/management"
// @import { Logging } from "github.com/spice-framework/spice/annotation/observability"

// @Application
// @Enable(expose=["health", "liveness", "readiness", "info", "metrics", "configprops", "modules"], access="loopback")
// @Logging
func main() {
	os.Exit(spiceapp.Main(os.Args[1:]))
}
