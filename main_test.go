package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/StevenBuglione/spice/config"
	"github.com/StevenBuglione/spice/examples/commerce/orders"
	commerce "github.com/StevenBuglione/spice/internal/spicegen/commerce"
	"github.com/StevenBuglione/spice/lifecycle"
	"github.com/StevenBuglione/spice/management"
	"github.com/StevenBuglione/spice/web"
)

func TestRunCheckConstructsCommerceApplication(t *testing.T) {
	var stdout bytes.Buffer
	if err := run([]string{"-check"}, &stdout); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got !=
		"Spice commerce ready: inventory -> orders -> payments + platform" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestRunReportsInvalidEnvironmentConfiguration(t *testing.T) {
	t.Setenv("SPICE_COMMERCE_SERVER_READ_HEADER_TIMEOUT", "not-a-duration")
	var stdout bytes.Buffer
	err := run([]string{"-check"}, &stdout)
	if err == nil || !strings.Contains(err.Error(), "commerce.server.read-header-timeout") {
		t.Fatalf("run() error = %v, want property-specific configuration failure", err)
	}
}

func TestGeneratedCommerceApplicationStartsAndStops(t *testing.T) {
	t.Parallel()
	overrides, err := config.NewMapSource(
		"test",
		map[string]string{"commerce.server.address": "127.0.0.1:0"},
	)
	if err != nil {
		t.Fatalf("NewMapSource() error = %v", err)
	}
	application, err := commerce.NewApplicationWithOptions(
		context.Background(),
		commerce.ApplicationOptions{Sources: []config.Source{overrides}},
	)
	if err != nil {
		t.Fatalf("NewApplicationWithOptions() error = %v", err)
	}
	if err := application.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got := application.State(); got != lifecycle.StateReady {
		t.Fatalf("State() = %s, want %s", got, lifecycle.StateReady)
	}

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelShutdown()
	if err := application.Stop(shutdownContext); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if got := application.State(); got != lifecycle.StateStopped {
		t.Fatalf("State() = %s, want %s", got, lifecycle.StateStopped)
	}
}

func TestGeneratedCommerceHTTPAndManagement(t *testing.T) {
	t.Parallel()
	metrics := management.NewHTTPMetrics()
	application, err := commerce.NewApplicationWithOptions(
		context.Background(),
		commerce.ApplicationOptions{HTTPObservers: []web.HTTPObserver{metrics}},
	)
	if err != nil {
		t.Fatalf("NewApplicationWithOptions() error = %v", err)
	}
	if configureErr := configureManagement(application, metrics); configureErr != nil {
		t.Fatalf("configureManagement() error = %v", configureErr)
	}
	server := httptest.NewServer(application.Handler())
	defer server.Close()

	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		server.URL+"/orders",
		strings.NewReader(`{"quantity":2}`),
	)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("POST /orders status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	var order orders.OrderResponse
	if decodeErr := json.NewDecoder(response.Body).Decode(&order); decodeErr != nil {
		t.Fatalf("Decode(order) error = %v", decodeErr)
	}
	if order.ID != "order-000001" || order.Quantity != 2 || order.TotalCents != 5000 {
		t.Fatalf("POST /orders response = %#v", order)
	}

	getResponse, err := server.Client().Get(server.URL + "/orders/" + order.ID)
	if err != nil {
		t.Fatalf("GET /orders/{id} error = %v", err)
	}
	defer getResponse.Body.Close()
	if getResponse.StatusCode != http.StatusOK {
		t.Fatalf("GET /orders/{id} status = %d, want %d", getResponse.StatusCode, http.StatusOK)
	}

	invalidRequest, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		server.URL+"/orders",
		strings.NewReader(`{"quantity":0}`),
	)
	if err != nil {
		t.Fatalf("NewRequestWithContext(invalid) error = %v", err)
	}
	invalidRequest.Header.Set("Content-Type", "application/json")
	invalidResponse, err := server.Client().Do(invalidRequest)
	if err != nil {
		t.Fatalf("Do(invalid) error = %v", err)
	}
	defer invalidResponse.Body.Close()
	var problem web.Problem
	if decodeErr := json.NewDecoder(invalidResponse.Body).Decode(&problem); decodeErr != nil {
		t.Fatalf("Decode(problem) error = %v", decodeErr)
	}
	if invalidResponse.StatusCode != http.StatusBadRequest ||
		problem.Detail != "quantity must be positive" {
		t.Fatalf("invalid order status=%d problem=%#v", invalidResponse.StatusCode, problem)
	}

	missingResponse, err := server.Client().Get(server.URL + "/orders/missing")
	if err != nil {
		t.Fatalf("GET /orders/missing error = %v", err)
	}
	defer missingResponse.Body.Close()
	if missingResponse.StatusCode != http.StatusNotFound {
		t.Fatalf(
			"GET /orders/missing status = %d, want %d",
			missingResponse.StatusCode,
			http.StatusNotFound,
		)
	}

	metricsResponse, err := server.Client().Get(server.URL + "/actuator/metrics")
	if err != nil {
		t.Fatalf("GET /actuator/metrics error = %v", err)
	}
	defer metricsResponse.Body.Close()
	var snapshot management.HTTPMetricsSnapshot
	if err := json.NewDecoder(metricsResponse.Body).Decode(&snapshot); err != nil {
		t.Fatalf("Decode(metrics) error = %v", err)
	}
	if len(snapshot.Routes) != 2 ||
		snapshot.Routes[0].Requests != 2 ||
		snapshot.Routes[1].Requests != 2 {
		t.Fatalf("metrics snapshot = %#v", snapshot)
	}
}

func TestRunRejectsUnknownFlag(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	if err := run([]string{"-unknown"}, &stdout); err == nil {
		t.Fatal("run() error = nil")
	}
}
