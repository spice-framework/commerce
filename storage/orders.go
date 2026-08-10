package storage

import (
	"context"

	"github.com/spice-framework/spice/data"
)

// Orders is the exact persistence contract consumed by the orders module.
type Orders interface {
	Save(context.Context, data.Executor, Record) error
	Find(context.Context, data.Executor, string) (Record, bool, error)
}
