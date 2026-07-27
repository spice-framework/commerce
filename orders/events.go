package orders

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/StevenBuglione/spice/event"
)

// @spice.import { Bean } from "github.com/StevenBuglione/spice/annotation/core"
// @spice.import { Listener, Topic } from "github.com/StevenBuglione/spice/annotation/event"

// OrderViewed is emitted after a successful uncached order lookup.
type OrderViewed struct {
	ID string
}

// ViewAudit records bounded per-order view counts for the reference
// application's typed event listener.
type ViewAudit struct {
	mu    sync.RWMutex
	views map[string]uint64
}

// NewViewAudit constructs the provider-owned order view listener.
//
// @Bean
func NewViewAudit() *ViewAudit {
	return &ViewAudit{views: make(map[string]uint64)}
}

// Record consumes one typed order-view event.
//
// @Listener(order=10)
func (audit *ViewAudit) Record(
	ctx context.Context,
	viewed OrderViewed,
) error {
	if audit == nil {
		return errors.New("record order view: audit is nil")
	}
	if ctx == nil {
		return errors.New("record order view: context is nil")
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	id := strings.TrimSpace(viewed.ID)
	if id == "" {
		return errors.New("record order view: order ID is required")
	}
	audit.mu.Lock()
	audit.views[id]++
	audit.mu.Unlock()
	return nil
}

// Views returns the recorded view count for one order.
func (audit *ViewAudit) Views(id string) uint64 {
	if audit == nil {
		return 0
	}
	audit.mu.RLock()
	defer audit.mu.RUnlock()
	return audit.views[strings.TrimSpace(id)]
}

// OrderEvents declares the generated exact order-view publisher.
//
// @Topic
func OrderEvents(*ViewAudit) event.Publisher[OrderViewed] {
	panic("Spice event marker bodies are never executed")
}
