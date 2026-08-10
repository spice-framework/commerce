// @import { Service } from "github.com/spice-framework/spice/annotation/core"
// @import { Module } from "github.com/spice-framework/spice/annotation/modulith"

// Package notifications owns explicit commerce receipt composition and mail
// transport selection.
//
// @Module
package notifications

import (
	"context"
	"errors"
	"fmt"

	"github.com/spice-framework/spice/mail"
)

// Notifier composes immutable typed MIME messages and sends through the exact
// mail.Sender interface bean.
//
// @Service(constructor=NewNotifier)
type Notifier struct {
	settings Settings
	sender   mail.Sender
	delivery *Delivery
	clock    Clock
}

// NewNotifier constructs the receipt workflow from explicit dependencies.
func NewNotifier(
	settings Settings,
	sender mail.Sender,
	delivery *Delivery,
	clock Clock,
) (*Notifier, error) {
	if sender == nil {
		return nil, errors.New("construct commerce notifier: sender is nil")
	}
	if delivery == nil {
		return nil, errors.New("construct commerce notifier: delivery is nil")
	}
	if clock == nil {
		return nil, errors.New("construct commerce notifier: clock is nil")
	}
	return &Notifier{
		settings: settings,
		sender:   sender,
		delivery: delivery,
		clock:    clock,
	}, nil
}

// SendReceipt validates, composes, and delivers one inspectable receipt with a
// deterministic text attachment. It never starts an implicit goroutine.
func (notifier *Notifier) SendReceipt(
	ctx context.Context,
	receipt Receipt,
) (Result, error) {
	if ctx == nil {
		return Result{}, errors.New("send commerce receipt: context is nil")
	}
	if cause := context.Cause(ctx); cause != nil {
		return Result{}, fmt.Errorf("send commerce receipt: %w", cause)
	}
	if notifier == nil ||
		notifier.sender == nil ||
		notifier.delivery == nil ||
		notifier.clock == nil {
		return Result{}, errors.New("send commerce receipt: notifier is nil")
	}
	if err := notifier.validateReceipt(receipt); err != nil {
		return Result{}, err
	}
	messageID := "receipt-" + receipt.OrderID + "@commerce.example"
	body := fmt.Sprintf(
		"Order: %s\nSKU: %s\nQuantity: %d\nTotal: %d cents\nAuthorization: %s\n",
		receipt.OrderID,
		receipt.SKU,
		receipt.Quantity,
		receipt.TotalCents,
		receipt.AuthorizationID,
	)
	message, err := mail.NewMessage(mail.MessageSpec{
		ID:       messageID,
		Date:     notifier.clock.Now(),
		From:     notifier.settings.From,
		To:       []string{notifier.settings.Recipient},
		Subject:  "Spice Commerce receipt " + receipt.OrderID,
		TextBody: body,
		Attachments: []mail.AttachmentSpec{{
			Filename:    receipt.OrderID + ".txt",
			ContentType: "text/plain; charset=utf-8",
			Data:        []byte(body),
		}},
	})
	if err != nil {
		return Result{}, fmt.Errorf("%w: compose message", ErrInvalidReceipt)
	}
	if err := notifier.sender.Send(ctx, message); err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return Result{}, fmt.Errorf("send commerce receipt: %w", cause)
		}
		return Result{}, ErrDelivery
	}
	return Result{
		MessageID:  messageID,
		Transport:  notifier.delivery.Mode(),
		Accepted:   true,
		Attachment: receipt.OrderID + ".txt",
	}, nil
}

func (notifier *Notifier) validateReceipt(receipt Receipt) error {
	if !notifier.safeIdentifier(receipt.OrderID) ||
		!notifier.safeIdentifier(receipt.SKU) ||
		!notifier.safeIdentifier(receipt.AuthorizationID) ||
		receipt.Quantity <= 0 ||
		receipt.TotalCents <= 0 {
		return fmt.Errorf("%w: every field must be valid", ErrInvalidReceipt)
	}
	return nil
}

func (notifier *Notifier) safeIdentifier(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '-' ||
			character == '_' {
			continue
		}
		return false
	}
	return true
}
