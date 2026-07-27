package payments

import (
	"context"
	"errors"
	"testing"
)

func TestServiceAuthorizesWithinPolicy(t *testing.T) {
	t.Parallel()
	service, err := NewService(Settings{MaximumCents: 5000})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	first, err := service.Authorize(context.Background(), "order-1", 2500)
	if err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	second, err := service.Authorize(context.Background(), "order-2", 1000)
	if err != nil {
		t.Fatalf("Authorize() second error = %v", err)
	}
	if first.ID != "payment-000001" || second.ID != "payment-000002" {
		t.Fatalf("authorization IDs = %q, %q", first.ID, second.ID)
	}
	if got := len(service.Approved()); got != 2 {
		t.Fatalf("len(Approved()) = %d, want 2", got)
	}
}

func TestServiceDeclinesAbovePolicyWithoutRecording(t *testing.T) {
	t.Parallel()
	service, err := NewService(Settings{MaximumCents: 100})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if _, err := service.Authorize(context.Background(), "order-1", 101); !errors.Is(err, ErrDeclined) {
		t.Fatalf("Authorize() error = %v, want ErrDeclined", err)
	}
	if got := len(service.Approved()); got != 0 {
		t.Fatalf("len(Approved()) = %d, want 0", got)
	}
}

func TestServiceHonorsCancellation(t *testing.T) {
	t.Parallel()
	service, err := NewService(Settings{MaximumCents: 100})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := service.Authorize(ctx, "order-1", 50); !errors.Is(err, context.Canceled) {
		t.Fatalf("Authorize() error = %v, want context.Canceled", err)
	}
	if got := len(service.Approved()); got != 0 {
		t.Fatalf("len(Approved()) = %d, want 0", got)
	}
}

func TestOfflineProcessorFailsClosedAndHonorsCancellation(t *testing.T) {
	t.Parallel()
	service := &OfflineProcessor{}
	if _, err := service.Authorize(
		context.Background(),
		"order-1",
		50,
	); !errors.Is(err, ErrDeclined) {
		t.Fatalf("Authorize() error = %v, want ErrDeclined", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Authorize(
		ctx,
		"order-1",
		50,
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("Authorize() error = %v, want context.Canceled", err)
	}
}
