// @import { Configuration, Implements, Service } from "github.com/StevenBuglione/spice/annotation/core"
// @import { Module } from "github.com/StevenBuglione/spice/annotation/modulith"

// Package notifications owns explicit commerce receipt composition and mail
// transport selection.
//
// @Module
package notifications

import (
	"context"
	"errors"
	"fmt"
	netmail "net/mail"
	"strings"
	"time"

	"github.com/StevenBuglione/spice/mail"
	"github.com/StevenBuglione/spice/mail/mailtest"
	"github.com/StevenBuglione/spice/starter/smtp"
)

const (
	transportTest = "test"
	transportSMTP = "smtp"
)

var (
	// ErrInvalidReceipt reports malformed receipt data.
	ErrInvalidReceipt = errors.New("invalid commerce receipt")
	// ErrDelivery reports a failed mail delivery without exposing transport
	// responses, credentials, addresses, or message content.
	ErrDelivery = errors.New("commerce receipt delivery failed")
)

// Settings selects an instance-owned mail transport and immutable envelope.
// Test transport is the safe zero-network default for the developer workflow.
//
// @Configuration(prefix="commerce.mail")
type Settings struct {
	Transport      string        `spice:"transport,default=test,env=SPICE_COMMERCE_MAIL_TRANSPORT"`
	From           string        `spice:"from,default=Spice Commerce <no-reply@commerce.example>,env=SPICE_COMMERCE_MAIL_FROM"`
	Recipient      string        `spice:"recipient,default=Developer <developer@commerce.example>,env=SPICE_COMMERCE_MAIL_RECIPIENT,secret"`
	TestCapacity   int           `spice:"test-capacity,default=100"`
	SMTPAddress    string        `spice:"smtp-address,env=SPICE_COMMERCE_MAIL_SMTP_ADDRESS"`
	SMTPServerName string        `spice:"smtp-server-name,env=SPICE_COMMERCE_MAIL_SMTP_SERVER_NAME"`
	SMTPMode       string        `spice:"smtp-mode,default=starttls,env=SPICE_COMMERCE_MAIL_SMTP_MODE"`
	SMTPUsername   string        `spice:"smtp-username,env=SPICE_COMMERCE_MAIL_SMTP_USERNAME,secret"`
	SMTPPassword   string        `spice:"smtp-password,env=SPICE_COMMERCE_MAIL_SMTP_PASSWORD,secret"`
	Timeout        time.Duration `spice:"timeout,default=5s"`
	MaxAttempts    int           `spice:"max-attempts,default=1"`
}

// Clock makes caller-owned message dates deterministic in tests.
type Clock interface {
	Now() time.Time
}

// SystemClock supplies the application clock through an explicit interface
// binding.
//
// @Service
// @Implements(Clock)
type SystemClock struct{}

// NewSystemClock constructs the stateless system clock.
func NewSystemClock() *SystemClock {
	return &SystemClock{}
}

// Now returns the current time. Message construction normalizes it to UTC
// seconds.
func (*SystemClock) Now() time.Time {
	return time.Now()
}

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
	if err := validateAddress("from", settings.From); err != nil {
		return nil, err
	}
	if err := validateAddress("recipient", settings.Recipient); err != nil {
		return nil, err
	}
	switch settings.Transport {
	case transportTest:
		sender, err := mailtest.New(mailtest.Config{
			Capacity: settings.TestCapacity,
		})
		if err != nil {
			return nil, fmt.Errorf("construct commerce test mail transport: %w", err)
		}
		return &Delivery{
			sender: sender,
			test:   sender,
			mode:   transportTest,
		}, nil
	case transportSMTP:
		sender, err := smtp.New(smtp.Config{
			Address:     settings.SMTPAddress,
			ServerName:  settings.SMTPServerName,
			Mode:        smtp.TLSMode(settings.SMTPMode),
			Username:    settings.SMTPUsername,
			Password:    settings.SMTPPassword,
			Timeout:     settings.Timeout,
			MaxAttempts: settings.MaxAttempts,
		})
		if err != nil {
			return nil, fmt.Errorf("construct commerce SMTP transport: %w", err)
		}
		return &Delivery{sender: sender, mode: transportSMTP}, nil
	default:
		return nil, errors.New(
			"construct commerce mail transport: transport must be test or smtp",
		)
	}
}

// Send delegates to the selected transport while preserving caller
// cancellation and transport-owned retry and observation semantics.
func (delivery *Delivery) Send(
	ctx context.Context,
	message mail.Message,
) error {
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
// SMTP delivery remains intentionally unreadable from application memory.
func (delivery *Delivery) Messages() []mailtest.Snapshot {
	if delivery == nil || delivery.test == nil {
		return nil
	}
	return delivery.test.Messages()
}

// Receipt is the transport-independent completed-order notification input.
type Receipt struct {
	OrderID         string
	SKU             string
	Quantity        int
	TotalCents      int
	AuthorizationID string
}

// Result is safe delivery metadata suitable for an HTTP response.
type Result struct {
	MessageID  string
	Transport  string
	Accepted   bool
	Attachment string
}

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
	if err := validateReceipt(receipt); err != nil {
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

func validateAddress(label, value string) error {
	if value == "" ||
		strings.TrimSpace(value) != value ||
		strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf(
			"construct commerce mail transport: %s address is invalid",
			label,
		)
	}
	parsed, err := netmail.ParseAddress(value)
	if err != nil || parsed.Address == "" {
		return fmt.Errorf(
			"construct commerce mail transport: %s address is invalid",
			label,
		)
	}
	return nil
}

func validateReceipt(receipt Receipt) error {
	if !safeIdentifier(receipt.OrderID) ||
		!safeIdentifier(receipt.SKU) ||
		!safeIdentifier(receipt.AuthorizationID) ||
		receipt.Quantity <= 0 ||
		receipt.TotalCents <= 0 {
		return fmt.Errorf("%w: every field must be valid", ErrInvalidReceipt)
	}
	return nil
}

func safeIdentifier(value string) bool {
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
