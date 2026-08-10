package storage

import (
	"database/sql"
	"errors"
)

// @import { Bean } from "github.com/spice-framework/spice/annotation/core"

// Native exposes the owned pool to the generated transaction manager.
//
// @Bean
func Native(database *Database) (*sql.DB, error) {
	if database == nil || database.native == nil {
		return nil, errors.New("provide commerce database: database is nil")
	}
	return database.native, nil
}
