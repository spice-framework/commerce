package platform

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/StevenBuglione/spice/examples/commerce/storage"
)

func TestServerStartsAndDrains(t *testing.T) {
	t.Parallel()
	server, err := NewServer(testSettings(), http.NewServeMux(), testDatabase(t))
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if server.Address() == testSettings().Address {
		t.Fatalf("Address() = %q, want bound ephemeral address", server.Address())
	}

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelShutdown()
	if err := server.Stop(shutdownContext); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestServerRejectsUnsafeConfiguration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		settings Settings
		handler  *http.ServeMux
		database *storage.Database
	}{
		{
			name:     "empty address",
			settings: Settings{},
			handler:  http.NewServeMux(),
			database: testDatabase(t),
		},
		{
			name:     "missing timeout",
			settings: Settings{Address: "127.0.0.1:0"},
			handler:  http.NewServeMux(),
			database: testDatabase(t),
		},
		{
			name:     "nil handler",
			settings: testSettings(),
			database: testDatabase(t),
		},
		{
			name:     "nil database",
			settings: testSettings(),
			handler:  http.NewServeMux(),
		},
	}
	for _, test := range tests {
		if _, err := NewServer(
			test.settings,
			test.handler,
			test.database,
		); err == nil {
			t.Fatalf("NewServer(%s) error = nil", test.name)
		}
	}
}

func TestServerHonorsCanceledStart(t *testing.T) {
	t.Parallel()
	server, err := NewServer(testSettings(), http.NewServeMux(), testDatabase(t))
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := server.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start() error = %v, want context.Canceled", err)
	}
	if got := server.Address(); got != testSettings().Address {
		t.Fatalf("Address() = %q, want configured address", got)
	}
}

func testSettings() Settings {
	return Settings{
		Address:           "127.0.0.1:0",
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       time.Second,
		WriteTimeout:      time.Second,
		IdleTimeout:       time.Second,
	}
}

func testDatabase(t *testing.T) *storage.Database {
	t.Helper()
	database, cleanup, err := storage.OpenDatabase(storage.Settings{
		URL: "memory://commerce",
	})
	if err != nil {
		t.Fatalf("storage.OpenDatabase() error = %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := cleanup(context.Background()); cleanupErr != nil {
			t.Errorf("storage cleanup error = %v", cleanupErr)
		}
	})
	return database
}
