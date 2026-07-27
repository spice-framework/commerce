// @import { Bean, Configuration } from "github.com/StevenBuglione/spice/annotation/core"
// @import { Module } from "github.com/StevenBuglione/spice/annotation/modulith"

// Package orders owns order placement.
//
// @Module(allowedDependencies=["github.com/StevenBuglione/spice/examples/commerce/inventory", "github.com/StevenBuglione/spice/examples/commerce/payments"])
package orders

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/StevenBuglione/spice/event"
	"github.com/StevenBuglione/spice/examples/commerce/inventory"
	"github.com/StevenBuglione/spice/examples/commerce/payments"
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
	payments  *payments.Service
	views     event.Publisher[OrderViewed]
	orders    []Order
	nextID    int
}

// NewService constructs the order service with explicit module dependencies.
//
// @Bean
func NewService(
	settings Settings,
	inventoryService *inventory.Service,
	paymentService *payments.Service,
	viewPublisher event.Publisher[OrderViewed],
) (*Service, error) {
	if strings.TrimSpace(settings.SKU) == "" || settings.UnitPriceCents <= 0 {
		return nil, fmt.Errorf(
			"%w: configured SKU must be set and unit price must be positive",
			ErrInvalidOrder,
		)
	}
	if inventoryService == nil || paymentService == nil {
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
	}, nil
}

// Place reserves stock, authorizes payment, and records a completed order.
func (service *Service) Place(ctx context.Context, request Request) (Order, error) {
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
	service.mu.Lock()
	service.orders = append(service.orders, order)
	service.mu.Unlock()
	return order, nil
}

func (service *Service) allocateID() string {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.nextID++
	return fmt.Sprintf("order-%06d", service.nextID)
}

// Orders returns a stable copy of completed orders.
func (service *Service) Orders() []Order {
	service.mu.RLock()
	defer service.mu.RUnlock()
	return append([]Order(nil), service.orders...)
}

// Find returns one completed order and publishes a typed view event. Missing
// orders do not publish.
func (service *Service) Find(
	ctx context.Context,
	id string,
) (Order, bool, error) {
	order, found := service.Order(id)
	if !found {
		return Order{}, false, nil
	}
	if err := service.views.Publish(ctx, OrderViewed{ID: order.ID}); err != nil {
		return Order{}, false, fmt.Errorf("observe order %s view: %w", order.ID, err)
	}
	return order, true, nil
}

// Order returns one completed order by its stable ID.
func (service *Service) Order(id string) (Order, bool) {
	service.mu.RLock()
	defer service.mu.RUnlock()
	for _, order := range service.orders {
		if order.ID == id {
			return order, true
		}
	}
	return Order{}, false
}
