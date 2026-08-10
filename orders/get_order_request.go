package orders

// GetOrderRequest binds an order ID from the route.
type GetOrderRequest struct {
	ID string `path:"id"`
}
