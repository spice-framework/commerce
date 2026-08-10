package platform

import "net/http"

// @import { Bean } from "github.com/spice-framework/spice/annotation/core"

// Mux owns construction of the generated controller route table.
type Mux struct{}

// NewMux supplies the route table populated by generated controller adapters.
//
// @Bean
func NewMux() *http.ServeMux {
	return http.NewServeMux()
}
