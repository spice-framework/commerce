package main

import (
	"os"

	spiceapp "github.com/spice-framework/commerce/internal/spicegen/commerce"
	_ "github.com/spice-framework/commerce/inventory"
	_ "github.com/spice-framework/commerce/notifications"
	_ "github.com/spice-framework/commerce/orders"
	_ "github.com/spice-framework/commerce/payments"
	_ "github.com/spice-framework/commerce/platform"
	_ "github.com/spice-framework/commerce/storage"
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
