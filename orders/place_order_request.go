package orders

import (
	"context"
	"net/http"

	"github.com/spice-framework/spice/web"
)

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
