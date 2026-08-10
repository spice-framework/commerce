package payments

import "context"

// Processor is the payment contract consumed by other commerce modules.
type Processor interface {
	Authorize(
		context.Context,
		string,
		int,
	) (Authorization, error)
}
