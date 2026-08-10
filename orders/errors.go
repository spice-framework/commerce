package orders

import "errors"

// ErrInvalidOrder reports malformed order input or configuration.
var ErrInvalidOrder = errors.New("invalid order")

// ErrOrderRejected reports a dependency rejection after compensation.
var ErrOrderRejected = errors.New("order rejected")
