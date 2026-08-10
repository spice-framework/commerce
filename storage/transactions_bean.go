package storage

import (
	"database/sql"

	"github.com/spice-framework/spice/data"
)

// @import { Bean } from "github.com/spice-framework/spice/annotation/core"

// Transactions provides the exact generated transaction boundary.
//
// @Bean
func Transactions(database *sql.DB) (*data.Manager, error) {
	return data.NewManager(database)
}
