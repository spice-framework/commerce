package payments

// @import { ConfigurationProperties } from "github.com/spice-framework/spice/annotation/core"

// Settings configures the reference payment policy.
//
// @ConfigurationProperties(prefix="commerce.payments")
type Settings struct {
	MaximumCents int `spice:"maximum-cents,default=1000000"`
}
