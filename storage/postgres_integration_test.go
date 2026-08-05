//go:build integration

package storage

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spice-framework/spice/data"
)

func TestPostgreSQLPersistenceSurvivesApplicationDatabaseRestart(t *testing.T) {
	connectionURL := strings.TrimSpace(os.Getenv("SPICE_TEST_POSTGRES_URL"))
	if connectionURL == "" {
		t.Skip("SPICE_TEST_POSTGRES_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	settings := Settings{
		URL:           connectionURL,
		AllowInsecure: strings.Contains(connectionURL, "sslmode=disable"),
	}

	first, firstCleanup, err := OpenDatabase(settings)
	if err != nil {
		t.Fatalf("OpenDatabase(first) error = %v", err)
	}
	if err := first.Migrate(ctx); err != nil {
		t.Fatalf("Migrate(first) error = %v", err)
	}
	firstNative, err := Native(first)
	if err != nil {
		t.Fatalf("Native(first) error = %v", err)
	}
	const orderID = "order-postgres-developer-proof"
	if _, err := firstNative.ExecContext(
		ctx,
		"DELETE FROM commerce_orders WHERE id = $1",
		orderID,
	); err != nil {
		t.Fatalf("delete prior integration order: %v", err)
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cleanupCancel()
		database, cleanup, openErr := OpenDatabase(settings)
		if openErr != nil {
			t.Errorf("OpenDatabase(cleanup) error = %v", openErr)
			return
		}
		defer func() {
			if cleanupErr := cleanup(cleanupContext); cleanupErr != nil {
				t.Errorf("database cleanup error = %v", cleanupErr)
			}
		}()
		native, nativeErr := Native(database)
		if nativeErr != nil {
			t.Errorf("Native(cleanup) error = %v", nativeErr)
			return
		}
		if _, deleteErr := native.ExecContext(
			cleanupContext,
			"DELETE FROM commerce_orders WHERE id = $1",
			orderID,
		); deleteErr != nil {
			t.Errorf("delete integration order: %v", deleteErr)
		}
	})

	manager, err := Transactions(firstNative)
	if err != nil {
		t.Fatalf("Transactions(first) error = %v", err)
	}
	repository, err := NewOrderRepository()
	if err != nil {
		t.Fatalf("NewOrderRepository() error = %v", err)
	}
	record := testRecord(orderID)
	if err := manager.Within(
		ctx,
		validTransactionDefinition(),
		func(transactionContext context.Context, executor data.Executor) error {
			return repository.Save(transactionContext, executor, record)
		},
	); err != nil {
		t.Fatalf("Within(save) error = %v", err)
	}
	if err := firstCleanup(ctx); err != nil {
		t.Fatalf("first database cleanup error = %v", err)
	}

	second, secondCleanup, err := OpenDatabase(settings)
	if err != nil {
		t.Fatalf("OpenDatabase(second) error = %v", err)
	}
	defer func() {
		if cleanupErr := secondCleanup(context.Background()); cleanupErr != nil {
			t.Errorf("second database cleanup error = %v", cleanupErr)
		}
	}()
	if err := second.Migrate(ctx); err != nil {
		t.Fatalf("Migrate(second) error = %v", err)
	}
	secondNative, err := Native(second)
	if err != nil {
		t.Fatalf("Native(second) error = %v", err)
	}
	persisted, found, err := repository.Find(ctx, secondNative, orderID)
	if err != nil || !found || persisted != record {
		t.Fatalf("Find(after restart) = %#v, %t, %v", persisted, found, err)
	}
}
