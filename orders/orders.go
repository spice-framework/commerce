// @import { Bean, Configuration, Qualifier } from "github.com/StevenBuglione/spice/annotation/core"
// @import { Module } from "github.com/StevenBuglione/spice/annotation/modulith"

// Package orders owns order placement.
//
// @Module(allowedDependencies=["github.com/StevenBuglione/spice/examples/commerce/inventory", "github.com/StevenBuglione/spice/examples/commerce/payments", "github.com/StevenBuglione/spice/examples/commerce/storage"])
package orders

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/StevenBuglione/spice/data"
	"github.com/StevenBuglione/spice/event"
	"github.com/StevenBuglione/spice/examples/commerce/inventory"
	"github.com/StevenBuglione/spice/examples/commerce/payments"
	"github.com/StevenBuglione/spice/examples/commerce/storage"
)

var (
	// ErrInvalidOrder reports malformed order input or configuration.
	ErrInvalidOrder = errors.New("invalid order")
	// ErrOrderRejected reports a dependency rejection after compensation.
	ErrOrderRejected = errors.New("order rejected")
)

// Settings configures the reference order catalog.
//
// @Configuration(prefix="commerce.orders")
type Settings struct {
	SKU            string `spice:"sku,default=SKU-RED"`
	UnitPriceCents int    `spice:"unit-price-cents,default=2500"`
}

// Request describes one order placement.
type Request struct {
	Quantity int
}

// Order is an immutable completed order.
type Order struct {
	ID              string
	SKU             string
	Quantity        int
	TotalCents      int
	AuthorizationID string
}

// Service coordinates inventory and payment as one compensated operation.
type Service struct {
	mu        sync.RWMutex
	settings  Settings
	inventory *inventory.Service
	payments  payments.Processor
	views     event.Publisher[OrderViewed]
	orders    storage.Orders
	reads     data.Executor
	nextID    int
}

// NewService constructs the order service with explicit module dependencies.
//
// @Bean
func NewService(
	settings Settings,
	inventoryService *inventory.Service,
	// @Qualifier("stripe")
	paymentService payments.Processor,
	viewPublisher event.Publisher[OrderViewed],
	orderRepository storage.Orders,
	readExecutor data.Executor,
) (*Service, error) {
	if strings.TrimSpace(settings.SKU) == "" || settings.UnitPriceCents <= 0 {
		return nil, fmt.Errorf(
			"%w: configured SKU must be set and unit price must be positive",
			ErrInvalidOrder,
		)
	}
	if inventoryService == nil ||
		paymentService == nil ||
		orderRepository == nil ||
		readExecutor == nil {
		return nil, fmt.Errorf("%w: module dependencies must not be nil", ErrInvalidOrder)
	}
	if viewPublisher == nil {
		return nil, fmt.Errorf("%w: order view publisher must not be nil", ErrInvalidOrder)
	}
	return &Service{
		settings:  settings,
		inventory: inventoryService,
		payments:  paymentService,
		views:     viewPublisher,
		orders:    orderRepository,
		reads:     readExecutor,
	}, nil
}

// Place reserves stock, authorizes payment, and records a completed order.
func (service *Service) Place(ctx context.Context, request Request) (Order, error) {
	return service.place(ctx, service.reads, request)
}

// PlaceWithin performs order placement through a caller-owned transaction
// executor.
func (service *Service) PlaceWithin(
	ctx context.Context,
	executor data.Executor,
	request Request,
) (Order, error) {
	return service.place(ctx, executor, request)
}

func (service *Service) place(
	ctx context.Context,
	executor data.Executor,
	request Request,
) (Order, error) {
	if request.Quantity <= 0 {
		return Order{}, fmt.Errorf("%w: quantity must be positive", ErrInvalidOrder)
	}
	if err := ctx.Err(); err != nil {
		return Order{}, fmt.Errorf("place order: %w", err)
	}

	orderID := service.allocateID()
	if err := service.inventory.Reserve(ctx, service.settings.SKU, request.Quantity); err != nil {
		return Order{}, fmt.Errorf("%w: reserve stock for %s: %w", ErrOrderRejected, orderID, err)
	}
	total := request.Quantity * service.settings.UnitPriceCents
	authorization, err := service.payments.Authorize(ctx, orderID, total)
	if err != nil {
		if releaseErr := service.inventory.Release(service.settings.SKU, request.Quantity); releaseErr != nil {
			return Order{}, errors.Join(
				fmt.Errorf("%w: authorize payment for %s: %w", ErrOrderRejected, orderID, err),
				fmt.Errorf("restore stock for %s: %w", orderID, releaseErr),
			)
		}
		return Order{}, fmt.Errorf(
			"%w: authorize payment for %s: %w",
			ErrOrderRejected,
			orderID,
			err,
		)
	}

	order := Order{
		ID:              orderID,
		SKU:             service.settings.SKU,
		Quantity:        request.Quantity,
		TotalCents:      total,
		AuthorizationID: authorization.ID,
	}
	if err := service.orders.Save(ctx, executor, storage.Record(order)); err != nil {
		if releaseErr := service.inventory.Release(
			service.settings.SKU,
			request.Quantity,
		); releaseErr != nil {
			return Order{}, errors.Join(
				fmt.Errorf("%w: persist %s: %w", ErrOrderRejected, orderID, err),
				fmt.Errorf("restore stock for %s: %w", orderID, releaseErr),
			)
		}
		return Order{}, fmt.Errorf(
			"%w: persist %s: %w",
			ErrOrderRejected,
			orderID,
			err,
		)
	}
	return order, nil
}

func (service *Service) allocateID() string {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.nextID++
	return fmt.Sprintf("order-%06d", service.nextID)
}

// Find returns one completed order and publishes a typed view event. Missing
// orders do not publish.
func (service *Service) Find(
	ctx context.Context,
	id string,
) (Order, bool, error) {
	record, found, err := service.orders.Find(ctx, service.reads, id)
	if err != nil {
		return Order{}, false, err
	}
	if !found {
		return Order{}, false, nil
	}
	order := Order(record)
	if err := service.views.Publish(ctx, OrderViewed{ID: order.ID}); err != nil {
		return Order{}, false, fmt.Errorf("observe order %s view: %w", order.ID, err)
	}
	return order, true, nil
}
