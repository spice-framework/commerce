package storage

import (
	"context"
	"errors"

	"github.com/spice-framework/spice/migration"
)

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
	if backend == nil || backend.database == nil || backend.database.memory == nil ||
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
