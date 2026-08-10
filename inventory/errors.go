package inventory

import "errors"

// ErrInvalidReservation reports malformed reservation input.
var ErrInvalidReservation = errors.New("invalid inventory reservation")

// ErrInsufficientStock reports that a reservation exceeds current stock.
var ErrInsufficientStock = errors.New("insufficient inventory stock")
