// Package management provides deterministic, opt-in production management
// probes and HTTP endpoints without a global registry.
package management

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	"path"
	"slices"
	"strings"

	"github.com/spice-framework/spice/config"
	"github.com/spice-framework/spice/lifecycle"
	"github.com/spice-framework/spice/web"
)

// Group identifies one independently queryable health concern.
type Group string

const (
	// GroupHealth contains broad application health checks.
	GroupHealth Group = "health"
	// GroupLiveness contains checks that decide whether a process is alive.
	GroupLiveness Group = "liveness"
	// GroupReadiness contains checks that decide whether traffic is safe.
	GroupReadiness Group = "readiness"
)

// Status is the stable client-facing result of a probe or report.
type Status string

const (
	// StatusUp means every selected check passed.
	StatusUp Status = "UP"
	// StatusDown means at least one selected check failed.
	StatusDown Status = "DOWN"
)

// Probe is one caller-owned, context-aware health check.
type Probe func(context.Context) error

// Check declares one named probe in one or more management groups. Module is
// optional ownership metadata and is safe to expose.
type Check struct {
	Name   string
	Module string
	Groups []Group
	Probe  Probe
}

// Component is one safe client-facing check result. Probe errors are
// intentionally excluded.
type Component struct {
	Name   string `json:"name"`
	Module string `json:"module,omitempty"`
	Status Status `json:"status"`
}

// Report is a deterministic group result.
type Report struct {
	Group      Group       `json:"group"`
	Status     Status      `json:"status"`
	Components []Component `json:"components"`
}

// Manager is an immutable collection of validated health checks.
type Manager struct {
	checks map[Group][]Check
}

// New validates, copies, and deterministically orders health checks.
func New(checks ...Check) (*Manager, error) {
	grouped := map[Group][]Check{
		GroupHealth:    {},
		GroupLiveness:  {},
		GroupReadiness: {},
	}
	seen := make(map[string]struct{})
	for checkIndex, check := range checks {
		if !validName(check.Name) {
			return nil, fmt.Errorf(
				"management check %d name %q must contain only letters, digits, '.', '_', or '-'",
				checkIndex,
				check.Name,
			)
		}
		if check.Probe == nil {
			return nil, fmt.Errorf("management check %q probe is nil", check.Name)
		}
		if len(check.Groups) == 0 {
			return nil, fmt.Errorf("management check %q declares no groups", check.Name)
		}
		for _, group := range check.Groups {
			if !validGroup(group) {
				return nil, fmt.Errorf("management check %q has unsupported group %q", check.Name, group)
			}
			key := string(group) + "\x00" + check.Name
			if _, duplicate := seen[key]; duplicate {
				return nil, fmt.Errorf("management group %q contains duplicate check %q", group, check.Name)
			}
			seen[key] = struct{}{}
			grouped[group] = append(grouped[group], Check{
				Name:   check.Name,
				Module: check.Module,
				Groups: []Group{group},
				Probe:  check.Probe,
			})
		}
	}
	for group := range grouped {
		slices.SortFunc(grouped[group], func(left, right Check) int {
			if compared := strings.Compare(left.Name, right.Name); compared != 0 {
				return compared
			}
			return strings.Compare(left.Module, right.Module)
		})
	}
	return &Manager{checks: grouped}, nil
}

// Report runs one group in deterministic check order. Probe failures are
// represented as DOWN and are never exposed as response details.
func (manager *Manager) Report(ctx context.Context, group Group) (Report, error) {
	if manager == nil {
		return Report{}, errors.New("management report: manager is nil")
	}
	if ctx == nil {
		return Report{}, errors.New("management report: context is nil")
	}
	checks, found := manager.checks[group]
	if !found {
		return Report{}, fmt.Errorf("management report: unsupported group %q", group)
	}
	report := Report{
		Group:      group,
		Status:     StatusUp,
		Components: make([]Component, 0, len(checks)),
	}
	for _, check := range checks {
		status := StatusUp
		if ctx.Err() != nil || check.Probe(ctx) != nil {
			status = StatusDown
			report.Status = StatusDown
		}
		report.Components = append(report.Components, Component{
			Name:   check.Name,
			Module: check.Module,
			Status: status,
		})
	}
	return report, nil
}

// LifecycleChecks adapts one generated application's observable state into
// health, liveness, and readiness checks.
func LifecycleChecks(
	name string,
	module string,
	state func() lifecycle.State,
) ([]Check, error) {
	if state == nil {
		return nil, errors.New("management lifecycle checks: state function is nil")
	}
	live := func(context.Context) error {
		switch current := state(); current {
		case lifecycle.StateConstructed,
			lifecycle.StateStarting,
			lifecycle.StateReady,
			lifecycle.StateStopping:
			return nil
		case lifecycle.StateInvalid,
			lifecycle.StateStopped,
			lifecycle.StateFailed:
			return fmt.Errorf("application lifecycle state is %s", current)
		default:
			return fmt.Errorf("application lifecycle state %q is unsupported", current)
		}
	}
	ready := func(context.Context) error {
		if current := state(); current != lifecycle.StateReady {
			return fmt.Errorf("application lifecycle state is %s", current)
		}
		return nil
	}
	return []Check{
		{Name: name, Module: module, Groups: []Group{GroupHealth, GroupLiveness}, Probe: live},
		{Name: name, Module: module, Groups: []Group{GroupReadiness}, Probe: ready},
	}, nil
}

// HandlerOptions configures one isolated management HTTP handler.
type HandlerOptions struct {
	BasePath      string
	Manager       *Manager
	Info          map[string]string
	Metrics       *HTTPMetrics
	Configuration *ConfigurationReport
	Modules       *ModuleReport
	Expose        []Endpoint
	Access        Access
}

// Access controls the network-origin policy for management requests.
type Access string

const (
	// AccessPublic accepts management requests from any network origin.
	AccessPublic Access = "public"
	// AccessLoopback accepts only direct IPv4 or IPv6 loopback peers. Proxy
	// forwarding headers are intentionally ignored.
	AccessLoopback Access = "loopback"
)

// Endpoint identifies one explicitly exposed management HTTP endpoint.
type Endpoint string

const (
	// EndpointHealth exposes the aggregate health report.
	EndpointHealth Endpoint = "health"
	// EndpointLiveness exposes the process liveness report.
	EndpointLiveness Endpoint = "liveness"
	// EndpointReadiness exposes the traffic readiness report.
	EndpointReadiness Endpoint = "readiness"
	// EndpointInfo exposes caller-owned static application metadata.
	EndpointInfo Endpoint = "info"
	// EndpointMetrics exposes generated-route HTTP metrics.
	EndpointMetrics Endpoint = "metrics"
	// EndpointConfigProps exposes redacted generated configuration metadata.
	EndpointConfigProps Endpoint = "configprops"
	// EndpointModules exposes the generated application-module canvas.
	EndpointModules Endpoint = "modules"
)

// ConfigurationProperty is one safe resolved configuration entry. Secret
// values are always redacted.
type ConfigurationProperty struct {
	Key      string      `json:"key"`
	Kind     config.Kind `json:"kind"`
	Module   string      `json:"module,omitempty"`
	Value    string      `json:"value,omitempty"`
	Source   string      `json:"source,omitempty"`
	Resolved bool        `json:"resolved"`
	Default  bool        `json:"default,omitempty"`
	Secret   bool        `json:"secret,omitempty"`
}

// ConfigurationReport is a deterministic generated configuration view.
type ConfigurationReport struct {
	Properties []ConfigurationProperty `json:"properties"`
}

// NewConfigurationReport combines one generated schema and its resolved
// snapshot without exposing raw secret values.
func NewConfigurationReport(
	schema config.Schema,
	snapshot config.Snapshot,
) (ConfigurationReport, error) {
	properties := schema.Properties()
	known := make(map[string]struct{}, len(properties))
	for _, property := range properties {
		known[property.Key] = struct{}{}
	}
	for _, key := range snapshot.Keys() {
		if _, found := known[key]; !found {
			return ConfigurationReport{}, fmt.Errorf(
				"construct configuration report: snapshot key %q is absent from schema",
				key,
			)
		}
	}
	redacted := snapshot.Redacted()
	report := ConfigurationReport{
		Properties: make([]ConfigurationProperty, 0, len(properties)),
	}
	for _, property := range properties {
		item := ConfigurationProperty{
			Key:    property.Key,
			Kind:   property.Kind,
			Module: property.Module,
			Secret: property.Secret,
		}
		if entry, resolved := snapshot.Entry(property.Key); resolved {
			item.Value = redacted[property.Key]
			item.Source = entry.Origin.Source
			item.Resolved = true
			item.Default = entry.Origin.Default
		}
		report.Properties = append(report.Properties, item)
	}
	return report, nil
}

// Handler serves one isolated set of management endpoints.
type Handler struct {
	basePath      string
	manager       *Manager
	info          map[string]string
	metrics       *HTTPMetrics
	configuration *ConfigurationReport
	modules       *ModuleReport
	exposed       map[Endpoint]struct{}
	access        Access
	mux           *http.ServeMux
}

// NewHandler constructs exactly the explicitly exposed management endpoints.
func NewHandler(options HandlerOptions) (*Handler, error) {
	if options.Manager == nil {
		return nil, errors.New("construct management handler: manager is nil")
	}
	basePath := options.BasePath
	if basePath == "" {
		basePath = "/actuator"
	}
	if !validBasePath(basePath) {
		return nil, fmt.Errorf("construct management handler: base path %q must be a clean absolute path below root", basePath)
	}
	access := options.Access
	if access == "" {
		access = AccessLoopback
	}
	if access != AccessPublic && access != AccessLoopback {
		return nil, fmt.Errorf(
			"construct management handler: access %q is unsupported",
			access,
		)
	}
	exposed, err := exposedEndpoints(
		options.Expose,
		options.Metrics != nil,
		options.Configuration != nil,
		options.Modules != nil,
	)
	if err != nil {
		return nil, err
	}
	handler := &Handler{
		basePath:      basePath,
		manager:       options.Manager,
		info:          cloneInfo(options.Info),
		metrics:       options.Metrics,
		configuration: cloneConfigurationReport(options.Configuration),
		modules:       cloneModuleReport(options.Modules),
		exposed:       exposed,
		access:        access,
		mux:           http.NewServeMux(),
	}
	if handler.exposes(EndpointHealth) {
		handler.mux.HandleFunc("GET "+basePath+"/health", handler.serveReport(GroupHealth))
	}
	if handler.exposes(EndpointLiveness) {
		handler.mux.HandleFunc("GET "+basePath+"/health/liveness", handler.serveReport(GroupLiveness))
	}
	if handler.exposes(EndpointReadiness) {
		handler.mux.HandleFunc("GET "+basePath+"/health/readiness", handler.serveReport(GroupReadiness))
	}
	if handler.exposes(EndpointInfo) {
		handler.mux.HandleFunc("GET "+basePath+"/info", handler.serveInfo)
	}
	if handler.exposes(EndpointMetrics) {
		handler.mux.HandleFunc("GET "+basePath+"/metrics", handler.serveMetrics)
	}
	if handler.exposes(EndpointConfigProps) {
		handler.mux.HandleFunc(
			"GET "+basePath+"/configprops",
			handler.serveConfiguration,
		)
	}
	if handler.exposes(EndpointModules) {
		handler.mux.HandleFunc(
			"GET "+basePath+"/modules",
			handler.serveModules,
		)
	}
	return handler, nil
}

func exposedEndpoints(
	configured []Endpoint,
	metrics bool,
	configuration bool,
	modules bool,
) (map[Endpoint]struct{}, error) {
	if configured == nil {
		configured = []Endpoint{
			EndpointHealth,
			EndpointLiveness,
			EndpointReadiness,
			EndpointInfo,
		}
		if metrics {
			configured = append(configured, EndpointMetrics)
		}
	}
	if len(configured) == 0 {
		return nil, errors.New("construct management handler: at least one endpoint must be exposed")
	}
	result := make(map[Endpoint]struct{}, len(configured))
	for index, endpoint := range configured {
		switch endpoint {
		case EndpointHealth, EndpointLiveness, EndpointReadiness, EndpointInfo:
		case EndpointMetrics:
			if !metrics {
				return nil, errors.New("construct management handler: metrics endpoint requires an HTTP metrics collector")
			}
		case EndpointConfigProps:
			if !configuration {
				return nil, errors.New("construct management handler: configprops endpoint requires a configuration report")
			}
		case EndpointModules:
			if !modules {
				return nil, errors.New("construct management handler: modules endpoint requires a module report")
			}
		default:
			return nil, fmt.Errorf(
				"construct management handler: endpoint %d %q is unsupported",
				index,
				endpoint,
			)
		}
		if _, duplicate := result[endpoint]; duplicate {
			return nil, fmt.Errorf(
				"construct management handler: endpoint %q is exposed more than once",
				endpoint,
			)
		}
		result[endpoint] = struct{}{}
	}
	return result, nil
}

func (handler *Handler) exposes(endpoint Endpoint) bool {
	if handler == nil {
		return false
	}
	_, exposed := handler.exposed[endpoint]
	return exposed
}

// Pattern returns the GET-only ServeMux subtree pattern used to mount this
// handler. Restricting the method prevents a management subtree from
// conflicting with an application's ordinary GET root route.
func (handler *Handler) Pattern() string {
	if handler == nil {
		return ""
	}
	return http.MethodGet + " " + handler.basePath + "/"
}

// ServeHTTP dispatches management requests without exposing other routes.
func (handler *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.mux == nil {
		http.Error(writer, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	if handler.access == AccessLoopback && !loopbackPeer(request.RemoteAddr) {
		if err := web.WriteProblem(writer, web.Problem{
			Type:   "urn:spice:management:loopback-required",
			Title:  http.StatusText(http.StatusForbidden),
			Status: http.StatusForbidden,
			Detail: "Management access is restricted to the loopback interface.",
		}); err != nil {
			return
		}
		return
	}
	handler.mux.ServeHTTP(writer, request)
}

func loopbackPeer(remoteAddress string) bool {
	host, _, err := net.SplitHostPort(remoteAddress)
	if err != nil {
		host = remoteAddress
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (handler *Handler) serveReport(group Group) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		report, err := handler.manager.Report(request.Context(), group)
		if err != nil {
			if writeErr := web.WriteError(writer, request, err, nil); writeErr != nil {
				return
			}
			return
		}
		status := http.StatusOK
		if report.Status == StatusDown {
			status = http.StatusServiceUnavailable
		}
		if writeErr := web.WriteJSON(writer, status, report); writeErr != nil {
			return
		}
	}
}

func (handler *Handler) serveInfo(writer http.ResponseWriter, _ *http.Request) {
	if writeErr := web.WriteJSON(writer, http.StatusOK, handler.info); writeErr != nil {
		return
	}
}

func (handler *Handler) serveMetrics(writer http.ResponseWriter, _ *http.Request) {
	if writeErr := web.WriteJSON(writer, http.StatusOK, handler.metrics.Snapshot()); writeErr != nil {
		return
	}
}

func (handler *Handler) serveConfiguration(
	writer http.ResponseWriter,
	_ *http.Request,
) {
	if writeErr := web.WriteJSON(
		writer,
		http.StatusOK,
		handler.configuration,
	); writeErr != nil {
		return
	}
}

func (handler *Handler) serveModules(
	writer http.ResponseWriter,
	_ *http.Request,
) {
	if writeErr := web.WriteJSON(
		writer,
		http.StatusOK,
		handler.modules,
	); writeErr != nil {
		return
	}
}

func cloneConfigurationReport(
	report *ConfigurationReport,
) *ConfigurationReport {
	if report == nil {
		return nil
	}
	return &ConfigurationReport{
		Properties: append(
			[]ConfigurationProperty(nil),
			report.Properties...,
		),
	}
}

func cloneModuleReport(report *ModuleReport) *ModuleReport {
	if report == nil {
		return nil
	}
	result := &ModuleReport{
		Schema:             report.Schema,
		Modules:            make([]ApplicationModule, len(report.Modules)),
		Edges:              append([]ModuleEdge(nil), report.Edges...),
		UnassignedPackages: append([]string(nil), report.UnassignedPackages...),
	}
	for index, module := range report.Modules {
		result.Modules[index] = module
		result.Modules[index].Packages = append([]string(nil), module.Packages...)
		result.Modules[index].NamedInterfaces = append(
			[]NamedInterface(nil),
			module.NamedInterfaces...,
		)
		result.Modules[index].AllowedDependencies = append(
			[]string(nil),
			module.AllowedDependencies...,
		)
		result.Modules[index].ObservedDependencies = append(
			[]string(nil),
			module.ObservedDependencies...,
		)
	}
	return result
}

func validName(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func validGroup(group Group) bool {
	return group == GroupHealth || group == GroupLiveness || group == GroupReadiness
}

func validBasePath(value string) bool {
	return strings.HasPrefix(value, "/") &&
		value != "/" &&
		path.Clean(value) == value &&
		!strings.ContainsAny(value, "{}* \t\r\n")
}

func cloneInfo(info map[string]string) map[string]string {
	result := maps.Clone(info)
	if result == nil {
		return map[string]string{}
	}
	return result
}
