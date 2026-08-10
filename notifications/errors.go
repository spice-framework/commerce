package notifications

import "errors"

// ErrInvalidReceipt reports malformed receipt data.
var ErrInvalidReceipt = errors.New("invalid commerce receipt")

// ErrDelivery reports a failed mail delivery without exposing transport
// responses, credentials, addresses, or message content.
var ErrDelivery = errors.New("commerce receipt delivery failed")
