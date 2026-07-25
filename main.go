package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/StevenBuglione/spice/config"
	commerce "github.com/StevenBuglione/spice/internal/spicegen/commerce"
	"github.com/StevenBuglione/spice/lifecycle"
	"github.com/StevenBuglione/spice/management"
	"github.com/StevenBuglione/spice/observability"
	"github.com/StevenBuglione/spice/web"
)

const shutdownTimeout = 10 * time.Second

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(os.Args[1:], os.Stdout, logger); err != nil {
		logger.Error("Spice commerce example failed", slog.Any("error", err))
		os.Exit(1) // Entrypoint exception: return a non-zero status when the server cannot run.
	}
}

func run(arguments []string, stdout io.Writer, logger *slog.Logger) error {
	if logger == nil {
		return errors.New("run commerce: logger is nil")
	}
	flags := flag.NewFlagSet("commerce", flag.ContinueOnError)
	flags.SetOutput(stdout)
	check := flags.Bool("check", false, "construct the generated application and exit")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	environment, err := config.OSEnvironment("SPICE_")
	if err != nil {
		return fmt.Errorf("configure environment source: %w", err)
	}
	httpLogs, err := observability.NewSlogHTTPObserver(logger)
	if err != nil {
		return fmt.Errorf("configure HTTP logging: %w", err)
	}
	lifecycleLogs, err := observability.NewSlogLifecycleObserver(logger)
	if err != nil {
		return fmt.Errorf("configure lifecycle logging: %w", err)
	}
	metrics := management.NewHTTPMetrics()
	application, err := commerce.NewApplicationWithOptions(
		context.Background(),
		commerce.ApplicationOptions{
			Sources:       []config.Source{environment},
			HTTPObservers: []web.HTTPObserver{metrics, httpLogs},
			Observers:     []lifecycle.Observer{lifecycleLogs},
		},
	)
	if err != nil {
		return fmt.Errorf("construct application: %w", err)
	}
	if err := configureManagement(application, metrics); err != nil {
		return err
	}
	if *check {
		if err := application.Stop(context.Background()); err != nil {
			return fmt.Errorf("release application: %w", err)
		}
		if _, err := fmt.Fprintln(
			stdout,
			"Spice commerce ready: inventory -> orders -> payments + platform",
		); err != nil {
			return fmt.Errorf("write readiness message: %w", err)
		}
		return nil
	}

	runContext, stopSignals := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stopSignals()
	return application.Run(runContext, func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), shutdownTimeout)
	})
}

func configureManagement(
	application *commerce.Application,
	metrics *management.HTTPMetrics,
) error {
	checks, err := management.LifecycleChecks(
		"commerce",
		"github.com/StevenBuglione/spice/examples/commerce/platform",
		application.State,
	)
	if err != nil {
		return fmt.Errorf("configure lifecycle checks: %w", err)
	}
	manager, err := management.New(checks...)
	if err != nil {
		return fmt.Errorf("configure management checks: %w", err)
	}
	handler, err := management.NewHandler(management.HandlerOptions{
		Manager: manager,
		Metrics: metrics,
		Info: map[string]string{
			"application": "commerce",
			"framework":   "Spice",
		},
	})
	if err != nil {
		return fmt.Errorf("configure management handler: %w", err)
	}
	mux, ok := application.Handler().(*http.ServeMux)
	if !ok {
		return fmt.Errorf(
			"configure management handler: generated handler is %T, want *http.ServeMux",
			application.Handler(),
		)
	}
	if err := web.Register(mux, handler.Pattern(), handler); err != nil {
		return fmt.Errorf("register management handler: %w", err)
	}
	return nil
}
