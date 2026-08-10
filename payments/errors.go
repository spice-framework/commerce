package payments

import "errors"

// ErrInvalidPayment reports malformed authorization input.
var ErrInvalidPayment = errors.New("invalid payment")

// ErrDeclined reports a payment rejected by policy.
var ErrDeclined = errors.New("payment declined")
