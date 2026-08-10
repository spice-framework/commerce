package payments

// Authorization is an immutable successful payment result.
type Authorization struct {
	ID        string
	Reference string
	Amount    int
}
