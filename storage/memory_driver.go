package storage

import (
	"database/sql/driver"
	"errors"
)

type memoryDriver struct{}

func (memoryDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("commerce memory driver requires its connector")
}
