package platform

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestServerStartsAndDrains(t *testing.T) {
	t.Parallel()
	server, err := NewServer(testSettings(), http.NewServeMux())
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
	}{
		{name: "empty address", settings: Settings{}, handler: http.NewServeMux()},
		{name: "missing timeout", settings: Settings{Address: "127.0.0.1:0"}, handler: http.NewServeMux()},
		{name: "nil handler", settings: testSettings()},
	}
	for _, test := range tests {
		if _, err := NewServer(test.settings, test.handler); err == nil {
			t.Fatalf("NewServer(%s) error = nil", test.name)
		}
	}
}

func TestServerHonorsCanceledStart(t *testing.T) {
	t.Parallel()
	server, err := NewServer(testSettings(), http.NewServeMux())
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
