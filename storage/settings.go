package storage

// @import { ConfigurationProperties } from "github.com/spice-framework/spice/annotation/core"

// Settings selects the instance-owned database implementation. The default
// memory URL exists only for the zero-dependency reference developer loop.
//
// @ConfigurationProperties(prefix="commerce.database")
type Settings struct {
	URL           string `spice:"url,default=memory://commerce,env=SPICE_COMMERCE_DATABASE_URL,secret"`
	AllowInsecure bool   `spice:"allow-insecure,default=false,env=SPICE_COMMERCE_DATABASE_ALLOW_INSECURE"`
}
