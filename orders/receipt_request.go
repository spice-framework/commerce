package orders

// ReceiptRequest binds the persisted order that should receive a test or SMTP
// receipt.
type ReceiptRequest struct {
	ID string `path:"id"`
}
