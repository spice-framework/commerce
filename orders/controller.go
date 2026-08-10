package orders

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/spice-framework/commerce/inventory"
	"github.com/spice-framework/commerce/notifications"
	"github.com/spice-framework/commerce/payments"
	"github.com/spice-framework/spice/data"
	"github.com/spice-framework/spice/web"
)

// @import { Cacheable } from "github.com/spice-framework/spice/annotation/cache"
// @import { Transactional } from "github.com/spice-framework/spice/annotation/data"
// @import { Authorize } from "github.com/spice-framework/spice/annotation/security"
// @import { Controller, Get, Post } from "github.com/spice-framework/spice/annotation/web"

// Controller exposes typed order operations.
//
// @Controller
type Controller struct {
	service  *Service
	notifier *notifications.Notifier
}

// NewController constructs the order HTTP boundary.
func NewController(
	service *Service,
	notifier *notifications.Notifier,
) (*Controller, error) {
	if service == nil || notifier == nil {
		return nil, errors.New("construct order controller: dependencies are nil")
	}
	return &Controller{service: service, notifier: notifier}, nil
}

// Place creates one order through generated strict JSON binding.
//
// @Post("/orders")
// @Transactional(isolation="serializable")
// @Authorize(allScopes=["orders:write"])
func (controller *Controller) Place(
	ctx context.Context,
	executor data.Executor,
	request PlaceOrderRequest,
) (OrderResponse, error) {
	order, err := controller.service.PlaceWithin(
		ctx,
		executor,
		Request{Quantity: request.Body.Quantity},
	)
	if err != nil {
		return OrderResponse{}, controller.publicOrderError(err)
	}
	return controller.orderResponse(order), nil
}

// Get returns one completed order.
//
// @Get("/orders/{id}")
// @Authorize(allScopes=["orders:read"], expression="authenticated && hasScope(\"orders:read\")")
func (controller *Controller) Get(
	ctx context.Context,
	request GetOrderRequest,
) (OrderResponse, error) {
	order, found, err := controller.service.Find(
		ctx,
		strings.TrimSpace(request.ID),
	)
	if err != nil {
		return OrderResponse{}, err
	}
	if !found {
		return OrderResponse{}, controller.orderNotFound()
	}
	return controller.orderResponse(order), nil
}

// Catalog returns public immutable product metadata. This route remains safe
// to cache because it contains no principal- or order-specific data.
//
// @Get("/catalog")
// @Cacheable(name="commerce.catalog")
func (controller *Controller) Catalog(
	ctx context.Context,
	_ CatalogRequest,
) (CatalogResponse, error) {
	if err := ctx.Err(); err != nil {
		return CatalogResponse{}, err
	}
	sku, unitPriceCents := controller.service.Catalog()
	return CatalogResponse{
		SKU:            sku,
		UnitPriceCents: unitPriceCents,
	}, nil
}

// SendReceipt delivers an inspectable receipt for one already committed order.
// It is deliberately separate from Place so external I/O never runs inside the
// database transaction.
//
// @Post("/orders/{id}/receipt")
// @Authorize(allScopes=["orders:notify"])
func (controller *Controller) SendReceipt(
	ctx context.Context,
	request ReceiptRequest,
) (ReceiptResponse, error) {
	order, found, err := controller.service.Find(
		ctx,
		strings.TrimSpace(request.ID),
	)
	if err != nil {
		return ReceiptResponse{}, err
	}
	if !found {
		return ReceiptResponse{}, controller.orderNotFound()
	}
	result, err := controller.notifier.SendReceipt(
		ctx,
		notifications.Receipt{
			OrderID:         order.ID,
			SKU:             order.SKU,
			Quantity:        order.Quantity,
			TotalCents:      order.TotalCents,
			AuthorizationID: order.AuthorizationID,
		},
	)
	if err != nil {
		return ReceiptResponse{}, web.NewError(web.Problem{
			Type:   "https://spice.dev/problems/receipt-delivery-failed",
			Title:  "Receipt delivery failed",
			Status: http.StatusBadGateway,
		}, err)
	}
	return ReceiptResponse{
		MessageID:  result.MessageID,
		Transport:  result.Transport,
		Accepted:   result.Accepted,
		Attachment: result.Attachment,
	}, nil
}

func (controller *Controller) orderNotFound() error {
	return web.NewError(web.Problem{
		Type:   "https://spice.dev/problems/order-not-found",
		Title:  "Order not found",
		Status: http.StatusNotFound,
	}, nil)
}

func (controller *Controller) publicOrderError(err error) error {
	status := http.StatusInternalServerError
	problemType := "https://spice.dev/problems/order-failed"
	title := "Order failed"
	detail := ""
	switch {
	case errors.Is(err, ErrInvalidOrder):
		status = http.StatusBadRequest
		problemType = "https://spice.dev/problems/invalid-order"
		title = "Invalid order"
	case errors.Is(err, inventory.ErrInsufficientStock):
		status = http.StatusConflict
		problemType = "https://spice.dev/problems/insufficient-stock"
		title = "Insufficient stock"
	case errors.Is(err, payments.ErrDeclined):
		status = http.StatusConflict
		problemType = "https://spice.dev/problems/payment-declined"
		title = "Payment declined"
	}
	return web.NewError(web.Problem{
		Type:   problemType,
		Title:  title,
		Status: status,
		Detail: detail,
	}, err)
}

func (controller *Controller) orderResponse(order Order) OrderResponse {
	return OrderResponse(order)
}
