package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spice-framework/spice/data"
	"github.com/spice-framework/spice/data/repository"
)

// @import { Implements, Repository } from "github.com/spice-framework/spice/annotation/core"

// OrderRepository is the reflection-free SQL implementation of Orders.
//
// @Repository(constructor=NewOrderRepository)
// @Implements(Orders)
type OrderRepository struct {
	find *repository.Query[Record]
}

// NewOrderRepository constructs bounded typed query metadata.
func NewOrderRepository() (*OrderRepository, error) {
	result := &OrderRepository{}
	find, err := repository.NewQuery(repository.QuerySpec[Record]{
		ID: "commerce.orders.find", Module: moduleID, Statement: findOrderSQL,
		MaxRows: 1, Decode: result.decodeRecord,
	})
	if err != nil {
		return nil, fmt.Errorf("construct order repository: %w", err)
	}
	result.find = find
	return result, nil
}

// Save inserts one immutable order through the caller-owned executor.
func (repository *OrderRepository) Save(ctx context.Context, executor data.Executor, record Record) error {
	if ctx == nil {
		return errors.New("save order: context is nil")
	}
	if cause := context.Cause(ctx); cause != nil {
		return fmt.Errorf("save order: %w", cause)
	}
	if err := repository.validateRecord(record); err != nil {
		return err
	}
	if executor == nil {
		return errors.New("save order: executor is nil")
	}
	result, err := executor.ExecContext(ctx, insertOrderSQL, record.ID, record.SKU,
		record.Quantity, record.TotalCents, record.AuthorizationID)
	if err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return fmt.Errorf("save order %q: %w", record.ID, cause)
		}
		if repository.isDuplicate(err) {
			return fmt.Errorf("save order %q: %w", record.ID, ErrDuplicate)
		}
		return fmt.Errorf("save order %q: persistence failed", record.ID)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return fmt.Errorf("save order %q: persistence cardinality is invalid", record.ID)
	}
	return nil
}

// Find returns one persisted order by exact ID.
func (repository *OrderRepository) Find(ctx context.Context, executor data.Executor, id string) (Record, bool, error) {
	if repository == nil || repository.find == nil {
		return Record{}, false, errors.New("find order: repository is nil")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Record{}, false, fmt.Errorf("%w: order ID is required", ErrInvalidRecord)
	}
	record, found, err := repository.find.Optional(ctx, executor, id)
	if err != nil {
		return Record{}, false, fmt.Errorf("find order %q: %w", id, err)
	}
	return record, found, nil
}

func (repository *OrderRepository) validateRecord(record Record) error {
	if strings.TrimSpace(record.ID) == "" || strings.TrimSpace(record.SKU) == "" ||
		record.Quantity <= 0 || record.TotalCents <= 0 ||
		strings.TrimSpace(record.AuthorizationID) == "" {
		return fmt.Errorf("%w: every field must be valid", ErrInvalidRecord)
	}
	return nil
}

func (repository *OrderRepository) decodeRecord(scanner repository.Scanner) (Record, error) {
	var record Record
	if err := scanner.Scan(&record.ID, &record.SKU, &record.Quantity,
		&record.TotalCents, &record.AuthorizationID); err != nil {
		return Record{}, fmt.Errorf("decode order record: %w", err)
	}
	return record, nil
}

func (repository *OrderRepository) isDuplicate(err error) bool {
	return errors.Is(err, errMemoryDuplicate) ||
		strings.Contains(strings.ToLower(err.Error()), "duplicate key")
}
