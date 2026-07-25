package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/StevenBuglione/spice/config"
	commerce "github.com/StevenBuglione/spice/internal/spicegen/commerce"
)

const shutdownTimeout = 10 * time.Second

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		log.Printf("Spice commerce example failed: %v", err)
		os.Exit(1) // Entrypoint exception: return a non-zero status when the server cannot run.
	}
}

func run(arguments []string, stdout io.Writer) error {
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
	application, err := commerce.NewApplicationWithOptions(
		context.Background(),
		commerce.ApplicationOptions{Sources: []config.Source{environment}},
	)
	if err != nil {
		return fmt.Errorf("construct application: %w", err)
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
