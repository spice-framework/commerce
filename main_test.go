package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	spicecache "github.com/StevenBuglione/spice/cache"
	"github.com/StevenBuglione/spice/config"
	spiceevent "github.com/StevenBuglione/spice/event"
	"github.com/StevenBuglione/spice/examples/commerce/orders"
	commerce "github.com/StevenBuglione/spice/internal/spicegen/commerce"
	"github.com/StevenBuglione/spice/lifecycle"
	"github.com/StevenBuglione/spice/management"
	"github.com/StevenBuglione/spice/web"
)

func TestGeneratedCommandCheckConstructsCommerceApplication(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := commerce.RunCommand(commerce.CommandOptions{
		Context:   context.Background(),
		Arguments: []string{"-check"},
		Stdout:    &stdout,
		Stderr:    &stderr,
		Logger:    discardLogger(),
	})
	if exitCode != commerce.ExitSuccess {
		t.Fatalf("RunCommand() exit = %d, stderr=%q", exitCode, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "Spice commerce ready." {
		t.Fatalf("stdout = %q", got)
	}
}

func TestGeneratedCommandReportsInvalidConfigurationWithoutRawValue(t *testing.T) {
	t.Parallel()
	source := mapSource(t, map[string]string{
		"commerce.server.read-header-timeout": "secret-invalid-duration",
	})
	var logs bytes.Buffer
	exitCode := commerce.RunCommand(commerce.CommandOptions{
		Context:   context.Background(),
		Arguments: []string{"-check"},
		Stderr:    &logs,
		Logger:    slog.New(slog.NewJSONHandler(&logs, nil)),
		Application: commerce.ApplicationOptions{
			Sources: []config.Source{source},
		},
	})
	if exitCode != commerce.ExitFailure ||
		!strings.Contains(logs.String(), "commerce.server.read-header-timeout") ||
		strings.Contains(logs.String(), "secret-invalid-duration") {
		t.Fatalf("RunCommand() exit=%d logs=%q", exitCode, logs.String())
	}
}

func TestGeneratedCommerceApplicationStartsAndStops(t *testing.T) {
	t.Parallel()
	overrides := mapSource(t, map[string]string{
		"commerce.server.address": "127.0.0.1:0",
		"spice.shutdown-timeout":  "250ms",
	})
	application, err := commerce.NewApplicationWithOptions(
		context.Background(),
		commerce.ApplicationOptions{
			Sources: []config.Source{overrides},
			Logger:  discardLogger(),
		},
	)
	if err != nil {
		t.Fatalf("NewApplicationWithOptions() error = %v", err)
	}
	if application.ShutdownTimeout() != 250*time.Millisecond {
		t.Fatalf("ShutdownTimeout() = %s, want 250ms", application.ShutdownTimeout())
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

func TestGeneratedCommerceCommandCancellationUsesFreshShutdownContext(t *testing.T) {
	t.Parallel()
	overrides := mapSource(t, map[string]string{
		"commerce.server.address": "127.0.0.1:0",
	})
	runContext, cancelRun := context.WithCancel(context.Background())
	started := make(chan struct{})
	var startedOnce sync.Once
	observer := func(_ context.Context, observation lifecycle.Observation) {
		if observation.Operation == lifecycle.OperationStart &&
			observation.Phase == lifecycle.PhaseEnd &&
			observation.Err == nil {
			startedOnce.Do(func() { close(started) })
		}
	}
	var receivedTimeout time.Duration
	freshShutdown := false
	results := make(chan int, 1)
	go func() {
		results <- commerce.RunCommand(commerce.CommandOptions{
			Context:         runContext,
			Logger:          discardLogger(),
			ShutdownTimeout: 500 * time.Millisecond,
			ShutdownContext: func(timeout time.Duration) (context.Context, context.CancelFunc) {
				receivedTimeout = timeout
				shutdownContext, cancel := context.WithTimeout(context.Background(), timeout)
				freshShutdown = shutdownContext != runContext && shutdownContext.Err() == nil
				return shutdownContext, cancel
			},
			Application: commerce.ApplicationOptions{
				Sources:   []config.Source{overrides},
				Observers: []lifecycle.Observer{observer},
			},
		})
	}()
	select {
	case <-started:
		cancelRun()
	case <-time.After(5 * time.Second):
		t.Fatal("commerce command did not start")
	}
	select {
	case exitCode := <-results:
		if exitCode != commerce.ExitSuccess {
			t.Fatalf("RunCommand() exit = %d", exitCode)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("commerce command did not stop")
	}
	if receivedTimeout != 500*time.Millisecond || !freshShutdown {
		t.Fatalf("shutdown timeout=%s fresh=%t", receivedTimeout, freshShutdown)
	}
}

func TestGeneratedCommerceHTTPAndManagement(t *testing.T) {
	t.Parallel()
	var cacheObservations []spicecache.Observation
	eventObserver := &commerceEventObserver{}
	application, err := commerce.NewApplicationWithOptions(
		context.Background(),
		commerce.ApplicationOptions{
			Logger: discardLogger(),
			CacheObservers: []spicecache.Observer{
				func(_ context.Context, observation spicecache.Observation) {
					cacheObservations = append(cacheObservations, observation)
				},
			},
			EventObservers: []spiceevent.Observer{eventObserver},
		},
	)
	if err != nil {
		t.Fatalf("NewApplicationWithOptions() error = %v", err)
	}
	t.Cleanup(func() {
		shutdownContext, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if stopErr := application.Stop(shutdownContext); stopErr != nil {
			t.Errorf("Stop() error = %v", stopErr)
		}
	})
	server := httptest.NewServer(application.Handler())
	defer server.Close()

	order := placeOrder(t, server, 2)
	if order.ID != "order-000001" || order.Quantity != 2 || order.TotalCents != 5000 {
		t.Fatalf("POST /orders response = %#v", order)
	}
	assertGETStatus(t, server, "/orders/"+order.ID, http.StatusOK)
	assertGETStatus(t, server, "/orders/"+order.ID, http.StatusOK)
	assertProblem(t, server, `{"quantity":0}`, http.StatusBadRequest, "quantity must be positive")
	assertGETStatus(t, server, "/orders/missing", http.StatusNotFound)

	var snapshot management.HTTPMetricsSnapshot
	decodeGET(t, server, "/actuator/metrics", &snapshot)
	requestsByMethod := make(map[string]uint64, len(snapshot.Routes))
	for _, route := range snapshot.Routes {
		requestsByMethod[route.Route.Method] = route.Requests
	}
	if len(snapshot.Routes) != 2 ||
		requestsByMethod[http.MethodGet] != 3 ||
		requestsByMethod[http.MethodPost] != 2 {
		t.Fatalf("metrics snapshot = %#v", snapshot)
	}
	wantCacheOperations := []spicecache.Operation{
		spicecache.OperationGet,
		spicecache.OperationPut,
		spicecache.OperationGet,
		spicecache.OperationGet,
	}
	if len(cacheObservations) != len(wantCacheOperations) {
		t.Fatalf("cache observations = %#v", cacheObservations)
	}
	if len(eventObserver.interactions) != 1 {
		t.Fatalf("event observations = %#v", eventObserver)
	}
	eventInteraction := eventObserver.interactions[0]
	if eventObserver.results != 1 ||
		eventObserver.lastErr != nil ||
		eventInteraction.Event.Module !=
			"github.com/StevenBuglione/spice/examples/commerce/orders" ||
		eventInteraction.Subscriber.Module !=
			"github.com/StevenBuglione/spice/examples/commerce/orders" ||
		eventInteraction.Subscriber.Order != 10 ||
		!strings.Contains(
			eventInteraction.Event.ID,
			"OrderEvents",
		) ||
		!strings.Contains(
			eventInteraction.Subscriber.ID,
			"ViewAudit",
		) {
		t.Fatalf("event observations = %#v", eventObserver)
	}
	if got := cacheOperations(cacheObservations); !slices.Equal(
		got,
		wantCacheOperations,
	) {
		t.Fatalf("cache operations = %v, want %v", got, wantCacheOperations)
	}
	if cacheObservations[0].Definition.ID != "commerce.orders.by-id" ||
		cacheObservations[0].Definition.Module !=
			"github.com/StevenBuglione/spice/examples/commerce/orders" ||
		cacheObservations[0].Hit ||
		!cacheObservations[2].Hit ||
		cacheObservations[3].Hit {
		t.Fatalf("cache observations = %#v", cacheObservations)
	}
	var info map[string]string
	decodeGET(t, server, "/actuator/info", &info)
	if info["application"] != "commerce" ||
		info["framework"] != "Spice" ||
		info["module"] != "github.com/StevenBuglione/spice/examples/commerce/bootstrap" ||
		len(info) != 3 {
		t.Fatalf("management info = %#v", info)
	}
	var configuration management.ConfigurationReport
	decodeGET(
		t,
		server,
		"/actuator/configprops",
		&configuration,
	)
	properties := make(
		map[string]management.ConfigurationProperty,
		len(configuration.Properties),
	)
	for _, property := range configuration.Properties {
		properties[property.Key] = property
	}
	if properties["commerce.orders.sku"].Value != "SKU-RED" ||
		!properties["commerce.orders.sku"].Default ||
		properties["spice.cache.commerce.orders.by-id.capacity"].Value !=
			"256" ||
		properties["spice.cache.commerce.orders.by-id.ttl"].Value != "5m" {
		t.Fatalf("configuration report = %#v", configuration)
	}
	for _, path := range []string{"/actuator/health", "/actuator/health/liveness"} {
		assertGETStatus(t, server, path, http.StatusOK)
	}
	assertGETStatus(
		t,
		server,
		"/actuator/health/readiness",
		http.StatusServiceUnavailable,
	)
	assertGETStatus(t, server, "/actuator/env", http.StatusNotFound)
}

type commerceEventObserver struct {
	interactions []spiceevent.Interaction
	results      int
	lastErr      error
}

func (observer *commerceEventObserver) BeginEvent(
	ctx context.Context,
	interaction spiceevent.Interaction,
) (context.Context, func(spiceevent.Result)) {
	observer.interactions = append(observer.interactions, interaction)
	return ctx, func(result spiceevent.Result) {
		observer.results++
		observer.lastErr = result.Err
	}
}

func cacheOperations(
	observations []spicecache.Observation,
) []spicecache.Operation {
	result := make([]spicecache.Operation, len(observations))
	for index, observation := range observations {
		result[index] = observation.Operation
	}
	return result
}

func TestGeneratedCommandRejectsUnknownFlag(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	exitCode := commerce.RunCommand(commerce.CommandOptions{
		Context:   context.Background(),
		Arguments: []string{"-unknown"},
		Stderr:    &stderr,
		Logger:    discardLogger(),
	})
	if exitCode != commerce.ExitUsage || stderr.Len() == 0 {
		t.Fatalf("RunCommand() exit=%d stderr=%q", exitCode, stderr.String())
	}
}

func TestCommerceMainIsOnlyTheProcessBoundary(t *testing.T) {
	t.Parallel()
	content, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(content)
	for _, forbidden := range []string{
		"context.",
		"configureManagement",
		"management.",
		"signal.",
		"slog.",
		"NewApplication",
		"RunCommand",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("main.go contains framework assembly %q:\n%s", forbidden, source)
		}
	}
	if !strings.Contains(source, "os.Exit(commerce.Main(os.Args[1:]))") ||
		len(strings.Split(strings.TrimSpace(source), "\n")) > 12 {
		t.Fatalf("main.go is not a tiny process boundary:\n%s", source)
	}
}

func placeOrder(t *testing.T, server *httptest.Server, quantity int) orders.OrderResponse {
	t.Helper()
	body := `{"quantity":` + strconv.Itoa(quantity) + `}`
	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		server.URL+"/orders",
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("POST /orders status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	var order orders.OrderResponse
	if err := json.NewDecoder(response.Body).Decode(&order); err != nil {
		t.Fatal(err)
	}
	return order
}

func assertProblem(
	t *testing.T,
	server *httptest.Server,
	body string,
	status int,
	detail string,
) {
	t.Helper()
	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		server.URL+"/orders",
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var problem web.Problem
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status || problem.Detail != detail {
		t.Fatalf("problem status=%d body=%#v", response.StatusCode, problem)
	}
}

func assertGETStatus(
	t *testing.T,
	server *httptest.Server,
	path string,
	status int,
) {
	t.Helper()
	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		server.URL+path,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		t.Fatalf("GET %s status = %d, want %d", path, response.StatusCode, status)
	}
}

func decodeGET(t *testing.T, server *httptest.Server, path string, target any) {
	t.Helper()
	response, err := server.Client().Get(server.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, want %d", path, response.StatusCode, http.StatusOK)
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}

func mapSource(t *testing.T, values map[string]string) config.Source {
	t.Helper()
	source, err := config.NewMapSource("test", values)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
