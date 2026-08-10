package notifications

import "time"

// Clock makes caller-owned message dates deterministic in tests.
type Clock interface {
	Now() time.Time
}
