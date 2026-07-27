// @spice.import { Bean, Configuration } from "github.com/StevenBuglione/spice/annotation/core"
// @spice.import { Module } from "github.com/StevenBuglione/spice/annotation/modulith"

// Package payments owns payment authorization behavior.
//
// @Module
package payments

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

var (
	// ErrInvalidPayment reports malformed authorization input.
	ErrInvalidPayment = errors.New("invalid payment")
	// ErrDeclined reports a payment rejected by policy.
	ErrDeclined = errors.New("payment declined")
)

// Settings configures the reference payment policy.
//
// @Configuration(prefix="commerce.payments")
type Settings struct {
	MaximumCents int `spice:"maximum-cents,default=1000000"`
}

// Authorization is an immutable successful payment result.
type Authorization struct {
	ID        string
	Reference string
	Amount    int
}

// Service owns payment authorization state for the reference application.
type Service struct {
	mu           sync.RWMutex
	maximumCents int
	approved     []Authorization
}

// NewService constructs the payment service from validated configuration.
//
// @Bean
func NewService(settings Settings) (*Service, error) {
	if settings.MaximumCents <= 0 {
		return nil, fmt.Errorf(
			"%w: maximum amount must be positive",
			ErrInvalidPayment,
		)
	}
	return &Service{maximumCents: settings.MaximumCents}, nil
}

// Authorize approves a valid payment within the configured policy.
func (service *Service) Authorize(
	ctx context.Context,
	reference string,
	amount int,
) (Authorization, error) {
	if err := ctx.Err(); err != nil {
		return Authorization{}, fmt.Errorf("authorize payment: %w", err)
	}
	if strings.TrimSpace(reference) == "" || amount <= 0 {
		return Authorization{}, fmt.Errorf(
			"%w: reference must be set and amount must be positive",
			ErrInvalidPayment,
		)
	}
	if amount > service.maximumCents {
		return Authorization{}, fmt.Errorf(
			"%w: amount %d exceeds configured maximum %d",
			ErrDeclined,
			amount,
			service.maximumCents,
		)
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	authorization := Authorization{
		ID:        fmt.Sprintf("payment-%06d", len(service.approved)+1),
		Reference: reference,
		Amount:    amount,
	}
	service.approved = append(service.approved, authorization)
	return authorization, nil
}

// Approved returns a stable copy of successful authorizations.
func (service *Service) Approved() []Authorization {
	service.mu.RLock()
	defer service.mu.RUnlock()
	return append([]Authorization(nil), service.approved...)
}
