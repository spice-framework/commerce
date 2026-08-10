package platform

import "time"

// @import { ConfigurationProperties } from "github.com/spice-framework/spice/annotation/core"

// Settings contains safe HTTP server defaults.
//
// @ConfigurationProperties(prefix="commerce.server")
type Settings struct {
	Address           string        `spice:"address,default=127.0.0.1:8081,env=SPICE_COMMERCE_ADDRESS"`
	ReadHeaderTimeout time.Duration `spice:"read-header-timeout,default=5s"`
	ReadTimeout       time.Duration `spice:"read-timeout,default=15s"`
	WriteTimeout      time.Duration `spice:"write-timeout,default=15s"`
	IdleTimeout       time.Duration `spice:"idle-timeout,default=60s"`
	DeveloperToken    string        `spice:"developer-token,env=SPICE_COMMERCE_DEVELOPER_TOKEN,secret"`
}
