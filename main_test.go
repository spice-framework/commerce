package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/StevenBuglione/spice/config"
	commerce "github.com/StevenBuglione/spice/internal/spicegen/commerce"
	"github.com/StevenBuglione/spice/lifecycle"
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

func TestRunRejectsUnknownFlag(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	if err := run([]string{"-unknown"}, &stdout); err == nil {
		t.Fatal("run() error = nil")
	}
}
