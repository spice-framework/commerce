package orders

// Order is an immutable completed order.
type Order struct {
	ID              string
	SKU             string
	Quantity        int
	TotalCents      int
	AuthorizationID string
}
