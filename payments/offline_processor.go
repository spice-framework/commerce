package payments

import (
	"context"
	"fmt"
)

// @import { Fallback, Implements, Qualifier, Service } from "github.com/spice-framework/spice/annotation/core"

// OfflineProcessor is a deliberately safe fallback candidate. It demonstrates
// that a regular qualified/primary bean wins while retaining an explicit
// deterministic fallback for applications that omit the Stripe service.
//
// @Service(name="offlineProcessor")
// @Implements(Processor)
// @Qualifier("offline")
// @Fallback
type OfflineProcessor struct{}

// Authorize fails closed when the offline fallback is selected.
func (*OfflineProcessor) Authorize(
	ctx context.Context,
	_ string,
	_ int,
) (Authorization, error) {
	if err := ctx.Err(); err != nil {
		return Authorization{}, fmt.Errorf(
			"authorize offline payment: %w",
			err,
		)
	}
	return Authorization{}, fmt.Errorf(
		"%w: offline payment processing is unavailable",
		ErrDeclined,
	)
}
