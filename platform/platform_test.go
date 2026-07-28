package platform

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/StevenBuglione/spice/examples/commerce/storage"
	"github.com/StevenBuglione/spice/security"
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

func TestDeveloperAuthenticationIsExplicitLoopbackAndScopeBound(t *testing.T) {
	t.Parallel()
	const token = "commerce-local-token"
	downstream := http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		principal, found := security.PrincipalFromContext(request.Context())
		if !found {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		if principal.Subject() != "commerce-developer" ||
			len(principal.Scopes()) != 3 {
			writer.WriteHeader(http.StatusForbidden)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	handler, err := developerAuthentication(
		"127.0.0.1:8081",
		token,
		downstream,
	)
	if err != nil {
		t.Fatalf("developerAuthentication() error = %v", err)
	}
	for _, test := range []struct {
		name   string
		header string
		status int
	}{
		{name: "absent", status: http.StatusUnauthorized},
		{
			name:   "wrong",
			header: "Bearer commerce-wrong-token",
			status: http.StatusUnauthorized,
		},
		{
			name:   "valid",
			header: "Bearer " + token,
			status: http.StatusNoContent,
		},
	} {
		request := httptest.NewRequest(
			http.MethodGet,
			"http://commerce.example/orders",
			nil,
		)
		request.Header.Set("Authorization", test.header)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Fatalf(
				"%s response status = %d, want %d",
				test.name,
				response.Code,
				test.status,
			)
		}
	}
	for _, test := range []struct {
		address string
		token   string
	}{
		{address: "0.0.0.0:8081", token: token},
		{address: "127.0.0.1:8081", token: "short"},
		{address: "127.0.0.1:8081", token: "commerce token with spaces"},
	} {
		if _, err := developerAuthentication(
			test.address,
			test.token,
			downstream,
		); err == nil {
			t.Fatalf(
				"developerAuthentication(%q) error = nil",
				test.address,
			)
		}
	}
	if handler, err := developerAuthentication(
		"0.0.0.0:8081",
		"",
		downstream,
	); err != nil || handler == nil {
		t.Fatalf(
			"disabled developerAuthentication() handler=%v error=%v",
			handler,
			err,
		)
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
