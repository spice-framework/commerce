package storage

import "errors"

// ErrInvalidRecord reports an invalid order persistence request.
var ErrInvalidRecord = errors.New("invalid order record")

// ErrDuplicate reports an order ID that is already persisted.
var ErrDuplicate = errors.New("order already exists")
