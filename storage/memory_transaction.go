package storage

import (
	"database/sql"
	"errors"
	"maps"
)

type memoryTransaction struct {
	connection   *memoryConnection
	records      map[string]Record
	createSchema bool
	done         bool
}

func (tx *memoryTransaction) Commit() error {
	if tx == nil || tx.connection == nil {
		return errors.New("commerce memory transaction is nil")
	}
	tx.connection.mu.Lock()
	defer tx.connection.mu.Unlock()
	if tx.done || tx.connection.tx != tx {
		return errors.New("commerce memory transaction is already complete")
	}
	tx.connection.database.mu.Lock()
	defer tx.connection.database.mu.Unlock()
	for id := range tx.records {
		if _, duplicate := tx.connection.database.records[id]; duplicate {
			tx.done = true
			tx.connection.tx = nil
			return errMemoryDuplicate
		}
	}
	if tx.createSchema {
		tx.connection.database.schema = true
	}
	maps.Copy(tx.connection.database.records, tx.records)
	tx.done = true
	tx.connection.tx = nil
	return nil
}

func (tx *memoryTransaction) Rollback() error {
	if tx == nil || tx.connection == nil {
		return errors.New("commerce memory transaction is nil")
	}
	tx.connection.mu.Lock()
	defer tx.connection.mu.Unlock()
	if tx.done || tx.connection.tx != tx {
		return sql.ErrTxDone
	}
	tx.done = true
	tx.connection.tx = nil
	return nil
}
