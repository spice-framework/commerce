// @import { Bean, Configuration, Implements, Repository } from "github.com/spice-framework/spice/annotation/core"
// @import { OnStart } from "github.com/spice-framework/spice/annotation/lifecycle"
// @import { Module } from "github.com/spice-framework/spice/annotation/modulith"

// Package storage owns commerce order persistence and its schema.
//
// @Module
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/spice-framework/spice/data"
	"github.com/spice-framework/spice/data/repository"
	"github.com/spice-framework/spice/lifecycle"
	"github.com/spice-framework/spice/migration"
	"github.com/spice-framework/spice/starter/postgres"
)

const (
	moduleID        = "github.com/spice-framework/spice/examples/commerce/storage"
	memoryURL       = "memory://commerce"
	migrationNumber = 202607280001
)

const createOrdersTableSQL = `CREATE TABLE IF NOT EXISTS commerce_orders (
	id text PRIMARY KEY,
	sku text NOT NULL,
	quantity bigint NOT NULL CHECK (quantity > 0),
	total_cents bigint NOT NULL CHECK (total_cents > 0),
	authorization_id text NOT NULL
);`

const insertOrderSQL = `INSERT INTO commerce_orders (
	id,
	sku,
	quantity,
	total_cents,
	authorization_id
) VALUES ($1, $2, $3, $4, $5)`

const findOrderSQL = `SELECT
	id,
	sku,
	quantity,
	total_cents,
	authorization_id
FROM commerce_orders
WHERE id = $1`

var (
	// ErrInvalidRecord reports an invalid order persistence request.
	ErrInvalidRecord = errors.New("invalid order record")
	// ErrDuplicate reports an order ID that is already persisted.
	ErrDuplicate = errors.New("order already exists")
)

// Settings selects the instance-owned database implementation. The default
// memory URL exists only for the zero-dependency reference developer loop.
//
// @Configuration(prefix="commerce.database")
type Settings struct {
	URL           string `spice:"url,default=memory://commerce,env=SPICE_COMMERCE_DATABASE_URL,secret"`
	AllowInsecure bool   `spice:"allow-insecure,default=false,env=SPICE_COMMERCE_DATABASE_ALLOW_INSECURE"`
}

// Record is the persistence representation shared with the orders module.
type Record struct {
	ID              string
	SKU             string
	Quantity        int
	TotalCents      int
	AuthorizationID string
}

// Orders is the exact persistence contract consumed by the orders module.
type Orders interface {
	Save(context.Context, data.Executor, Record) error
	Find(context.Context, data.Executor, string) (Record, bool, error)
}

// Database owns one database/sql pool and the module migration lifecycle.
type Database struct {
	native *sql.DB
	memory *memoryDatabase
}

// OpenDatabase constructs a database without connecting. PostgreSQL uses the
// reviewed starter; the exact memory URL uses an instance-owned test connector.
//
// @Bean
func OpenDatabase(settings Settings) (*Database, lifecycle.Cleanup, error) {
	rawURL := strings.TrimSpace(settings.URL)
	if rawURL != settings.URL || rawURL == "" {
		return nil, nil, errors.New("construct commerce database: URL is required")
	}
	if rawURL == memoryURL {
		memory := newMemoryDatabase()
		native := sql.OpenDB(memoryConnector{database: memory})
		native.SetMaxOpenConns(1)
		native.SetMaxIdleConns(1)
		database := &Database{native: native, memory: memory}
		return database, closeDatabase(native), nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil ||
		(parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		return nil, nil, errors.New(
			"construct commerce database: URL must use memory, postgres, or postgresql",
		)
	}
	native, err := postgres.Open(postgres.Options{
		URL:             rawURL,
		ApplicationName: "spice-commerce",
		AllowInsecure:   settings.AllowInsecure,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("construct commerce database: %w", err)
	}
	return &Database{native: native}, closeDatabase(native), nil
}

func closeDatabase(database *sql.DB) lifecycle.Cleanup {
	return func(context.Context) error {
		if closeErr := database.Close(); closeErr != nil {
			return fmt.Errorf("close commerce database: %w", closeErr)
		}
		return nil
	}
}

// Migrate reconciles and applies the module-owned schema before dependent
// lifecycle hooks start.
//
// @OnStart
func (database *Database) Migrate(ctx context.Context) error {
	if ctx == nil {
		return errors.New("migrate commerce database: context is nil")
	}
	if database == nil || database.native == nil {
		return errors.New("migrate commerce database: database is nil")
	}
	plan, err := migration.NewPlan([]migration.Spec{{
		Version: migrationNumber,
		Module:  moduleID,
		Name:    "create commerce orders",
		SQL:     createOrdersTableSQL,
	}})
	if err != nil {
		return fmt.Errorf("construct commerce migration plan: %w", err)
	}
	backend, err := database.migrationBackend()
	if err != nil {
		return err
	}
	runner, err := migration.NewRunner(backend)
	if err != nil {
		return fmt.Errorf("construct commerce migration runner: %w", err)
	}
	if _, err := runner.Run(ctx, plan); err != nil {
		return fmt.Errorf("migrate commerce database: %w", err)
	}
	return nil
}

func (database *Database) migrationBackend() (migration.Backend, error) {
	if database.memory != nil {
		return &memoryMigrationBackend{database: database}, nil
	}
	backend, err := postgres.NewMigrationBackend(
		database.native,
		postgres.MigrationOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("construct commerce PostgreSQL migration backend: %w", err)
	}
	return backend, nil
}

// Native exposes the owned pool to the generated transaction manager.
//
// @Bean
func Native(database *Database) (*sql.DB, error) {
	if database == nil || database.native == nil {
		return nil, errors.New("provide commerce database: database is nil")
	}
	return database.native, nil
}

// Transactions provides the exact generated transaction boundary.
//
// @Bean
func Transactions(database *sql.DB) (*data.Manager, error) {
	return data.NewManager(database)
}

// ReadExecutor provides the exact interface bean used for non-transactional
// cached reads.
//
// @Bean
func ReadExecutor(database *sql.DB) data.Executor {
	return database
}

// OrderRepository is the reflection-free SQL implementation of Orders.
//
// @Repository(constructor=NewOrderRepository)
// @Implements(Orders)
type OrderRepository struct {
	find *repository.Query[Record]
}

// NewOrderRepository constructs bounded typed query metadata.
func NewOrderRepository() (*OrderRepository, error) {
	find, err := repository.NewQuery(repository.QuerySpec[Record]{
		ID:        "commerce.orders.find",
		Module:    moduleID,
		Statement: findOrderSQL,
		MaxRows:   1,
		Decode:    decodeRecord,
	})
	if err != nil {
		return nil, fmt.Errorf("construct order repository: %w", err)
	}
	return &OrderRepository{find: find}, nil
}

// Save inserts one immutable order through the caller-owned executor.
func (*OrderRepository) Save(
	ctx context.Context,
	executor data.Executor,
	record Record,
) error {
	if ctx == nil {
		return errors.New("save order: context is nil")
	}
	if cause := context.Cause(ctx); cause != nil {
		return fmt.Errorf("save order: %w", cause)
	}
	if err := validateRecord(record); err != nil {
		return err
	}
	if executor == nil {
		return errors.New("save order: executor is nil")
	}
	result, err := executor.ExecContext(
		ctx,
		insertOrderSQL,
		record.ID,
		record.SKU,
		record.Quantity,
		record.TotalCents,
		record.AuthorizationID,
	)
	if err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return fmt.Errorf("save order %q: %w", record.ID, cause)
		}
		if isDuplicate(err) {
			return fmt.Errorf("save order %q: %w", record.ID, ErrDuplicate)
		}
		return fmt.Errorf("save order %q: persistence failed", record.ID)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return fmt.Errorf("save order %q: persistence cardinality is invalid", record.ID)
	}
	return nil
}

// Find returns one persisted order by exact ID.
func (repository *OrderRepository) Find(
	ctx context.Context,
	executor data.Executor,
	id string,
) (Record, bool, error) {
	if repository == nil || repository.find == nil {
		return Record{}, false, errors.New("find order: repository is nil")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Record{}, false, fmt.Errorf("%w: order ID is required", ErrInvalidRecord)
	}
	record, found, err := repository.find.Optional(ctx, executor, id)
	if err != nil {
		return Record{}, false, fmt.Errorf("find order %q: %w", id, err)
	}
	return record, found, nil
}

func validateRecord(record Record) error {
	if strings.TrimSpace(record.ID) == "" ||
		strings.TrimSpace(record.SKU) == "" ||
		record.Quantity <= 0 ||
		record.TotalCents <= 0 ||
		strings.TrimSpace(record.AuthorizationID) == "" {
		return fmt.Errorf("%w: every field must be valid", ErrInvalidRecord)
	}
	return nil
}

func decodeRecord(scanner repository.Scanner) (Record, error) {
	var record Record
	if err := scanner.Scan(
		&record.ID,
		&record.SKU,
		&record.Quantity,
		&record.TotalCents,
		&record.AuthorizationID,
	); err != nil {
		return Record{}, fmt.Errorf("decode order record: %w", err)
	}
	return record, nil
}

func isDuplicate(err error) bool {
	return errors.Is(err, errMemoryDuplicate) ||
		strings.Contains(strings.ToLower(err.Error()), "duplicate key")
}

// Mode reports the selected backend without exposing its secret URL.
func (database *Database) Mode() string {
	if database != nil && database.memory != nil {
		return "memory"
	}
	if database != nil && database.native != nil {
		return "postgres"
	}
	return ""
}

// AppliedMigrations returns the in-memory developer backend's durable registry
// count. PostgreSQL registry inspection remains an explicit database concern.
func (database *Database) AppliedMigrations() int {
	if database == nil || database.memory == nil {
		return 0
	}
	database.memory.mu.Lock()
	defer database.memory.mu.Unlock()
	return len(database.memory.applied)
}
