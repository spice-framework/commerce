package inventory

import (
	"context"
	"errors"
	"testing"
)

func TestServiceReservationLifecycle(t *testing.T) {
	t.Parallel()
	service, err := NewService(Settings{SKU: "widget", InitialStock: 3})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if err := service.Reserve(context.Background(), "widget", 2); err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}
	if got := service.Available("widget"); got != 1 {
		t.Fatalf("Available() = %d, want 1", got)
	}
	if err := service.Release("widget", 2); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	if got := service.Available("widget"); got != 3 {
		t.Fatalf("Available() after release = %d, want 3", got)
	}
}

func TestServiceRejectsReservationWithoutMutation(t *testing.T) {
	t.Parallel()
	service, err := NewService(Settings{SKU: "widget", InitialStock: 1})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if err := service.Reserve(context.Background(), "widget", 2); !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("Reserve() error = %v, want ErrInsufficientStock", err)
	}
	if got := service.Available("widget"); got != 1 {
		t.Fatalf("Available() = %d, want unchanged stock 1", got)
	}
}

func TestServiceHonorsCancellation(t *testing.T) {
	t.Parallel()
	service, err := NewService(Settings{SKU: "widget", InitialStock: 1})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := service.Reserve(ctx, "widget", 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("Reserve() error = %v, want context.Canceled", err)
	}
	if got := service.Available("widget"); got != 1 {
		t.Fatalf("Available() = %d, want unchanged stock 1", got)
	}
}

func TestServiceAuditsInventoryInvariants(t *testing.T) {
	t.Parallel()
	service, err := NewService(Settings{SKU: "widget", InitialStock: 1})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if err := service.Audit(context.Background()); err != nil {
		t.Fatalf("Audit() error = %v", err)
	}
	if err := service.VerifySKU(
		context.Background(),
		"widget",
	); err != nil {
		t.Fatalf("VerifySKU() error = %v", err)
	}
	if err := service.VerifySKU(
		context.Background(),
		"missing",
	); !errors.Is(err, ErrInvalidReservation) {
		t.Fatalf(
			"VerifySKU(missing) error = %v, want ErrInvalidReservation",
			err,
		)
	}

	service.stock["widget"] = -1
	if err := service.Audit(context.Background()); !errors.Is(
		err,
		ErrInvalidReservation,
	) {
		t.Fatalf(
			"Audit(invalid) error = %v, want ErrInvalidReservation",
			err,
		)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.Audit(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Audit(canceled) error = %v, want context.Canceled", err)
	}
	if err := service.VerifySKU(
		ctx,
		"widget",
	); !errors.Is(err, context.Canceled) {
		t.Fatalf(
			"VerifySKU(canceled) error = %v, want context.Canceled",
			err,
		)
	}
}
