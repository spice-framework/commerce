package smtp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"net/textproto"
	"time"

	spicemail "github.com/spice-framework/spice/mail"
	"github.com/spice-framework/spice/retry"
)

// Stage identifies the SMTP operation that failed without exposing message or
// server-response content.
type Stage string

const (
	// StageDial establishes the TCP or implicit-TLS connection.
	StageDial Stage = "dial"
	// StageGreeting reads the SMTP greeting and establishes the client.
	StageGreeting Stage = "greeting"
	// StageHello sends the configured EHLO/HELO identity.
	StageHello Stage = "hello"
	// StageStartTLS requires and negotiates STARTTLS.
	StageStartTLS Stage = "starttls"
	// StageAuthenticate authenticates after TLS is active.
	StageAuthenticate Stage = "authenticate"
	// StageEnvelopeFrom sends MAIL FROM.
	StageEnvelopeFrom Stage = "envelope-from"
	// StageRecipient sends one RCPT TO command.
	StageRecipient Stage = "recipient"
	// StageData starts the SMTP DATA transaction.
	StageData Stage = "data"
	// StageMessage writes MIME bytes after DATA has begun.
	StageMessage Stage = "message"
	// StageAcceptance waits for the final server acceptance after message data.
	StageAcceptance Stage = "acceptance"
)

// Outcome is the payload-free result of one delivery attempt.
type Outcome string

const (
	// OutcomeDelivered means the server accepted the complete message.
	OutcomeDelivered Outcome = "delivered"
	// OutcomeRetrying means a safe pre-DATA transient failure will be retried.
	OutcomeRetrying Outcome = "retrying"
	// OutcomeFailed means delivery stopped without server acceptance.
	OutcomeFailed Outcome = "failed"
	// OutcomeCanceled means caller cancellation or the configured timeout
	// stopped the attempt.
	OutcomeCanceled Outcome = "canceled"
)

// Observation contains bounded delivery metadata. It intentionally excludes
// credentials, addresses, subjects, bodies, attachments, and server text.
type Observation struct {
	MessageID   string
	Attempt     int
	MaxAttempts int
	Outcome     Outcome
	Stage       Stage
	Code        int
	Temporary   bool
	NextBackoff time.Duration
	Duration    time.Duration
}

// Observer receives one observation synchronously after each attempt.
type Observer func(context.Context, Observation)

// DeliveryError reports a sanitized SMTP failure. Unwrap preserves the
// underlying error for programmatic cancellation and network classification;
// Error never includes its potentially sensitive text.
type DeliveryError struct {
	stage     Stage
	code      int
	temporary bool
	retrySafe bool
	cause     error
}

// Error returns a payload- and server-text-free failure description.
func (err *DeliveryError) Error() string {
	if err == nil {
		return "SMTP delivery failed"
	}
	if err.code != 0 {
		return fmt.Sprintf("SMTP delivery failed at %s with status %d", err.stage, err.code)
	}
	return fmt.Sprintf("SMTP delivery failed at %s", err.stage)
}

// Unwrap exposes the original error for errors.Is and errors.As.
func (err *DeliveryError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

// Stage returns the failed operation.
func (err *DeliveryError) Stage() Stage {
	if err == nil {
		return ""
	}
	return err.stage
}

// Code returns an SMTP response code, or zero for transport failures.
func (err *DeliveryError) Code() int {
	if err == nil {
		return 0
	}
	return err.code
}

// Temporary reports whether the failure was classified as transient.
func (err *DeliveryError) Temporary() bool {
	return err != nil && err.temporary
}

// RetrySafe reports whether another attempt cannot duplicate a message that
// may already have been accepted.
func (err *DeliveryError) RetrySafe() bool {
	return err != nil && err.retrySafe
}

// Sender owns immutable configuration for one SMTP service. It has no global
// client and opens one connection per Send attempt.
type Sender struct {
	config normalizedConfig
}

type smtpSession struct {
	connection net.Conn
	client     *smtp.Client
	stop       func() bool
}

// New validates and defensively copies an SMTP sender configuration. It never
// resolves a host or opens a network connection.
func New(config Config) (*Sender, error) {
	normalized, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}
	return &Sender{config: normalized}, nil
}

// Send delivers one immutable message with caller-owned cancellation. Only
// transient failures before DATA begins are eligible for bounded retry.
func (sender *Sender) Send(ctx context.Context, message spicemail.Message) error {
	if sender == nil {
		return errors.New("send SMTP message: sender is nil")
	}
	if ctx == nil {
		return errors.New("send SMTP message: context is nil")
	}
	if err := validateMessage(message); err != nil {
		return err
	}
	config := sender.config
	return retry.Run(ctx, retry.Policy{
		ID:             "smtp.Send",
		Module:         "github.com/spice-framework/spice/starter/smtp",
		MaxAttempts:    config.maxAttempts,
		InitialBackoff: config.initialBackoff,
		MaxBackoff:     config.maxBackoff,
		Multiplier:     config.multiplier,
		Retryable:      retryableDelivery,
		Wait:           config.wait,
		Observer: func(observeCtx context.Context, observation retry.Observation) {
			observeAttempt(config, observeCtx, message.ID(), observation)
		},
	}, func(attemptCtx context.Context, _ retry.Attempt) error {
		return sender.sendAttempt(attemptCtx, message)
	})
}

func (sender *Sender) sendAttempt(ctx context.Context, message spicemail.Message) error {
	attemptCtx, cancel := context.WithTimeout(ctx, sender.config.timeout)
	defer cancel()

	session, sessionErr := sender.openSession(attemptCtx)
	if sessionErr != nil {
		return sessionErr
	}
	defer session.close()
	if envelopeErr := sendEnvelope(attemptCtx, session.client, message); envelopeErr != nil {
		return envelopeErr
	}
	return sendMessageData(attemptCtx, session.client, message)
}

func (sender *Sender) openSession(ctx context.Context) (*smtpSession, error) {
	connection, dialErr := sender.dial(ctx)
	if dialErr != nil {
		return nil, newDeliveryError(ctx, StageDial, true, dialErr)
	}
	stopCancellation := context.AfterFunc(ctx, func() {
		closeSilently(connection)
	})
	session := &smtpSession{
		connection: connection,
		stop:       stopCancellation,
	}
	if deadline, ok := ctx.Deadline(); ok {
		if deadlineErr := connection.SetDeadline(deadline); deadlineErr != nil {
			session.close()
			return nil, newDeliveryError(ctx, StageDial, true, deadlineErr)
		}
	}
	client, greetingErr := smtp.NewClient(connection, sender.config.serverName)
	if greetingErr != nil {
		session.close()
		return nil, newDeliveryError(ctx, StageGreeting, true, greetingErr)
	}
	session.client = client
	if helloErr := client.Hello(sender.config.clientName); helloErr != nil {
		session.close()
		return nil, newDeliveryError(ctx, StageHello, true, helloErr)
	}
	if sender.config.mode == TLSModeStartTLS {
		if tlsErr := sender.startTLS(client); tlsErr != nil {
			session.close()
			return nil, newDeliveryError(ctx, StageStartTLS, true, tlsErr)
		}
	}
	if authErr := sender.authenticate(client); authErr != nil {
		session.close()
		return nil, newDeliveryError(ctx, StageAuthenticate, true, authErr)
	}
	return session, nil
}

func (sender *Sender) authenticate(client *smtp.Client) error {
	if sender.config.username == "" {
		return nil
	}
	auth := smtp.PlainAuth(
		"",
		sender.config.username,
		sender.config.password,
		sender.config.serverName,
	)
	return client.Auth(auth)
}

func sendEnvelope(
	ctx context.Context,
	client *smtp.Client,
	message spicemail.Message,
) error {
	if mailErr := client.Mail(message.EnvelopeFrom()); mailErr != nil {
		return newDeliveryError(ctx, StageEnvelopeFrom, true, mailErr)
	}
	for _, recipient := range message.Recipients() {
		if recipientErr := client.Rcpt(recipient); recipientErr != nil {
			return newDeliveryError(ctx, StageRecipient, true, recipientErr)
		}
	}
	return nil
}

func sendMessageData(
	ctx context.Context,
	client *smtp.Client,
	message spicemail.Message,
) error {
	writer, dataErr := client.Data()
	if dataErr != nil {
		return newDeliveryError(ctx, StageData, true, dataErr)
	}
	if _, writeErr := writer.Write(message.Bytes()); writeErr != nil {
		closeSilently(writer)
		return newDeliveryError(ctx, StageMessage, false, writeErr)
	}
	if acceptanceErr := writer.Close(); acceptanceErr != nil {
		return newDeliveryError(ctx, StageAcceptance, false, acceptanceErr)
	}
	// A failed QUIT after final DATA acceptance does not make replay safe.
	quitAfterAcceptance(client)
	return nil
}

func (session *smtpSession) close() {
	if session == nil {
		return
	}
	if session.stop != nil {
		session.stop()
	}
	if session.client != nil {
		closeSilently(session.client)
	}
	if session.connection != nil {
		closeSilently(session.connection)
	}
}

func closeSilently(closer io.Closer) {
	if closeErr := closer.Close(); closeErr != nil {
		return
	}
}

func quitAfterAcceptance(client *smtp.Client) {
	if quitErr := client.Quit(); quitErr != nil {
		return
	}
}

func (sender *Sender) dial(ctx context.Context) (net.Conn, error) {
	dialer := &net.Dialer{}
	if sender.config.mode == TLSModeImplicitTLS {
		tlsDialer := &tls.Dialer{
			NetDialer: dialer,
			Config:    sender.config.tlsConfig.Clone(),
		}
		return tlsDialer.DialContext(ctx, "tcp", sender.config.address)
	}
	return dialer.DialContext(ctx, "tcp", sender.config.address)
}

func (sender *Sender) startTLS(client *smtp.Client) error {
	if available, _ := client.Extension("STARTTLS"); !available {
		return errors.New("SMTP server does not advertise required STARTTLS")
	}
	return client.StartTLS(sender.config.tlsConfig.Clone())
}

func validateMessage(message spicemail.Message) error {
	if message.ID() == "" ||
		message.EnvelopeFrom() == "" ||
		len(message.Recipients()) == 0 ||
		len(message.Bytes()) == 0 {
		return errors.New("send SMTP message: initialized mail message is required")
	}
	return nil
}

func retryableDelivery(err error) bool {
	delivery, ok := errors.AsType[*DeliveryError](err)
	return ok && delivery.temporary && delivery.retrySafe
}

func newDeliveryError(
	ctx context.Context,
	stage Stage,
	retrySafe bool,
	cause error,
) *DeliveryError {
	cause = contextualCause(ctx, cause)
	code, temporary := classifyFailure(cause)
	return &DeliveryError{
		stage:     stage,
		code:      code,
		temporary: temporary,
		retrySafe: retrySafe,
		cause:     cause,
	}
}

func contextualCause(ctx context.Context, cause error) error {
	if contextCause := context.Cause(ctx); contextCause != nil {
		return contextCause
	}
	deadline, bounded := ctx.Deadline()
	networkError, timedOut := errors.AsType[net.Error](cause)
	if bounded &&
		!time.Now().Before(deadline) &&
		timedOut &&
		networkError != nil &&
		networkError.Timeout() {
		// A connection deadline and its context timer expire at the same
		// instant. The network read can win that race by a few scheduler
		// ticks, before context.Cause observes the configured timeout.
		return context.DeadlineExceeded
	}
	return cause
}

func classifyFailure(err error) (int, bool) {
	if protocolError, ok := errors.AsType[*textproto.Error](err); ok {
		return protocolError.Code, protocolError.Code >= 400 && protocolError.Code < 500
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return 0, false
	}
	if networkError, ok := errors.AsType[net.Error](err); ok {
		return 0, networkError != nil
	}
	return 0, errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

func observeAttempt(
	config normalizedConfig,
	ctx context.Context,
	messageID string,
	observation retry.Observation,
) {
	if config.observer == nil {
		return
	}
	result := Observation{
		MessageID:   messageID,
		Attempt:     observation.Attempt.Number,
		MaxAttempts: observation.Attempt.Max,
		Outcome:     OutcomeDelivered,
		NextBackoff: observation.NextBackoff,
		Duration:    observation.Duration,
	}
	if observation.Err != nil {
		result.Outcome = OutcomeFailed
		if errors.Is(observation.Err, context.Canceled) ||
			errors.Is(observation.Err, context.DeadlineExceeded) {
			result.Outcome = OutcomeCanceled
		}
		if delivery, ok := errors.AsType[*DeliveryError](observation.Err); ok {
			result.Stage = delivery.stage
			result.Code = delivery.code
			result.Temporary = delivery.temporary
			if delivery.retrySafe &&
				delivery.temporary &&
				observation.Attempt.Number < observation.Attempt.Max &&
				context.Cause(ctx) == nil {
				result.Outcome = OutcomeRetrying
			}
		}
	}
	config.observer(ctx, result)
}
