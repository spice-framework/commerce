package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spice-framework/spice/migration"
)

type memoryMigrationSession struct {
	database *Database
}

func (session *memoryMigrationSession) Applied(context.Context) ([]migration.Applied, error) {
	session.database.memory.mu.Lock()
	defer session.database.memory.mu.Unlock()
	return append([]migration.Applied(nil), session.database.memory.applied...), nil
}

func (session *memoryMigrationSession) Apply(ctx context.Context, entry migration.Migration) error {
	tx, err := session.database.native.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin commerce memory migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, entry.SQL()); err != nil {
		return errors.Join(fmt.Errorf("execute commerce memory migration: %w", err), tx.Rollback())
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit commerce memory migration: %w", err)
	}
	session.database.memory.mu.Lock()
	defer session.database.memory.mu.Unlock()
	session.database.memory.applied = append(session.database.memory.applied, migration.Applied{
		Version: entry.Version(), Module: entry.Module(), Name: entry.Name(),
		Checksum: entry.Checksum(), AppliedAt: time.Now().UTC(),
	})
	return nil
}
