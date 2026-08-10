package orders

// CatalogResponse is the stable public product offered by the reference
// service.
type CatalogResponse struct {
	SKU            string `json:"sku"`
	UnitPriceCents int    `json:"unit_price_cents"`
}
