package notifications

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/StevenBuglione/spice/mail"
)

var (
	fixedReceiptTime = time.Date(2026, 7, 28, 12, 30, 0, 0, time.UTC)
	errTestDelivery  = errors.New("test delivery failed with secret detail")
)

type fixedClock struct {
	value time.Time
}

func (clock fixedClock) Now() time.Time {
	return clock.value
}

type failingSender struct{}

func (failingSender) Send(context.Context, mail.Message) error {
	return errTestDelivery
}

func TestNotifierDeliversInspectableTestReceipt(t *testing.T) {
	t.Parallel()
	settings := validTestSettings()
	delivery, err := NewDelivery(settings)
	if err != nil {
		t.Fatalf("NewDelivery() error = %v", err)
	}
	notifier, err := NewNotifier(
		settings,
		delivery,
		delivery,
		fixedClock{value: fixedReceiptTime},
	)
	if err != nil {
		t.Fatalf("NewNotifier() error = %v", err)
	}
	result, err := notifier.SendReceipt(
		context.Background(),
		validReceipt(),
	)
	if err != nil {
		t.Fatalf("SendReceipt() error = %v", err)
	}
	if result != (Result{
		MessageID:  "receipt-order-000001@commerce.example",
		Transport:  transportTest,
		Accepted:   true,
		Attachment: "order-000001.txt",
	}) {
		t.Fatalf("SendReceipt() result = %#v", result)
	}
	messages := delivery.Messages()
	if len(messages) != 1 {
		t.Fatalf("Messages() = %#v", messages)
	}
	message := messages[0]
	if message.ID() != result.MessageID ||
		message.EnvelopeFrom() != "no-reply@commerce.example" ||
		len(message.Recipients()) != 1 ||
		message.Recipients()[0] != "developer@commerce.example" ||
		message.Subject() != "Spice Commerce receipt order-000001" ||
		!strings.Contains(message.TextBody(), "Total: 5000 cents") {
		t.Fatalf("delivered message = %#v", message)
	}
	attachments := message.Attachments()
	if len(attachments) != 1 ||
		attachments[0].Filename() != "order-000001.txt" ||
		attachments[0].ContentType() != "text/plain; charset=utf-8" ||
		!strings.Contains(
			string(attachments[0].Bytes()),
			"Authorization: payment-000001",
		) {
		t.Fatalf("delivered attachments = %#v", attachments)
	}
}

func TestNotifierPreservesCancellationAndSanitizesDeliveryFailure(t *testing.T) {
	t.Parallel()
	settings := validTestSettings()
	notifier, err := NewNotifier(
		settings,
		failingSender{},
		&Delivery{mode: transportTest},
		fixedClock{value: fixedReceiptTime},
	)
	if err != nil {
		t.Fatalf("NewNotifier() error = %v", err)
	}
	if _, err := notifier.SendReceipt(
		context.Background(),
		validReceipt(),
	); !errors.Is(err, ErrDelivery) ||
		strings.Contains(err.Error(), "secret detail") {
		t.Fatalf("SendReceipt(failure) error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := notifier.SendReceipt(
		ctx,
		validReceipt(),
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("SendReceipt(canceled) error = %v", err)
	}
}

func TestNotificationValidationAndTransportSelection(t *testing.T) {
	t.Parallel()
	for _, settings := range []Settings{
		{},
		{
			Transport:    transportTest,
			From:         "invalid",
			Recipient:    "developer@commerce.example",
			TestCapacity: 1,
		},
		{
			Transport:    transportTest,
			From:         "no-reply@commerce.example",
			Recipient:    "invalid",
			TestCapacity: 1,
		},
		{
			Transport:    "unknown",
			From:         "no-reply@commerce.example",
			Recipient:    "developer@commerce.example",
			TestCapacity: 1,
		},
	} {
		if _, err := NewDelivery(settings); err == nil {
			t.Fatalf("NewDelivery(%#v) error = nil", settings)
		}
	}
	smtpSettings := validTestSettings()
	smtpSettings.Transport = transportSMTP
	smtpSettings.SMTPAddress = "smtp.example:465"
	smtpSettings.SMTPServerName = "smtp.example"
	smtpSettings.SMTPMode = string("implicit-tls")
	smtpSettings.SMTPUsername = "commerce"
	smtpSettings.SMTPPassword = "secret"
	delivery, err := NewDelivery(smtpSettings)
	if err != nil {
		t.Fatalf("NewDelivery(SMTP) error = %v", err)
	}
	if delivery.Mode() != transportSMTP || delivery.Messages() != nil {
		t.Fatalf(
			"SMTP delivery mode=%q messages=%#v",
			delivery.Mode(),
			delivery.Messages(),
		)
	}
	if _, err := NewNotifier(
		validTestSettings(),
		nil,
		delivery,
		fixedClock{value: fixedReceiptTime},
	); err == nil {
		t.Fatal("NewNotifier(nil sender) error = nil")
	}
	if _, err := NewNotifier(
		validTestSettings(),
		failingSender{},
		nil,
		fixedClock{value: fixedReceiptTime},
	); err == nil {
		t.Fatal("NewNotifier(nil delivery) error = nil")
	}
	if _, err := NewNotifier(
		validTestSettings(),
		failingSender{},
		delivery,
		nil,
	); err == nil {
		t.Fatal("NewNotifier(nil clock) error = nil")
	}
}

func TestNotifierRejectsInvalidReceiptsAndNilValues(t *testing.T) {
	t.Parallel()
	settings := validTestSettings()
	delivery, err := NewDelivery(settings)
	if err != nil {
		t.Fatalf("NewDelivery() error = %v", err)
	}
	notifier, err := NewNotifier(
		settings,
		delivery,
		delivery,
		fixedClock{value: fixedReceiptTime},
	)
	if err != nil {
		t.Fatalf("NewNotifier() error = %v", err)
	}
	for _, receipt := range []Receipt{
		{},
		{
			OrderID:         "order/one",
			SKU:             "SKU-RED",
			Quantity:        1,
			TotalCents:      2500,
			AuthorizationID: "payment-1",
		},
		{
			OrderID:         "order-1",
			SKU:             "SKU RED",
			Quantity:        1,
			TotalCents:      2500,
			AuthorizationID: "payment-1",
		},
	} {
		if _, err := notifier.SendReceipt(
			context.Background(),
			receipt,
		); !errors.Is(err, ErrInvalidReceipt) {
			t.Fatalf("SendReceipt(%#v) error = %v", receipt, err)
		}
	}
	if _, err := notifier.SendReceipt(nilContext(), validReceipt()); err == nil {
		t.Fatal("SendReceipt(nil context) error = nil")
	}
	if _, err := (*Notifier)(nil).SendReceipt(
		context.Background(),
		validReceipt(),
	); err == nil {
		t.Fatal("nil Notifier.SendReceipt() error = nil")
	}
	if err := (*Delivery)(nil).Send(
		context.Background(),
		mail.Message{},
	); err == nil {
		t.Fatal("nil Delivery.Send() error = nil")
	}
	if err := delivery.Send(nilContext(), mail.Message{}); err == nil {
		t.Fatal("Delivery.Send(nil context) error = nil")
	}
	if (*Delivery)(nil).Mode() != "" ||
		(*Delivery)(nil).Messages() != nil {
		t.Fatal("nil Delivery metadata is not empty")
	}
}

func validTestSettings() Settings {
	return Settings{
		Transport:    transportTest,
		From:         "Spice Commerce <no-reply@commerce.example>",
		Recipient:    "Developer <developer@commerce.example>",
		TestCapacity: 10,
		SMTPMode:     "starttls",
		Timeout:      time.Second,
		MaxAttempts:  1,
	}
}

func validReceipt() Receipt {
	return Receipt{
		OrderID:         "order-000001",
		SKU:             "SKU-RED",
		Quantity:        2,
		TotalCents:      5000,
		AuthorizationID: "payment-000001",
	}
}

func nilContext() context.Context {
	return nil
}
