package inventory

// @import { ConfigurationProperties } from "github.com/spice-framework/spice/annotation/core"

// Settings configures the reference inventory module.
//
// @ConfigurationProperties(prefix="commerce.inventory")
type Settings struct {
	SKU          string `spice:"sku,default=SKU-RED"`
	InitialStock int    `spice:"initial-stock,default=10"`
}
