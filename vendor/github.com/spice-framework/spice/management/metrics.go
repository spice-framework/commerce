package management

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/spice-framework/spice/web"
)

// MaxHTTPMetricRoutes is the hard cardinality bound for one collector.
const MaxHTTPMetricRoutes = 4096

// HTTPMetrics is an instance-owned, concurrency-safe generated-route metrics
// collector. Its labels come only from compiler-generated route metadata.
type HTTPMetrics struct {
	mu                  sync.Mutex
	routes              map[web.RouteMetadata]*httpRouteMetric
	maxRoutes           int
	droppedObservations uint64
}

type httpRouteMetric struct {
	requests           uint64
	inFlight           int64
	responses          map[int]uint64
	bytes              uint64
	totalDurationNanos int64
	maxDurationNanos   int64
	panics             uint64
}

// StatusCount is one deterministic HTTP response-status count.
type StatusCount struct {
	Status int    `json:"status"`
	Count  uint64 `json:"count"`
}

// HTTPRouteMetric is one immutable route metrics snapshot.
type HTTPRouteMetric struct {
	Route              web.RouteMetadata `json:"route"`
	Requests           uint64            `json:"requests"`
	InFlight           int64             `json:"in_flight"`
	Responses          []StatusCount     `json:"responses"`
	Bytes              uint64            `json:"bytes"`
	TotalDurationNanos int64             `json:"total_duration_nanos"`
	MaxDurationNanos   int64             `json:"max_duration_nanos"`
	Panics             uint64            `json:"panics"`
}

// HTTPMetricsSnapshot is a deterministic immutable metrics view.
type HTTPMetricsSnapshot struct {
	Routes              []HTTPRouteMetric `json:"routes"`
	DroppedObservations uint64            `json:"dropped_observations"`
}

// NewHTTPMetrics creates an empty generated-route metrics collector.
func NewHTTPMetrics() *HTTPMetrics {
	return &HTTPMetrics{
		routes:    make(map[web.RouteMetadata]*httpRouteMetric),
		maxRoutes: MaxHTTPMetricRoutes,
	}
}

// BeginHTTP implements web.HTTPObserver.
func (metrics *HTTPMetrics) BeginHTTP(
	ctx context.Context,
	route web.RouteMetadata,
) (context.Context, func(web.HTTPResult)) {
	if metrics == nil {
		return ctx, nil
	}
	metrics.mu.Lock()
	if metrics.routes == nil {
		metrics.routes = make(map[web.RouteMetadata]*httpRouteMetric)
	}
	item := metrics.routes[route]
	if item == nil {
		if len(metrics.routes) >= metrics.routeLimit() {
			metrics.droppedObservations++
			metrics.mu.Unlock()
			return ctx, nil
		}
		item = &httpRouteMetric{responses: make(map[int]uint64)}
		metrics.routes[route] = item
	}
	item.requests++
	item.inFlight++
	metrics.mu.Unlock()

	var once sync.Once
	return ctx, func(result web.HTTPResult) {
		once.Do(func() {
			metrics.finish(route, result)
		})
	}
}

// Snapshot returns route and response status metrics in stable order.
func (metrics *HTTPMetrics) Snapshot() HTTPMetricsSnapshot {
	snapshot := HTTPMetricsSnapshot{Routes: []HTTPRouteMetric{}}
	if metrics == nil {
		return snapshot
	}
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	snapshot.DroppedObservations = metrics.droppedObservations
	for route, item := range metrics.routes {
		value := HTTPRouteMetric{
			Route:              route,
			Requests:           item.requests,
			InFlight:           item.inFlight,
			Responses:          make([]StatusCount, 0, len(item.responses)),
			Bytes:              item.bytes,
			TotalDurationNanos: item.totalDurationNanos,
			MaxDurationNanos:   item.maxDurationNanos,
			Panics:             item.panics,
		}
		for status, count := range item.responses {
			value.Responses = append(value.Responses, StatusCount{Status: status, Count: count})
		}
		slices.SortFunc(value.Responses, func(left, right StatusCount) int {
			return left.Status - right.Status
		})
		snapshot.Routes = append(snapshot.Routes, value)
	}
	slices.SortFunc(snapshot.Routes, compareHTTPRouteMetrics)
	return snapshot
}

func (metrics *HTTPMetrics) routeLimit() int {
	if metrics.maxRoutes <= 0 {
		return MaxHTTPMetricRoutes
	}
	return metrics.maxRoutes
}

func (metrics *HTTPMetrics) finish(route web.RouteMetadata, result web.HTTPResult) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	item := metrics.routes[route]
	if item == nil {
		return
	}
	if item.inFlight > 0 {
		item.inFlight--
	}
	item.responses[result.Status]++
	if result.Bytes > 0 {
		item.bytes += uint64(result.Bytes)
	}
	duration := result.Duration.Nanoseconds()
	if duration > 0 {
		item.totalDurationNanos += duration
		if duration > item.maxDurationNanos {
			item.maxDurationNanos = duration
		}
	}
	if result.Panicked {
		item.panics++
	}
}

func compareHTTPRouteMetrics(left, right HTTPRouteMetric) int {
	if compared := strings.Compare(left.Route.Module, right.Route.Module); compared != 0 {
		return compared
	}
	if compared := strings.Compare(left.Route.Pattern, right.Route.Pattern); compared != 0 {
		return compared
	}
	if compared := strings.Compare(left.Route.Method, right.Route.Method); compared != 0 {
		return compared
	}
	return strings.Compare(left.Route.ID, right.Route.ID)
}
