// @import { Execute } from "github.com/spice-framework/spice/annotation/async"
// @import { Bean, Configuration } from "github.com/spice-framework/spice/annotation/core"
// @import { Module } from "github.com/spice-framework/spice/annotation/modulith"
// @import { FixedDelay } from "github.com/spice-framework/spice/annotation/schedule"

// Package inventory owns stock availability and reservation behavior.
//
// @Module
package inventory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

var (
	// ErrInvalidReservation reports malformed reservation input.
	ErrInvalidReservation = errors.New("invalid inventory reservation")
	// ErrInsufficientStock reports that a reservation exceeds current stock.
	ErrInsufficientStock = errors.New("insufficient inventory stock")
)

// Settings configures the reference inventory module.
//
// @Configuration(prefix="commerce.inventory")
type Settings struct {
	SKU          string `spice:"sku,default=SKU-RED"`
	InitialStock int    `spice:"initial-stock,default=10"`
}

// Service owns inventory state for the reference application.
type Service struct {
	mu    sync.RWMutex
	stock map[string]int
}

// NewService constructs the inventory service from validated configuration.
//
// @Bean
func NewService(settings Settings) (*Service, error) {
	sku := strings.TrimSpace(settings.SKU)
	if sku == "" {
		return nil, fmt.Errorf("%w: configured SKU is empty", ErrInvalidReservation)
	}
	if settings.InitialStock < 0 {
		return nil, fmt.Errorf(
			"%w: initial stock for %q must not be negative",
			ErrInvalidReservation,
			sku,
		)
	}
	return &Service{stock: map[string]int{sku: settings.InitialStock}}, nil
}

// Reserve atomically removes quantity units from available stock.
func (service *Service) Reserve(ctx context.Context, sku string, quantity int) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("reserve inventory: %w", err)
	}
	if sku == "" || quantity <= 0 {
		return fmt.Errorf(
			"%w: SKU must be set and quantity must be positive",
			ErrInvalidReservation,
		)
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	available := service.stock[sku]
	if available < quantity {
		return fmt.Errorf(
			"%w: requested %d units of %q, available %d",
			ErrInsufficientStock,
			quantity,
			sku,
			available,
		)
	}
	service.stock[sku] = available - quantity
	return nil
}

// Release restores a previously successful reservation.
func (service *Service) Release(sku string, quantity int) error {
	if sku == "" || quantity <= 0 {
		return fmt.Errorf(
			"%w: SKU must be set and quantity must be positive",
			ErrInvalidReservation,
		)
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	available, known := service.stock[sku]
	if !known {
		return fmt.Errorf("%w: SKU %q is not configured", ErrInvalidReservation, sku)
	}
	service.stock[sku] = available + quantity
	return nil
}

// Audit verifies the module's stock invariants without mutating inventory.
//
// @FixedDelay(delay="5m", initialDelay="30s")
func (service *Service) Audit(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("audit inventory: %w", err)
	}
	service.mu.RLock()
	defer service.mu.RUnlock()
	for sku, available := range service.stock {
		if strings.TrimSpace(sku) == "" || available < 0 {
			return fmt.Errorf(
				"%w: invalid stock state for %q",
				ErrInvalidReservation,
				sku,
			)
		}
	}
	return nil
}

// VerifySKU asynchronously checks one configured stock record without
// mutating inventory.
//
// @Execute
func (service *Service) VerifySKU(
	ctx context.Context,
	sku string,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("verify inventory SKU: %w", err)
	}
	service.mu.RLock()
	defer service.mu.RUnlock()
	available, known := service.stock[sku]
	if !known || available < 0 {
		return fmt.Errorf(
			"%w: invalid stock state for %q",
			ErrInvalidReservation,
			sku,
		)
	}
	return nil
}

// Available returns the current stock for sku.
func (service *Service) Available(sku string) int {
	service.mu.RLock()
	defer service.mu.RUnlock()
	return service.stock[sku]
}
