package orders

// @import { ConfigurationProperties } from "github.com/spice-framework/spice/annotation/core"

// Settings configures the reference order catalog.
//
// @ConfigurationProperties(prefix="commerce.orders")
type Settings struct {
	SKU            string `spice:"sku,default=SKU-RED"`
	UnitPriceCents int    `spice:"unit-price-cents,default=2500"`
}
