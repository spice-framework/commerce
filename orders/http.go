package orders

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/StevenBuglione/spice/examples/commerce/inventory"
	"github.com/StevenBuglione/spice/examples/commerce/payments"
	"github.com/StevenBuglione/spice/web"
)

// Controller exposes typed order operations.
//
// @Controller
type Controller struct {
	service *Service
}

// NewController constructs the order HTTP boundary.
//
// @Bean
func NewController(service *Service) *Controller {
	return &Controller{service: service}
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

// OrderResponse is the stable public order representation.
type OrderResponse struct {
	ID              string `json:"id"`
	SKU             string `json:"sku"`
	Quantity        int    `json:"quantity"`
	TotalCents      int    `json:"total_cents"`
	AuthorizationID string `json:"authorization_id"`
}

// Place creates one order through generated strict JSON binding.
//
// @Post("/orders")
func (controller *Controller) Place(
	ctx context.Context,
	request PlaceOrderRequest,
) (OrderResponse, error) {
	order, err := controller.service.Place(ctx, Request{Quantity: request.Body.Quantity})
	if err != nil {
		return OrderResponse{}, publicOrderError(err)
	}
	return orderResponse(order), nil
}

// Get returns one completed order.
//
// @Get("/orders/{id}")
// @cache.Cacheable(name="commerce.orders.by-id")
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
		return OrderResponse{}, web.NewError(web.Problem{
			Type:   "https://spice.dev/problems/order-not-found",
			Title:  "Order not found",
			Status: http.StatusNotFound,
		}, nil)
	}
	return orderResponse(order), nil
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
