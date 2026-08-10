package orders

// ReceiptResponse contains safe delivery metadata and no recipient or message
// content.
type ReceiptResponse struct {
	MessageID  string `json:"message_id"`
	Transport  string `json:"transport"`
	Accepted   bool   `json:"accepted"`
	Attachment string `json:"attachment"`
}
