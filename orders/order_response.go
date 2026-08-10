package orders

// OrderResponse is the stable public order representation.
type OrderResponse struct {
	ID              string `json:"id"`
	SKU             string `json:"sku"`
	Quantity        int    `json:"quantity"`
	TotalCents      int    `json:"total_cents"`
	AuthorizationID string `json:"authorization_id"`
}
