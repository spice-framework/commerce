package notifications

import (
	"context"
	"errors"
	"fmt"
	netmail "net/mail"
	"strings"

	"github.com/spice-framework/spice/mail"
	"github.com/spice-framework/spice/mail/mailtest"
	startersmtp "github.com/spice-framework/starter-smtp"
)

// @import { Implements, Service } from "github.com/spice-framework/spice/annotation/core"

const (
	transportTest = "test"
	transportSMTP = "smtp"
)

// Delivery is the one concrete mail.Sender bean. It owns either a bounded
// test sender or the secure SMTP starter and never installs a global client.
//
// @Service(constructor=NewDelivery)
// @Implements(mail.Sender)
type Delivery struct {
	sender mail.Sender
	test   *mailtest.Sender
	mode   string
}

// NewDelivery validates the public envelope and constructs the selected
// instance-owned transport without opening a network connection.
func NewDelivery(settings Settings) (*Delivery, error) {
	delivery := &Delivery{}
	if err := delivery.validateAddress("from", settings.From); err != nil {
		return nil, err
	}
	if err := delivery.validateAddress("recipient", settings.Recipient); err != nil {
		return nil, err
	}
	switch settings.Transport {
	case transportTest:
		sender, err := mailtest.New(mailtest.Config{Capacity: settings.TestCapacity})
		if err != nil {
			return nil, fmt.Errorf("construct commerce test mail transport: %w", err)
		}
		return &Delivery{sender: sender, test: sender, mode: transportTest}, nil
	case transportSMTP:
		sender, err := startersmtp.New(startersmtp.Config{
			Address: settings.SMTPAddress, ServerName: settings.SMTPServerName,
			Mode: startersmtp.TLSMode(settings.SMTPMode), Username: settings.SMTPUsername,
			Password: settings.SMTPPassword, Timeout: settings.Timeout, MaxAttempts: settings.MaxAttempts,
		})
		if err != nil {
			return nil, fmt.Errorf("construct commerce SMTP transport: %w", err)
		}
		return &Delivery{sender: sender, mode: transportSMTP}, nil
	default:
		return nil, errors.New("construct commerce mail transport: transport must be test or smtp")
	}
}

// Send delegates to the selected transport while preserving caller
// cancellation and transport-owned retry and observation semantics.
func (delivery *Delivery) Send(ctx context.Context, message mail.Message) error {
	if ctx == nil {
		return errors.New("send commerce mail: context is nil")
	}
	if delivery == nil || delivery.sender == nil {
		return errors.New("send commerce mail: delivery is nil")
	}
	return delivery.sender.Send(ctx, message)
}

// Mode reports the selected transport without exposing its configuration.
func (delivery *Delivery) Mode() string {
	if delivery == nil {
		return ""
	}
	return delivery.mode
}

// Messages returns delivered snapshots only for the bounded test transport.
func (delivery *Delivery) Messages() []mailtest.Snapshot {
	if delivery == nil || delivery.test == nil {
		return nil
	}
	return delivery.test.Messages()
}

func (delivery *Delivery) validateAddress(label, value string) error {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("construct commerce mail transport: %s address is invalid", label)
	}
	parsed, err := netmail.ParseAddress(value)
	if err != nil || parsed.Address == "" {
		return fmt.Errorf("construct commerce mail transport: %s address is invalid", label)
	}
	return nil
}
