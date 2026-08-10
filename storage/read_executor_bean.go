package storage

import (
	"database/sql"

	"github.com/spice-framework/spice/data"
)

// @import { Bean } from "github.com/spice-framework/spice/annotation/core"

// ReadExecutor provides the exact interface bean used for non-transactional
// cached reads.
//
// @Bean
func ReadExecutor(database *sql.DB) data.Executor {
	return database
}
