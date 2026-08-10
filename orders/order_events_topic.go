package orders

import "github.com/spice-framework/spice/event"

// @import { Topic } from "github.com/spice-framework/spice/annotation/event"

// OrderEvents declares the generated exact order-view publisher.
//
// @Topic
func OrderEvents(*ViewAudit) event.Publisher[OrderViewed] {
	panic("Spice event marker bodies are never executed")
}
