// Package bootstrap declares the reference application roots without owning runtime behavior.
package bootstrap

import (
	"github.com/StevenBuglione/spice/examples/commerce/orders"
	"github.com/StevenBuglione/spice/examples/commerce/platform"
)

// Commerce marks the HTTP server and order service as application roots.
//
// @Application
func Commerce(*platform.Server, *orders.Service) {
	panic("Spice application marker bodies are never executed")
}
