package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/spice-framework/spice/data"
	"github.com/spice-framework/spice/migration"
)

var errRollbackProbe = errors.New("rollback probe")

func TestMemoryDatabaseMigratesAndPersistsTransactions(t *testing.T) {
	t.Parallel()
	database, cleanup, err := OpenDatabase(Settings{URL: memoryURL})
	if err != nil {
		t.Fatalf("OpenDatabase() error = %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := cleanup(context.Background()); cleanupErr != nil {
			t.Errorf("database cleanup error = %v", cleanupErr)
		}
	})
	if database.Mode() != "memory" || database.AppliedMigrations() != 0 {
		t.Fatalf(
			"database mode=%q migrations=%d",
			database.Mode(),
			database.AppliedMigrations(),
		)
	}
	if migrationErr := database.Migrate(context.Background()); migrationErr != nil {
		t.Fatalf("Migrate() error = %v", migrationErr)
	}
	if migrationErr := database.Migrate(context.Background()); migrationErr != nil {
		t.Fatalf("Migrate(second) error = %v", migrationErr)
	}
	if database.AppliedMigrations() != 1 {
		t.Fatalf("AppliedMigrations() = %d, want 1", database.AppliedMigrations())
	}

	native, err := Native(database)
	if err != nil {
		t.Fatalf("Native() error = %v", err)
	}
	manager, err := Transactions(native)
	if err != nil {
		t.Fatalf("Transactions() error = %v", err)
	}
	repository, err := NewOrderRepository()
	if err != nil {
		t.Fatalf("NewOrderRepository() error = %v", err)
	}
	record := testRecord("order-000001")
	err = manager.Within(
		context.Background(),
		validTransactionDefinition(),
		func(ctx context.Context, executor data.Executor) error {
			return repository.Save(ctx, executor, record)
		},
	)
	if err != nil {
		t.Fatalf("Within(save) error = %v", err)
	}
	persisted, found, err := repository.Find(
		context.Background(),
		ReadExecutor(native),
		record.ID,
	)
	if err != nil || !found || persisted != record {
		t.Fatalf("Find() = %#v, %t, %v", persisted, found, err)
	}
}

func TestMemoryDatabaseRollsBackAndRejectsDuplicates(t *testing.T) {
	t.Parallel()
	native, manager, repository := newMemoryRepository(t)
	record := testRecord("order-rollback")
	err := manager.Within(
		context.Background(),
		validTransactionDefinition(),
		func(ctx context.Context, executor data.Executor) error {
			if saveErr := repository.Save(ctx, executor, record); saveErr != nil {
				return saveErr
			}
			return errRollbackProbe
		},
	)
	if !errors.Is(err, errRollbackProbe) {
		t.Fatalf("Within(rollback) error = %v", err)
	}
	if _, found, findErr := repository.Find(
		context.Background(),
		native,
		record.ID,
	); findErr != nil || found {
		t.Fatalf("Find(rolled back) = found=%t err=%v", found, findErr)
	}

	if saveErr := manager.Within(
		context.Background(),
		validTransactionDefinition(),
		func(ctx context.Context, executor data.Executor) error {
			return repository.Save(ctx, executor, record)
		},
	); saveErr != nil {
		t.Fatalf("Within(first save) error = %v", saveErr)
	}
	err = manager.Within(
		context.Background(),
		validTransactionDefinition(),
		func(ctx context.Context, executor data.Executor) error {
			return repository.Save(ctx, executor, record)
		},
	)
	if err == nil ||
		!errors.Is(err, ErrDuplicate) ||
		strings.Contains(err.Error(), insertOrderSQL) {
		t.Fatalf("Within(duplicate) error = %v", err)
	}
}

func TestMemoryDatabaseConcurrentMigrationIsDeterministic(t *testing.T) {
	t.Parallel()
	database, cleanup, err := OpenDatabase(Settings{URL: memoryURL})
	if err != nil {
		t.Fatalf("OpenDatabase() error = %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := cleanup(context.Background()); cleanupErr != nil {
			t.Errorf("database cleanup error = %v", cleanupErr)
		}
	})
	var wait sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wait.Go(func() {
			errs <- database.Migrate(context.Background())
		})
	}
	wait.Wait()
	close(errs)
	for migrationErr := range errs {
		if migrationErr != nil {
			t.Fatalf("Migrate() error = %v", migrationErr)
		}
	}
	if database.AppliedMigrations() != 1 {
		t.Fatalf("AppliedMigrations() = %d, want 1", database.AppliedMigrations())
	}
}

func TestStorageValidationAndCancellation(t *testing.T) {
	t.Parallel()
	for _, settings := range []Settings{
		{},
		{URL: " memory://commerce"},
		{URL: "file://commerce"},
		{URL: "postgres://incomplete"},
	} {
		if _, _, err := OpenDatabase(settings); err == nil {
			t.Fatalf("OpenDatabase(%#v) error = nil", settings)
		}
	}
	if _, err := Native(nil); err == nil {
		t.Fatal("Native(nil) error = nil")
	}
	if err := (*Database)(nil).Migrate(context.Background()); err == nil {
		t.Fatal("nil Database.Migrate() error = nil")
	}
	if err := (&Database{}).Migrate(nilContext()); err == nil {
		t.Fatal("Migrate(nil context) error = nil")
	}

	native, _, repository := newMemoryRepository(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := repository.Save(
		ctx,
		native,
		testRecord("order-canceled"),
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("Save(canceled) error = %v", err)
	}
	if _, _, err := repository.Find(ctx, native, "order-canceled"); !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf("Find(canceled) error = %v", err)
	}
	if err := repository.Save(
		context.Background(),
		nil,
		testRecord("order-no-executor"),
	); err == nil {
		t.Fatal("Save(nil executor) error = nil")
	}
	if err := repository.Save(
		context.Background(),
		native,
		Record{},
	); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("Save(invalid) error = %v", err)
	}
	if _, _, err := (*OrderRepository)(nil).Find(
		context.Background(),
		native,
		"order",
	); err == nil {
		t.Fatal("nil repository Find() error = nil")
	}
}

func TestMemoryDriverRejectsUnsupportedAndInvalidOperations(t *testing.T) {
	t.Parallel()
	database := newMemoryDatabase()
	connector := memoryConnector{database: database}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := connector.Connect(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("Connect(canceled) error = %v", err)
	}
	if _, err := connector.Driver().Open(""); err == nil {
		t.Fatal("Driver.Open() error = nil")
	}

	connection := &memoryConnection{database: database}
	if _, err := connection.Prepare("SELECT 1"); err == nil {
		t.Fatal("Prepare() error = nil")
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := connection.Ping(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("Ping(canceled) error = %v", err)
	}
	if _, err := connection.BeginTx(canceled, driver.TxOptions{}); !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf("BeginTx(canceled) error = %v", err)
	}
	if _, err := connection.ExecContext(
		context.Background(),
		"DROP TABLE commerce_orders",
		nil,
	); err == nil {
		t.Fatal("ExecContext(unknown) error = nil")
	}
	if _, err := connection.QueryContext(
		context.Background(),
		"SELECT 1",
		nil,
	); err == nil {
		t.Fatal("QueryContext(unknown) error = nil")
	}
	if _, err := connection.QueryContext(
		context.Background(),
		findOrderSQL,
		[]driver.NamedValue{{Value: 42}},
	); err == nil {
		t.Fatal("QueryContext(non-string ID) error = nil")
	}

	tx, err := connection.Begin()
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if _, err := connection.Begin(); err == nil {
		t.Fatal("Begin(second) error = nil")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if err := tx.Rollback(); !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("Rollback(second) error = %v", err)
	}
	if err := (*memoryTransaction)(nil).Commit(); err == nil {
		t.Fatal("nil transaction Commit() error = nil")
	}
	if err := (*memoryTransaction)(nil).Rollback(); err == nil {
		t.Fatal("nil transaction Rollback() error = nil")
	}
}

func TestMemoryDriverTransactionAndRowsBoundaries(t *testing.T) {
	t.Parallel()
	database := newMemoryDatabase()
	connection := &memoryConnection{database: database}
	txValue, err := connection.Begin()
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	tx, ok := txValue.(*memoryTransaction)
	if !ok {
		t.Fatalf("Begin() transaction type = %T", txValue)
	}
	arguments := recordNamedValues(testRecord("order-transaction"))
	if _, err := connection.ExecContext(
		context.Background(),
		insertOrderSQL,
		arguments,
	); err != nil {
		t.Fatalf("ExecContext(insert) error = %v", err)
	}
	if _, err := connection.ExecContext(
		context.Background(),
		insertOrderSQL,
		arguments,
	); !errors.Is(err, errMemoryDuplicate) {
		t.Fatalf("ExecContext(duplicate in transaction) error = %v", err)
	}
	database.records["order-transaction"] = testRecord("order-existing")
	if err := tx.Commit(); !errors.Is(err, errMemoryDuplicate) {
		t.Fatalf("Commit(duplicate) error = %v", err)
	}
	if err := tx.Commit(); err == nil {
		t.Fatal("Commit(second) error = nil")
	}

	rows := &memoryRows{values: [][]driver.Value{{"value"}}}
	if got := rows.Columns(); len(got) != 5 || got[0] != "id" {
		t.Fatalf("Columns() = %#v", got)
	}
	destination := make([]driver.Value, 1)
	if err := rows.Next(destination); err != nil || destination[0] != "value" {
		t.Fatalf("Next(first) destination=%#v error=%v", destination, err)
	}
	if err := rows.Next(destination); !errors.Is(err, io.EOF) {
		t.Fatalf("Next(end) error = %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("rows.Close() error = %v", err)
	}
}

func TestMemoryDriverValidatesSchemaArgumentsAndMigrationBackend(t *testing.T) {
	t.Parallel()
	database := newMemoryDatabase()
	database.schema = false
	connection := &memoryConnection{database: database}
	if _, err := connection.insert(testRecord("order-before-schema")); !errors.Is(
		err,
		errMemorySchema,
	) {
		t.Fatalf("insert(before schema) error = %v", err)
	}
	if _, _, err := connection.find("order-before-schema"); !errors.Is(
		err,
		errMemorySchema,
	) {
		t.Fatalf("find(before schema) error = %v", err)
	}
	if _, err := connection.createSchema(); err != nil {
		t.Fatalf("createSchema() error = %v", err)
	}
	if _, err := recordArguments(nil); err == nil {
		t.Fatal("recordArguments(short) error = nil")
	}
	invalidArguments := recordNamedValues(testRecord("order-invalid"))
	invalidArguments[2].Value = "two"
	if _, err := recordArguments(invalidArguments); err == nil {
		t.Fatal("recordArguments(invalid type) error = nil")
	}
	if value, ok := integerArgument("one"); ok || value != 0 {
		t.Fatalf("integerArgument(string) = %d, %t", value, ok)
	}
	if normalized := normalizeSQL(" SELECT \n  1 "); normalized != "SELECT 1" {
		t.Fatalf("normalizeSQL() = %q", normalized)
	}

	for name, backend := range map[string]*memoryMigrationBackend{
		"nil":              nil,
		"missing database": {},
		"empty database":   {database: &Database{}},
	} {
		if err := backend.RunLocked(
			context.Background(),
			func(context.Context, migration.Session) error { return nil },
		); err == nil {
			t.Fatalf("%s RunLocked() error = nil", name)
		}
	}
	backend := &memoryMigrationBackend{
		database: &Database{
			native: sql.OpenDB(memoryConnector{database: database}),
			memory: database,
		},
	}
	t.Cleanup(func() {
		if err := backend.database.native.Close(); err != nil {
			t.Errorf("backend database close error = %v", err)
		}
	})
	if err := backend.RunLocked(nilContext(), func(
		context.Context,
		migration.Session,
	) error {
		return nil
	}); err == nil {
		t.Fatal("RunLocked(nil context) error = nil")
	}
	if err := backend.RunLocked(context.Background(), nil); err == nil {
		t.Fatal("RunLocked(nil work) error = nil")
	}
}

func TestStorageMetadataBoundaries(t *testing.T) {
	t.Parallel()
	if mode := (*Database)(nil).Mode(); mode != "" {
		t.Fatalf("nil Database.Mode() = %q", mode)
	}
	if count := (*Database)(nil).AppliedMigrations(); count != 0 {
		t.Fatalf("nil Database.AppliedMigrations() = %d", count)
	}
	postgresDatabase := &Database{native: sql.OpenDB(
		memoryConnector{database: newMemoryDatabase()},
	)}
	t.Cleanup(func() {
		if err := postgresDatabase.native.Close(); err != nil {
			t.Errorf("postgres database close error = %v", err)
		}
	})
	if mode := postgresDatabase.Mode(); mode != "postgres" {
		t.Fatalf("postgres Database.Mode() = %q", mode)
	}
	if count := postgresDatabase.AppliedMigrations(); count != 0 {
		t.Fatalf("postgres Database.AppliedMigrations() = %d", count)
	}
	if _, err := (&Database{}).migrationBackend(); err == nil {
		t.Fatal("empty Database.migrationBackend() error = nil")
	}
	if _, err := Transactions(nil); err == nil {
		t.Fatal("Transactions(nil) error = nil")
	}
}

func newMemoryRepository(
	t *testing.T,
) (data.Executor, *data.Manager, *OrderRepository) {
	t.Helper()
	database, cleanup, err := OpenDatabase(Settings{URL: memoryURL})
	if err != nil {
		t.Fatalf("OpenDatabase() error = %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := cleanup(context.Background()); cleanupErr != nil {
			t.Errorf("database cleanup error = %v", cleanupErr)
		}
	})
	if migrationErr := database.Migrate(context.Background()); migrationErr != nil {
		t.Fatalf("Migrate() error = %v", migrationErr)
	}
	native, err := Native(database)
	if err != nil {
		t.Fatalf("Native() error = %v", err)
	}
	manager, err := Transactions(native)
	if err != nil {
		t.Fatalf("Transactions() error = %v", err)
	}
	repository, err := NewOrderRepository()
	if err != nil {
		t.Fatalf("NewOrderRepository() error = %v", err)
	}
	return native, manager, repository
}

func testRecord(id string) Record {
	return Record{
		ID:              id,
		SKU:             "SKU-RED",
		Quantity:        2,
		TotalCents:      5000,
		AuthorizationID: "payment-000001",
	}
}

func validTransactionDefinition() data.Definition {
	return data.Definition{
		ID:     "commerce.orders.place",
		Module: moduleID,
	}
}

func recordNamedValues(record Record) []driver.NamedValue {
	return []driver.NamedValue{
		{Ordinal: 1, Value: record.ID},
		{Ordinal: 2, Value: record.SKU},
		{Ordinal: 3, Value: int64(record.Quantity)},
		{Ordinal: 4, Value: int64(record.TotalCents)},
		{Ordinal: 5, Value: record.AuthorizationID},
	}
}

func nilContext() context.Context {
	return nil
}
