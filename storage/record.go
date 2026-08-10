package storage

// Record is the persistence representation shared with the orders module.
type Record struct {
	ID              string
	SKU             string
	Quantity        int
	TotalCents      int
	AuthorizationID string
}
