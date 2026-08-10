package storage

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"sync"
)

type memoryConnection struct {
	mu       sync.Mutex
	database *memoryDatabase
	tx       *memoryTransaction
}

func (*memoryConnection) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("commerce memory driver does not support prepared statements")
}

func (*memoryConnection) Close() error { return nil }

func (connection *memoryConnection) Begin() (driver.Tx, error) {
	return connection.BeginTx(context.Background(), driver.TxOptions{})
}

func (connection *memoryConnection) BeginTx(
	ctx context.Context,
	_ driver.TxOptions,
) (driver.Tx, error) {
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	connection.mu.Lock()
	defer connection.mu.Unlock()
	if connection.tx != nil {
		return nil, errors.New("commerce memory transaction is already active")
	}
	tx := &memoryTransaction{connection: connection, records: make(map[string]Record)}
	connection.tx = tx
	return tx, nil
}

func (connection *memoryConnection) Ping(ctx context.Context) error {
	return context.Cause(ctx)
}

func (connection *memoryConnection) ExecContext(
	ctx context.Context,
	statement string,
	arguments []driver.NamedValue,
) (driver.Result, error) {
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	switch connection.normalizeSQL(statement) {
	case connection.normalizeSQL(createOrdersTableSQL):
		return connection.createSchema()
	case connection.normalizeSQL(insertOrderSQL):
		record, err := connection.recordArguments(arguments)
		if err != nil {
			return nil, err
		}
		return connection.insert(record)
	default:
		return nil, errors.New("commerce memory driver rejected unknown statement")
	}
}

func (connection *memoryConnection) QueryContext(
	ctx context.Context,
	statement string,
	arguments []driver.NamedValue,
) (driver.Rows, error) {
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	if connection.normalizeSQL(statement) != connection.normalizeSQL(findOrderSQL) ||
		len(arguments) != 1 {
		return nil, errors.New("commerce memory driver rejected unknown query")
	}
	id, ok := arguments[0].Value.(string)
	if !ok {
		return nil, errors.New("commerce memory query requires a string ID")
	}
	record, found, err := connection.find(id)
	if err != nil {
		return nil, err
	}
	rows := &memoryRows{}
	if found {
		rows.values = [][]driver.Value{{
			record.ID, record.SKU, int64(record.Quantity),
			int64(record.TotalCents), record.AuthorizationID,
		}}
	}
	return rows, nil
}

func (connection *memoryConnection) createSchema() (driver.Result, error) {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	if connection.tx != nil {
		connection.tx.createSchema = true
		return driver.RowsAffected(0), nil
	}
	connection.database.mu.Lock()
	connection.database.schema = true
	connection.database.mu.Unlock()
	return driver.RowsAffected(0), nil
}

func (connection *memoryConnection) insert(record Record) (driver.Result, error) {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	if connection.tx != nil {
		if _, duplicate := connection.tx.records[record.ID]; duplicate {
			return nil, errMemoryDuplicate
		}
		connection.database.mu.Lock()
		_, duplicate := connection.database.records[record.ID]
		connection.database.mu.Unlock()
		if duplicate {
			return nil, errMemoryDuplicate
		}
		connection.tx.records[record.ID] = record
		return driver.RowsAffected(1), nil
	}
	connection.database.mu.Lock()
	defer connection.database.mu.Unlock()
	if !connection.database.schema {
		return nil, errMemorySchema
	}
	if _, duplicate := connection.database.records[record.ID]; duplicate {
		return nil, errMemoryDuplicate
	}
	connection.database.records[record.ID] = record
	return driver.RowsAffected(1), nil
}

func (connection *memoryConnection) find(id string) (Record, bool, error) {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	if connection.tx != nil {
		if record, found := connection.tx.records[id]; found {
			return record, true, nil
		}
	}
	connection.database.mu.Lock()
	defer connection.database.mu.Unlock()
	if !connection.database.schema {
		return Record{}, false, errMemorySchema
	}
	record, found := connection.database.records[id]
	return record, found, nil
}

func (connection *memoryConnection) recordArguments(arguments []driver.NamedValue) (Record, error) {
	if len(arguments) != 5 {
		return Record{}, errors.New("commerce memory insert requires five arguments")
	}
	id, idOK := arguments[0].Value.(string)
	sku, skuOK := arguments[1].Value.(string)
	quantity, quantityOK := connection.integerArgument(arguments[2].Value)
	total, totalOK := connection.integerArgument(arguments[3].Value)
	authorizationID, authorizationOK := arguments[4].Value.(string)
	if !idOK || !skuOK || !quantityOK || !totalOK || !authorizationOK {
		return Record{}, errors.New("commerce memory insert argument types are invalid")
	}
	return Record{
		ID: id, SKU: sku, Quantity: quantity, TotalCents: total,
		AuthorizationID: authorizationID,
	}, nil
}

func (connection *memoryConnection) integerArgument(value any) (int, bool) {
	number, ok := value.(int64)
	if !ok {
		return 0, false
	}
	converted := int(number)
	return converted, int64(converted) == number
}

func (connection *memoryConnection) normalizeSQL(statement string) string {
	return strings.Join(strings.Fields(statement), " ")
}
