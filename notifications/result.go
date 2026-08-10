package notifications

// Result is safe delivery metadata suitable for an HTTP response.
type Result struct {
	MessageID  string
	Transport  string
	Accepted   bool
	Attachment string
}
