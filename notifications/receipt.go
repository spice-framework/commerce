package notifications

// Receipt is the transport-independent completed-order notification input.
type Receipt struct {
	OrderID         string
	SKU             string
	Quantity        int
	TotalCents      int
	AuthorizationID string
}
