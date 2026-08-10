// @import { Bean } from "github.com/spice-framework/spice/annotation/core"
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

	"github.com/spice-framework/spice/lifecycle"
	"github.com/spice-framework/spice/migration"
	postgres "github.com/spice-framework/starter-postgres"
)

// Database owns one database/sql pool and the module migration lifecycle.
type Database struct {
	native *sql.DB
	memory *memoryDatabase
}

// NewDatabase constructs a database without connecting. PostgreSQL uses the
// reviewed starter; the exact memory URL uses an instance-owned test connector.
//
// @Bean
func NewDatabase(settings Settings) (*Database, lifecycle.Cleanup, error) {
	rawURL := strings.TrimSpace(settings.URL)
	if rawURL != settings.URL || rawURL == "" {
		return nil, nil, errors.New("construct commerce database: URL is required")
	}
	if rawURL == memoryURL {
		memory := &memoryDatabase{records: make(map[string]Record), schema: true}
		native := sql.OpenDB(memoryConnector{database: memory})
		native.SetMaxOpenConns(1)
		native.SetMaxIdleConns(1)
		database := &Database{native: native, memory: memory}
		return database, database.cleanup(), nil
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
	database := &Database{native: native}
	return database, database.cleanup(), nil
}

func (database *Database) cleanup() lifecycle.Cleanup {
	return func(context.Context) error {
		if closeErr := database.native.Close(); closeErr != nil {
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
