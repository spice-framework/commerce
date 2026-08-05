package orders

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/spice-framework/spice/event"
	"github.com/spice-framework/spice/examples/commerce/inventory"
	"github.com/spice-framework/spice/examples/commerce/notifications"
	"github.com/spice-framework/spice/examples/commerce/payments"
	"github.com/spice-framework/spice/examples/commerce/storage"
	"github.com/spice-framework/spice/web"
)

func TestServicePlacesOrderAcrossModules(t *testing.T) {
	t.Parallel()
	service, stock, payment := newTestService(t, 3, 5000)

	order, err := service.Place(context.Background(), Request{Quantity: 2})
	if err != nil {
		t.Fatalf("Place() error = %v", err)
	}
	if order.ID != "order-000001" || order.AuthorizationID != "payment-000001" {
		t.Fatalf("Place() order = %#v", order)
	}
	if order.TotalCents != 5000 || stock.Available("widget") != 1 {
		t.Fatalf("Place() total=%d stock=%d", order.TotalCents, stock.Available("widget"))
	}
	if got := len(payment.Approved()); got != 1 {
		t.Fatalf("len(Approved()) = %d, want 1", got)
	}
}

func TestServiceRestoresStockWhenPaymentIsDeclined(t *testing.T) {
	t.Parallel()
	service, stock, payment := newTestService(t, 2, 1000)

	_, err := service.Place(context.Background(), Request{Quantity: 1})
	if !errors.Is(err, ErrOrderRejected) || !errors.Is(err, payments.ErrDeclined) {
		t.Fatalf("Place() error = %v, want rejected declined order", err)
	}
	if got := stock.Available("widget"); got != 2 {
		t.Fatalf("Available() = %d, want compensated stock 2", got)
	}
	if got := len(payment.Approved()); got != 0 {
		t.Fatalf("len(Approved()) = %d, want 0", got)
	}
	if _, found, findErr := service.Find(
		context.Background(),
		"order-000001",
	); findErr != nil || found {
		t.Fatalf("Find(rejected) = found=%t err=%v", found, findErr)
	}
}

func TestServiceRejectsCanceledOrderWithoutSideEffects(t *testing.T) {
	t.Parallel()
	service, stock, payment := newTestService(t, 2, 5000)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := service.Place(ctx, Request{Quantity: 1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Place() error = %v, want context.Canceled", err)
	}
	if got := stock.Available("widget"); got != 2 {
		t.Fatalf("Available() = %d, want unchanged stock 2", got)
	}
	if got := len(payment.Approved()); got != 0 {
		t.Fatalf("len(Approved()) = %d, want 0", got)
	}
}

func TestControllerReturnsSafePaymentProblem(t *testing.T) {
	t.Parallel()
	service, _, _ := newTestService(t, 2, 1000)
	controller := newTestController(t, service)

	_, err := controller.Place(
		context.Background(),
		service.reads,
		PlaceOrderRequest{Body: PlaceOrderBody{Quantity: 1}},
	)
	var carrier web.ProblemCarrier
	if !errors.As(err, &carrier) {
		t.Fatalf("Place() error = %v, want ProblemCarrier", err)
	}
	problem := carrier.Problem()
	if problem.Status != http.StatusConflict ||
		problem.Type != "https://spice.dev/problems/payment-declined" ||
		problem.Detail != "" {
		t.Fatalf("Place() problem = %#v", problem)
	}
}

func TestPlaceOrderRequestValidation(t *testing.T) {
	t.Parallel()
	err := (PlaceOrderRequest{}).Validate(context.Background())
	var carrier web.ProblemCarrier
	if !errors.As(err, &carrier) || carrier.Problem().Status != http.StatusBadRequest {
		t.Fatalf("Validate() error = %v, want safe bad-request problem", err)
	}
}

func TestControllerGetsCompletedOrder(t *testing.T) {
	t.Parallel()
	service, _, _ := newTestService(t, 2, 5000)
	controller := newTestController(t, service)
	placed, err := controller.Place(
		context.Background(),
		service.reads,
		PlaceOrderRequest{Body: PlaceOrderBody{Quantity: 1}},
	)
	if err != nil {
		t.Fatalf("Place() error = %v", err)
	}
	got, err := controller.Get(context.Background(), GetOrderRequest{ID: placed.ID})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != placed {
		t.Fatalf("Get() = %#v, want %#v", got, placed)
	}
	if _, err := controller.Get(context.Background(), GetOrderRequest{ID: "missing"}); err == nil {
		t.Fatal("Get(missing) error = nil")
	}
}

func TestControllerSendsReceiptOnlyForPersistedOrder(t *testing.T) {
	t.Parallel()
	service, _, _ := newTestService(t, 2, 5000)
	controller, delivery := newTestControllerWithDelivery(t, service)
	placed, err := controller.Place(
		context.Background(),
		service.reads,
		PlaceOrderRequest{Body: PlaceOrderBody{Quantity: 1}},
	)
	if err != nil {
		t.Fatalf("Place() error = %v", err)
	}
	response, err := controller.SendReceipt(
		context.Background(),
		ReceiptRequest{ID: placed.ID},
	)
	if err != nil {
		t.Fatalf("SendReceipt() error = %v", err)
	}
	if response.MessageID != "receipt-"+placed.ID+"@commerce.example" ||
		len(delivery.Messages()) != 1 {
		t.Fatalf(
			"SendReceipt() response=%#v messages=%#v",
			response,
			delivery.Messages(),
		)
	}
	if _, err := controller.SendReceipt(
		context.Background(),
		ReceiptRequest{ID: "missing"},
	); err == nil {
		t.Fatal("SendReceipt(missing) error = nil")
	}
}

func TestControllerRejectsNilDependencies(t *testing.T) {
	t.Parallel()
	if _, err := NewController(nil, nil); err == nil {
		t.Fatal("NewController(nil) error = nil")
	}
}

func TestServicePublishesOnlySuccessfulOrderViews(t *testing.T) {
	t.Parallel()
	audit := NewViewAudit()
	publisher, err := event.NewTopic(
		event.Definition{
			ID:     "orders.OrderViewed",
			Module: "example.com/commerce/orders",
		},
		[]event.Subscriber[OrderViewed]{
			{
				ID:     "orders.ViewAudit.Record",
				Module: "example.com/commerce/orders",
				Order:  10,
				Handle: audit.Record,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	service, _, _ := newTestServiceWithPublisher(t, 2, 5000, publisher)
	placed, err := service.Place(context.Background(), Request{Quantity: 1})
	if err != nil {
		t.Fatalf("Place() error = %v", err)
	}
	if _, found, findErr := service.Find(
		context.Background(),
		placed.ID,
	); findErr != nil || !found {
		t.Fatalf("Find() = (found=%t, err=%v)", found, findErr)
	}
	if _, found, findErr := service.Find(
		context.Background(),
		"missing",
	); findErr != nil || found {
		t.Fatalf("Find(missing) = (found=%t, err=%v)", found, findErr)
	}
	if got := audit.Views(placed.ID); got != 1 {
		t.Fatalf("Views() = %d, want 1", got)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, found, findErr := service.Find(
		cancelled,
		placed.ID,
	); !errors.Is(findErr, context.Canceled) || found {
		t.Fatalf(
			"Find(cancelled) = (found=%t, err=%v)",
			found,
			findErr,
		)
	}
	if got := audit.Views(placed.ID); got != 1 {
		t.Fatalf("cancelled Find changed Views() to %d", got)
	}
}

func newTestController(t *testing.T, service *Service) *Controller {
	t.Helper()
	controller, _ := newTestControllerWithDelivery(t, service)
	return controller
}

func newTestControllerWithDelivery(
	t *testing.T,
	service *Service,
) (*Controller, *notifications.Delivery) {
	t.Helper()
	settings := notifications.Settings{
		Transport:    "test",
		From:         "Spice Commerce <no-reply@commerce.example>",
		Recipient:    "Developer <developer@commerce.example>",
		TestCapacity: 10,
	}
	delivery, err := notifications.NewDelivery(settings)
	if err != nil {
		t.Fatalf("notifications.NewDelivery() error = %v", err)
	}
	notifier, err := notifications.NewNotifier(
		settings,
		delivery,
		delivery,
		notifications.NewSystemClock(),
	)
	if err != nil {
		t.Fatalf("notifications.NewNotifier() error = %v", err)
	}
	controller, err := NewController(service, notifier)
	if err != nil {
		t.Fatalf("NewController() error = %v", err)
	}
	return controller, delivery
}

func newTestService(
	t *testing.T,
	stockCount int,
	maximumCents int,
) (*Service, *inventory.Service, *payments.Service) {
	t.Helper()
	audit := NewViewAudit()
	publisher, err := event.NewTopic(
		event.Definition{
			ID:     "orders.OrderViewed",
			Module: "example.com/commerce/orders",
		},
		[]event.Subscriber[OrderViewed]{
			{
				ID:     "orders.ViewAudit.Record",
				Module: "example.com/commerce/orders",
				Order:  10,
				Handle: audit.Record,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return newTestServiceWithPublisher(
		t,
		stockCount,
		maximumCents,
		publisher,
	)
}

func newTestServiceWithPublisher(
	t *testing.T,
	stockCount int,
	maximumCents int,
	publisher event.Publisher[OrderViewed],
) (*Service, *inventory.Service, *payments.Service) {
	t.Helper()
	stock, err := inventory.NewService(inventory.Settings{SKU: "widget", InitialStock: stockCount})
	if err != nil {
		t.Fatalf("inventory.NewService() error = %v", err)
	}
	payment, err := payments.NewService(payments.Settings{MaximumCents: maximumCents})
	if err != nil {
		t.Fatalf("payments.NewService() error = %v", err)
	}
	database, cleanup, err := storage.OpenDatabase(storage.Settings{
		URL: "memory://commerce",
	})
	if err != nil {
		t.Fatalf("storage.OpenDatabase() error = %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := cleanup(context.Background()); cleanupErr != nil {
			t.Errorf("storage cleanup error = %v", cleanupErr)
		}
	})
	if migrationErr := database.Migrate(context.Background()); migrationErr != nil {
		t.Fatalf("Database.Migrate() error = %v", migrationErr)
	}
	native, err := storage.Native(database)
	if err != nil {
		t.Fatalf("storage.Native() error = %v", err)
	}
	orderRepository, err := storage.NewOrderRepository()
	if err != nil {
		t.Fatalf("storage.NewOrderRepository() error = %v", err)
	}
	service, err := NewService(
		Settings{SKU: "widget", UnitPriceCents: 2500},
		stock,
		payment,
		publisher,
		orderRepository,
		native,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service, stock, payment
}
