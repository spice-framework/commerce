package storage

import (
	"context"
	"database/sql/driver"
)

type memoryConnector struct {
	database *memoryDatabase
}

func (connector memoryConnector) Connect(ctx context.Context) (driver.Conn, error) {
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	return &memoryConnection{database: connector.database}, nil
}

func (connector memoryConnector) Driver() driver.Driver {
	return memoryDriver{}
}
