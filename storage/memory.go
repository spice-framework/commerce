package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/StevenBuglione/spice/migration"
)

var (
	errMemoryDuplicate = errors.New("memory order already exists")
	errMemorySchema    = errors.New("memory order schema is not ready")
)

type memoryDatabase struct {
	mu          sync.Mutex
	migrationMu sync.Mutex
	records     map[string]Record
	applied     []migration.Applied
	schema      bool
}

func newMemoryDatabase() *memoryDatabase {
	return &memoryDatabase{
		records: make(map[string]Record),
		schema:  true,
	}
}

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

type memoryDriver struct{}

func (memoryDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("commerce memory driver requires its connector")
}

type memoryConnection struct {
	mu       sync.Mutex
	database *memoryDatabase
	tx       *memoryTransaction
}

func (*memoryConnection) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("commerce memory driver does not support prepared statements")
}

func (*memoryConnection) Close() error {
	return nil
}

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
	tx := &memoryTransaction{
		connection: connection,
		records:    make(map[string]Record),
	}
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
	switch normalizeSQL(statement) {
	case normalizeSQL(createOrdersTableSQL):
		return connection.createSchema()
	case normalizeSQL(insertOrderSQL):
		record, err := recordArguments(arguments)
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
	if normalizeSQL(statement) != normalizeSQL(findOrderSQL) ||
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
			record.ID,
			record.SKU,
			int64(record.Quantity),
			int64(record.TotalCents),
			record.AuthorizationID,
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

type memoryRows struct {
	values [][]driver.Value
	index  int
}

func (*memoryRows) Columns() []string {
	return []string{
		"id",
		"sku",
		"quantity",
		"total_cents",
		"authorization_id",
	}
}

func (*memoryRows) Close() error {
	return nil
}

func (rows *memoryRows) Next(destination []driver.Value) error {
	if rows.index >= len(rows.values) {
		return io.EOF
	}
	copy(destination, rows.values[rows.index])
	rows.index++
	return nil
}

type memoryMigrationBackend struct {
	database *Database
}

func (backend *memoryMigrationBackend) RunLocked(
	ctx context.Context,
	work func(context.Context, migration.Session) error,
) error {
	if ctx == nil {
		return errors.New("run commerce memory migrations: context is nil")
	}
	if backend == nil ||
		backend.database == nil ||
		backend.database.memory == nil ||
		backend.database.native == nil {
		return errors.New("run commerce memory migrations: backend is nil")
	}
	if work == nil {
		return errors.New("run commerce memory migrations: work is nil")
	}
	backend.database.memory.migrationMu.Lock()
	defer backend.database.memory.migrationMu.Unlock()
	return work(ctx, &memoryMigrationSession{database: backend.database})
}

type memoryMigrationSession struct {
	database *Database
}

func (session *memoryMigrationSession) Applied(
	context.Context,
) ([]migration.Applied, error) {
	session.database.memory.mu.Lock()
	defer session.database.memory.mu.Unlock()
	return append([]migration.Applied(nil), session.database.memory.applied...), nil
}

func (session *memoryMigrationSession) Apply(
	ctx context.Context,
	entry migration.Migration,
) error {
	tx, err := session.database.native.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin commerce memory migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, entry.SQL()); err != nil {
		return errors.Join(
			fmt.Errorf("execute commerce memory migration: %w", err),
			tx.Rollback(),
		)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit commerce memory migration: %w", err)
	}
	session.database.memory.mu.Lock()
	defer session.database.memory.mu.Unlock()
	session.database.memory.applied = append(
		session.database.memory.applied,
		migration.Applied{
			Version:   entry.Version(),
			Module:    entry.Module(),
			Name:      entry.Name(),
			Checksum:  entry.Checksum(),
			AppliedAt: time.Now().UTC(),
		},
	)
	return nil
}

func recordArguments(arguments []driver.NamedValue) (Record, error) {
	if len(arguments) != 5 {
		return Record{}, errors.New("commerce memory insert requires five arguments")
	}
	id, idOK := arguments[0].Value.(string)
	sku, skuOK := arguments[1].Value.(string)
	quantity, quantityOK := integerArgument(arguments[2].Value)
	total, totalOK := integerArgument(arguments[3].Value)
	authorizationID, authorizationOK := arguments[4].Value.(string)
	if !idOK || !skuOK || !quantityOK || !totalOK || !authorizationOK {
		return Record{}, errors.New("commerce memory insert argument types are invalid")
	}
	return Record{
		ID:              id,
		SKU:             sku,
		Quantity:        quantity,
		TotalCents:      total,
		AuthorizationID: authorizationID,
	}, nil
}

func integerArgument(value any) (int, bool) {
	number, ok := value.(int64)
	if !ok {
		return 0, false
	}
	converted := int(number)
	return converted, int64(converted) == number
}

func normalizeSQL(statement string) string {
	return strings.Join(strings.Fields(statement), " ")
}

var (
	_ driver.Connector      = memoryConnector{}
	_ driver.Conn           = (*memoryConnection)(nil)
	_ driver.ConnBeginTx    = (*memoryConnection)(nil)
	_ driver.ExecerContext  = (*memoryConnection)(nil)
	_ driver.QueryerContext = (*memoryConnection)(nil)
	_ driver.Pinger         = (*memoryConnection)(nil)
	_ driver.Tx             = (*memoryTransaction)(nil)
	_ driver.Rows           = (*memoryRows)(nil)
	_ migration.Backend     = (*memoryMigrationBackend)(nil)
	_ migration.Session     = (*memoryMigrationSession)(nil)
)
