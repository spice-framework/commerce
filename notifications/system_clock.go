package notifications

import "time"

// @import { Implements, Service } from "github.com/spice-framework/spice/annotation/core"

// SystemClock supplies the application clock through an explicit interface
// binding.
//
// @Service
// @Implements(Clock)
type SystemClock struct{}

// NewSystemClock constructs the stateless system clock.
func NewSystemClock() *SystemClock {
	return &SystemClock{}
}

// Now returns the current time. Message construction normalizes it to UTC
// seconds.
func (*SystemClock) Now() time.Time {
	return time.Now()
}
