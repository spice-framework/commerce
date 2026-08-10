package orders

// PlaceOrderBody is the strict JSON payload for a new order.
type PlaceOrderBody struct {
	Quantity int `json:"quantity"`
}
