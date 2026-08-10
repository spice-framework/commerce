package storage

import (
	"database/sql/driver"
	"io"
)

type memoryRows struct {
	values [][]driver.Value
	index  int
}

func (*memoryRows) Columns() []string {
	return []string{"id", "sku", "quantity", "total_cents", "authorization_id"}
}

func (*memoryRows) Close() error { return nil }

func (rows *memoryRows) Next(destination []driver.Value) error {
	if rows.index >= len(rows.values) {
		return io.EOF
	}
	copy(destination, rows.values[rows.index])
	rows.index++
	return nil
}
