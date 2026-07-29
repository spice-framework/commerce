package main

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	spiceasync "github.com/StevenBuglione/spice/async"
	spicecache "github.com/StevenBuglione/spice/cache"
	"github.com/StevenBuglione/spice/config"
	spiceevent "github.com/StevenBuglione/spice/event"
	"github.com/StevenBuglione/spice/examples/commerce/orders"
	spiceapp "github.com/StevenBuglione/spice/internal/spicegen/commerce"
	"github.com/StevenBuglione/spice/lifecycle"
	"github.com/StevenBuglione/spice/management"
	"github.com/StevenBuglione/spice/security"
	"github.com/StevenBuglione/spice/spicetest"
	"github.com/StevenBuglione/spice/web"
)

func TestGeneratedCommandCheckConstructsCommerceApplication(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := spiceapp.RunCommand(spiceapp.CommandOptions{
		Context:   context.Background(),
		Arguments: []string{"-check"},
		Stdout:    &stdout,
		Stderr:    &stderr,
		Logger:    discardLogger(),
	})
	if exitCode != spiceapp.ExitSuccess {
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
	exitCode := spiceapp.RunCommand(spiceapp.CommandOptions{
		Context:   context.Background(),
		Arguments: []string{"-check"},
		Stderr:    &logs,
		Logger:    slog.New(slog.NewJSONHandler(&logs, nil)),
		Application: spiceapp.ApplicationOptions{
			Sources: []config.Source{source},
		},
	})
	if exitCode != spiceapp.ExitFailure ||
		!strings.Contains(logs.String(), "commerce.server.read-header-timeout") ||
		strings.Contains(logs.String(), "secret-invalid-duration") {
		t.Fatalf("RunCommand() exit=%d logs=%q", exitCode, logs.String())
	}
}

func TestGeneratedCommerceConfigurationOverrides(t *testing.T) {
	t.Parallel()
	source := mapSource(t, map[string]string{
		"commerce.mail.transport":               "test",
		"commerce.mail.from":                    "Commerce <sender@example.com>",
		"commerce.mail.recipient":               "Recipient <recipient@example.com>",
		"commerce.mail.test-capacity":           "7",
		"commerce.mail.smtp-address":            "smtp.example:465",
		"commerce.mail.smtp-server-name":        "smtp.example",
		"commerce.mail.smtp-mode":               "implicit-tls",
		"commerce.mail.smtp-username":           "commerce",
		"commerce.mail.smtp-password":           "not-logged",
		"commerce.mail.timeout":                 "3s",
		"commerce.mail.max-attempts":            "2",
		"commerce.server.developer-token":       "commerce-override-token",
		"spice.cache.commerce.catalog.capacity": "17",
		"spice.cache.commerce.catalog.ttl":      "45s",
	})
	application, err := spiceapp.NewApplicationWithOptions(
		context.Background(),
		spiceapp.ApplicationOptions{
			Sources: []config.Source{source},
			Logger:  discardLogger(),
		},
	)
	if err != nil {
		t.Fatalf("NewApplicationWithOptions() error = %v", err)
	}
	if err := application.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestGeneratedCommerceApplicationStartsAndStops(t *testing.T) {
	t.Parallel()
	overrides := mapSource(t, map[string]string{
		"commerce.server.address": "127.0.0.1:0",
		"spice.shutdown-timeout":  "250ms",
	})
	asyncResults := make(chan spiceasync.Result, 1)
	testContext, err := spicetest.NewContext(
		context.Background(),
		func(ctx context.Context) (*spiceapp.Application, error) {
			return spiceapp.NewApplicationWithOptions(
				ctx,
				spiceapp.ApplicationOptions{
					Sources: []config.Source{overrides},
					Logger:  discardLogger(),
					AsyncObservers: []spiceasync.Observer{
						func(_ context.Context, result spiceasync.Result) {
							asyncResults <- result
						},
					},
				},
			)
		},
		spicetest.ContextOptions{ShutdownTimeout: 2 * time.Second},
	)
	if err != nil {
		t.Fatalf("spicetest.NewContext() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := testContext.Close(); closeErr != nil {
			t.Errorf("application test context Close() error = %v", closeErr)
		}
	})
	application := testContext.Application()
	if application.ShutdownTimeout() != 250*time.Millisecond {
		t.Fatalf("ShutdownTimeout() = %s, want 250ms", application.ShutdownTimeout())
	}
	components := application.Components()
	if components.StripeProcessor == nil ||
		components.OfflineProcessor == nil ||
		components.OrdersService == nil ||
		components.OrderRepository == nil ||
		components.Delivery == nil {
		t.Fatal("Components() has missing required singleton beans")
	}
	if got := application.State(); got != lifecycle.StateReady {
		t.Fatalf("State() = %s, want %s", got, lifecycle.StateReady)
	}
	if err := application.SubmitServiceVerifySKU(
		context.Background(),
		"SKU-RED",
	); err != nil {
		t.Fatalf("SubmitServiceVerifySKU() error = %v", err)
	}

	if err := testContext.Close(); err != nil {
		t.Fatalf("test context Close() error = %v", err)
	}
	if got := application.State(); got != lifecycle.StateStopped {
		t.Fatalf("State() = %s, want %s", got, lifecycle.StateStopped)
	}
	result := <-asyncResults
	if result.Err != nil ||
		result.Panicked ||
		result.Definition.Module !=
			"github.com/StevenBuglione/spice/examples/commerce/inventory" ||
		!strings.Contains(result.Definition.ID, "VerifySKU") {
		t.Fatalf("async result = %#v", result)
	}
	if snapshot := application.AsyncSnapshot(); snapshot != (spiceasync.Snapshot{
		Submitted: 1,
		Completed: 1,
		Closed:    true,
	}) {
		t.Fatalf("AsyncSnapshot() = %#v", snapshot)
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
			observation.Err == nil &&
			observation.Module ==
				"github.com/StevenBuglione/spice/examples/commerce/platform" {
			startedOnce.Do(func() { close(started) })
		}
	}
	var receivedTimeout time.Duration
	freshShutdown := false
	results := make(chan int, 1)
	go func() {
		results <- spiceapp.RunCommand(spiceapp.CommandOptions{
			Context:         runContext,
			Logger:          discardLogger(),
			ShutdownTimeout: 500 * time.Millisecond,
			ShutdownContext: func(timeout time.Duration) (context.Context, context.CancelFunc) {
				receivedTimeout = timeout
				shutdownContext, cancel := context.WithTimeout(context.Background(), timeout)
				freshShutdown = shutdownContext != runContext && shutdownContext.Err() == nil
				return shutdownContext, cancel
			},
			Application: spiceapp.ApplicationOptions{
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
		if exitCode != spiceapp.ExitSuccess {
			t.Fatalf("RunCommand() exit = %d", exitCode)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("commerce command did not stop")
	}
	if receivedTimeout != 500*time.Millisecond || !freshShutdown {
		t.Fatalf("shutdown timeout=%s fresh=%t", receivedTimeout, freshShutdown)
	}
}

func TestCommerceDeveloperProof(t *testing.T) {
	t.Parallel()
	var cacheObservations []spicecache.Observation
	eventObserver := &commerceEventObserver{}
	var authorizationDecisions []security.Decision
	server, err := spicetest.NewHTTP(
		context.Background(),
		func(ctx context.Context) (spicetest.HTTPApplication, error) {
			return spiceapp.NewApplicationWithOptions(
				ctx,
				spiceapp.ApplicationOptions{
					Logger: discardLogger(),
					CacheObservers: []spicecache.Observer{
						func(_ context.Context, observation spicecache.Observation) {
							cacheObservations = append(cacheObservations, observation)
						},
					},
					EventObservers: []spiceevent.Observer{eventObserver},
					Middleware: []web.Middleware{
						commerceTestAuthentication(t),
					},
					AuthorizationObservers: []security.Observer{
						func(_ context.Context, decision security.Decision) {
							authorizationDecisions = append(
								authorizationDecisions,
								decision,
							)
						},
					},
				},
			)
		},
		spicetest.HTTPOptions{ShutdownTimeout: time.Second},
	)
	if err != nil {
		t.Fatalf("spicetest.NewHTTP() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := server.Close(); closeErr != nil {
			t.Errorf("HTTP test slice Close() error = %v", closeErr)
		}
	})

	order := placeOrder(t, server, 2, "full")
	if order.ID != "order-000001" || order.Quantity != 2 || order.TotalCents != 5000 {
		t.Fatalf("POST /orders response = %#v", order)
	}
	assertGETStatus(t, server, "/orders/"+order.ID, http.StatusOK, "full")
	assertGETStatus(t, server, "/orders/"+order.ID, http.StatusOK, "full")
	assertProblem(
		t,
		server,
		`{"quantity":0}`,
		http.StatusBadRequest,
		"quantity must be positive",
		"full",
	)
	assertGETStatus(t, server, "/orders/missing", http.StatusNotFound, "full")
	receiptResponse, err := server.Do(
		context.Background(),
		spicetest.HTTPRequest{
			Method: http.MethodPost,
			Path:   "/orders/" + order.ID + "/receipt",
			Header: commerceAuthorizationHeader("full"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if receiptResponse.StatusCode != http.StatusOK {
		t.Fatalf(
			"POST receipt status=%d body=%s",
			receiptResponse.StatusCode,
			receiptResponse.Body,
		)
	}
	var receipt orders.ReceiptResponse
	if err := receiptResponse.DecodeJSON(&receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.MessageID != "receipt-"+order.ID+"@commerce.example" ||
		receipt.Transport != "test" ||
		!receipt.Accepted ||
		receipt.Attachment != order.ID+".txt" {
		t.Fatalf("receipt response = %#v", receipt)
	}
	for range 2 {
		var catalog orders.CatalogResponse
		decodeGET(t, server, "/catalog", &catalog)
		if catalog.SKU != "SKU-RED" ||
			catalog.UnitPriceCents != 2500 {
			t.Fatalf("catalog response = %#v", catalog)
		}
	}
	assertProblemRequest(
		t,
		server,
		spicetest.HTTPRequest{
			Method: http.MethodPost,
			Path:   "/orders",
			JSON:   map[string]int{"quantity": 1},
		},
		http.StatusUnauthorized,
		"Authentication required",
	)
	assertProblemRequest(
		t,
		server,
		spicetest.HTTPRequest{
			Method: http.MethodPost,
			Path:   "/orders",
			Header: commerceAuthorizationHeader("read"),
			JSON:   map[string]int{"quantity": 1},
		},
		http.StatusForbidden,
		"Forbidden",
	)

	var snapshot management.HTTPMetricsSnapshot
	decodeGET(t, server, "/actuator/metrics", &snapshot)
	requestsByMethod := make(map[string]uint64, len(snapshot.Routes))
	for _, route := range snapshot.Routes {
		requestsByMethod[route.Route.Method] += route.Requests
	}
	if len(snapshot.Routes) != 4 ||
		requestsByMethod[http.MethodGet] != 5 ||
		requestsByMethod[http.MethodPost] != 5 {
		t.Fatalf("metrics snapshot = %#v", snapshot)
	}
	wantCacheOperations := []spicecache.Operation{
		spicecache.OperationGet,
		spicecache.OperationPut,
		spicecache.OperationGet,
	}
	if len(cacheObservations) != len(wantCacheOperations) {
		t.Fatalf("cache observations = %#v", cacheObservations)
	}
	if len(eventObserver.interactions) != 3 {
		t.Fatalf("event observations = %#v", eventObserver)
	}
	eventInteraction := eventObserver.interactions[0]
	if eventObserver.results != 3 ||
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
	if cacheObservations[0].Definition.ID != "commerce.catalog" ||
		cacheObservations[0].Definition.Module !=
			"github.com/StevenBuglione/spice/examples/commerce/orders" ||
		cacheObservations[0].Hit ||
		!cacheObservations[2].Hit {
		t.Fatalf("cache observations = %#v", cacheObservations)
	}
	if len(authorizationDecisions) != 8 {
		t.Fatalf("authorization decisions = %#v", authorizationDecisions)
	}
	decisionReasons := make(map[security.Reason]int)
	allowedDecisions := 0
	for _, decision := range authorizationDecisions {
		decisionReasons[decision.Reason]++
		if decision.Allowed {
			allowedDecisions++
		}
	}
	if allowedDecisions != 6 ||
		decisionReasons[security.ReasonUnauthenticated] != 1 ||
		decisionReasons[security.ReasonScope] != 1 {
		t.Fatalf("authorization decisions = %#v", authorizationDecisions)
	}
	var info map[string]string
	decodeGET(t, server, "/actuator/info", &info)
	if info["application"] != "commerce" ||
		info["framework"] != "Spice" ||
		info["module"] != "github.com/StevenBuglione/spice/examples/commerce" ||
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
		properties["commerce.database.url"].Value != "<redacted>" ||
		!properties["commerce.database.url"].Secret ||
		properties["commerce.mail.transport"].Value != "test" ||
		properties["commerce.mail.recipient"].Value != "<redacted>" ||
		!properties["commerce.mail.recipient"].Secret ||
		properties["commerce.mail.smtp-password"].Value != "" ||
		properties["commerce.mail.smtp-password"].Resolved ||
		!properties["commerce.mail.smtp-password"].Secret ||
		properties["commerce.server.developer-token"].Value != "" ||
		properties["commerce.server.developer-token"].Resolved ||
		!properties["commerce.server.developer-token"].Secret ||
		properties["spice.async.max-concurrency"].Value != "16" ||
		properties["spice.cache.commerce.catalog.capacity"].Value !=
			"256" ||
		properties["spice.cache.commerce.catalog.ttl"].Value != "5m" {
		t.Fatalf("configuration report = %#v", configuration)
	}
	var modules management.ModuleReport
	decodeGET(t, server, "/actuator/modules", &modules)
	if modules.Schema != "spice.modules/v1" ||
		len(modules.Modules) != 6 ||
		len(modules.Edges) != 5 ||
		!slices.Equal(
			modules.UnassignedPackages,
			[]string{
				"github.com/StevenBuglione/spice/examples/commerce",
			},
		) {
		t.Fatalf("module report = %#v", modules)
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
	exitCode := spiceapp.RunCommand(spiceapp.CommandOptions{
		Context:   context.Background(),
		Arguments: []string{"-unknown"},
		Stderr:    &stderr,
		Logger:    discardLogger(),
	})
	if exitCode != spiceapp.ExitUsage || stderr.Len() == 0 {
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
		"management.New",
		"signal.",
		"slog.",
		"NewApplication",
		"RunCommand",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("main.go contains framework assembly %q:\n%s", forbidden, source)
		}
	}
	if !strings.Contains(source, "os.Exit(spiceapp.Main(os.Args[1:]))") ||
		!strings.Contains(source, "// @Application") ||
		!strings.Contains(source, "// @import { Application }") ||
		len(strings.Split(strings.TrimSpace(source), "\n")) > 18 {
		t.Fatalf("main.go is not a tiny process boundary:\n%s", source)
	}
}

func placeOrder(
	t *testing.T,
	server *spicetest.HTTP,
	quantity int,
	token string,
) orders.OrderResponse {
	t.Helper()
	response, err := server.Do(
		context.Background(),
		spicetest.HTTPRequest{
			Method: http.MethodPost,
			Path:   "/orders",
			Header: commerceAuthorizationHeader(token),
			JSON:   map[string]int{"quantity": quantity},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("POST /orders status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	var order orders.OrderResponse
	if err := response.DecodeJSON(&order); err != nil {
		t.Fatal(err)
	}
	return order
}

func assertProblem(
	t *testing.T,
	server *spicetest.HTTP,
	body string,
	status int,
	detail string,
	token string,
) {
	t.Helper()
	response, err := server.Do(
		context.Background(),
		spicetest.HTTPRequest{
			Method: http.MethodPost,
			Path:   "/orders",
			Header: http.Header{
				"Content-Type":  []string{"application/json"},
				"Authorization": []string{"Bearer " + token},
			},
			Body: []byte(body),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	problem, err := response.Problem()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status || problem.Detail != detail {
		t.Fatalf("problem status=%d body=%#v", response.StatusCode, problem)
	}
}

func assertGETStatus(
	t *testing.T,
	server *spicetest.HTTP,
	path string,
	status int,
	token ...string,
) {
	t.Helper()
	header := make(http.Header)
	if len(token) != 0 {
		header = commerceAuthorizationHeader(token[0])
	}
	response, err := server.Do(
		context.Background(),
		spicetest.HTTPRequest{
			Method: http.MethodGet,
			Path:   path,
			Header: header,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status {
		t.Fatalf(
			"GET %s status = %d, want %d, body=%s",
			path,
			response.StatusCode,
			status,
			response.Body,
		)
	}
}

func assertProblemRequest(
	t *testing.T,
	server *spicetest.HTTP,
	request spicetest.HTTPRequest,
	status int,
	title string,
) {
	t.Helper()
	response, err := server.Do(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	problem, err := response.Problem()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status || problem.Title != title {
		t.Fatalf(
			"problem status=%d body=%#v",
			response.StatusCode,
			problem,
		)
	}
}

func decodeGET(
	t *testing.T,
	server *spicetest.HTTP,
	path string,
	target any,
) {
	t.Helper()
	response, err := server.Do(
		context.Background(),
		spicetest.HTTPRequest{Method: http.MethodGet, Path: path},
	)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, want %d", path, response.StatusCode, http.StatusOK)
	}
	if err := response.DecodeJSON(target); err != nil {
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

func commerceTestAuthentication(t *testing.T) web.Middleware {
	t.Helper()
	full, err := security.NewPrincipal(
		"commerce-developer",
		"https://issuer.example",
		nil,
		[]string{"orders:notify", "orders:read", "orders:write"},
	)
	if err != nil {
		t.Fatal(err)
	}
	read, err := security.NewPrincipal(
		"commerce-reader",
		"https://issuer.example",
		nil,
		[]string{"orders:read"},
	)
	if err != nil {
		t.Fatal(err)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			var principal security.Principal
			switch request.Header.Get("Authorization") {
			case "Bearer full":
				principal = full
			case "Bearer read":
				principal = read
			default:
				next.ServeHTTP(writer, request)
				return
			}
			ctx, attachErr := security.WithPrincipal(
				request.Context(),
				principal,
			)
			if attachErr != nil {
				http.Error(
					writer,
					"test authentication failed",
					http.StatusInternalServerError,
				)
				return
			}
			next.ServeHTTP(writer, request.WithContext(ctx))
		})
	}
}

func commerceAuthorizationHeader(token string) http.Header {
	return http.Header{"Authorization": []string{"Bearer " + token}}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
