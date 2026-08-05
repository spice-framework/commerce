package orders

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/spice-framework/spice/data"
	"github.com/spice-framework/spice/examples/commerce/inventory"
	"github.com/spice-framework/spice/examples/commerce/notifications"
	"github.com/spice-framework/spice/examples/commerce/payments"
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

// PlaceOrderBody is the strict JSON payload for a new order.
type PlaceOrderBody struct {
	Quantity int `json:"quantity"`
}

// PlaceOrderRequest binds the order JSON body.
type PlaceOrderRequest struct {
	Body PlaceOrderBody `body:""`
}

// Validate rejects invalid quantities before invoking the controller.
func (request PlaceOrderRequest) Validate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if request.Body.Quantity <= 0 {
		return web.NewError(web.Problem{
			Type:   "https://spice.dev/problems/invalid-order",
			Title:  "Invalid order",
			Status: http.StatusBadRequest,
			Detail: "quantity must be positive",
		}, ErrInvalidOrder)
	}
	return nil
}

// GetOrderRequest binds an order ID from the route.
type GetOrderRequest struct {
	ID string `path:"id"`
}

// ReceiptRequest binds the persisted order that should receive a test or SMTP
// receipt.
type ReceiptRequest struct {
	ID string `path:"id"`
}

// CatalogRequest is the explicit empty request DTO for the public catalog.
type CatalogRequest struct{}

// OrderResponse is the stable public order representation.
type OrderResponse struct {
	ID              string `json:"id"`
	SKU             string `json:"sku"`
	Quantity        int    `json:"quantity"`
	TotalCents      int    `json:"total_cents"`
	AuthorizationID string `json:"authorization_id"`
}

// ReceiptResponse contains safe delivery metadata and no recipient or message
// content.
type ReceiptResponse struct {
	MessageID  string `json:"message_id"`
	Transport  string `json:"transport"`
	Accepted   bool   `json:"accepted"`
	Attachment string `json:"attachment"`
}

// CatalogResponse is the stable public product offered by the reference
// service.
type CatalogResponse struct {
	SKU            string `json:"sku"`
	UnitPriceCents int    `json:"unit_price_cents"`
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
		return OrderResponse{}, publicOrderError(err)
	}
	return orderResponse(order), nil
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
		return OrderResponse{}, orderNotFound()
	}
	return orderResponse(order), nil
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
		return ReceiptResponse{}, orderNotFound()
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

func orderNotFound() error {
	return web.NewError(web.Problem{
		Type:   "https://spice.dev/problems/order-not-found",
		Title:  "Order not found",
		Status: http.StatusNotFound,
	}, nil)
}

func publicOrderError(err error) error {
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

func orderResponse(order Order) OrderResponse {
	return OrderResponse(order)
}
