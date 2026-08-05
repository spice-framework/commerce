// Package smtp provides a secure, instance-owned SMTP transport for Spice
// mail messages.
package smtp

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/StevenBuglione/spice/retry"
)

const (
	defaultTimeout     = 10 * time.Second
	maxTimeout         = 2 * time.Minute
	maxAddressBytes    = 512
	maxIdentityBytes   = 255
	maxCredentialBytes = 4 << 10
	maxAttempts        = 10
)

// TLSMode identifies the required SMTP transport-security negotiation.
type TLSMode string

const (
	// TLSModeStartTLS connects over TCP and requires the server to advertise
	// and complete STARTTLS before authentication or envelope commands.
	TLSModeStartTLS TLSMode = "starttls"
	// TLSModeImplicitTLS establishes TLS before reading the SMTP greeting.
	TLSModeImplicitTLS TLSMode = "implicit-tls"
)

// Config defines one immutable SMTP sender. New validates and copies the
// configuration without opening a network connection.
type Config struct {
	Address        string
	ServerName     string
	ClientName     string
	Mode           TLSMode
	Username       string
	Password       string
	TLSConfig      *tls.Config
	Timeout        time.Duration
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Multiplier     uint32
	Wait           retry.Waiter
	Observer       Observer
}

type normalizedConfig struct {
	address        string
	serverName     string
	clientName     string
	mode           TLSMode
	username       string
	password       string
	tlsConfig      *tls.Config
	timeout        time.Duration
	maxAttempts    int
	initialBackoff time.Duration
	maxBackoff     time.Duration
	multiplier     uint32
	wait           retry.Waiter
	observer       Observer
}

func normalizeConfig(config Config) (normalizedConfig, error) {
	host, addressErr := validateAddress(config.Address)
	if addressErr != nil {
		return normalizedConfig{}, addressErr
	}
	serverName, serverNameErr := normalizeServerName(config.ServerName, host)
	if serverNameErr != nil {
		return normalizedConfig{}, serverNameErr
	}
	clientName, mode, clientErr := normalizeClient(config.ClientName, config.Mode)
	if clientErr != nil {
		return normalizedConfig{}, clientErr
	}
	if authErr := validateAuthentication(config.Username, config.Password); authErr != nil {
		return normalizedConfig{}, authErr
	}
	timeout, attempts, retryErr := normalizeRetry(config)
	if retryErr != nil {
		return normalizedConfig{}, retryErr
	}
	tlsConfig, tlsErr := secureTLSConfig(config.TLSConfig, serverName)
	if tlsErr != nil {
		return normalizedConfig{}, tlsErr
	}
	return normalizedConfig{
		address:        config.Address,
		serverName:     serverName,
		clientName:     clientName,
		mode:           mode,
		username:       config.Username,
		password:       config.Password,
		tlsConfig:      tlsConfig,
		timeout:        timeout,
		maxAttempts:    attempts,
		initialBackoff: config.InitialBackoff,
		maxBackoff:     config.MaxBackoff,
		multiplier:     config.Multiplier,
		wait:           config.Wait,
		observer:       config.Observer,
	}, nil
}

func normalizeClient(clientName string, mode TLSMode) (string, TLSMode, error) {
	if clientName == "" {
		clientName = "localhost"
	}
	if identityErr := validateIdentity("client name", clientName); identityErr != nil {
		return "", "", identityErr
	}
	if mode == "" {
		mode = TLSModeStartTLS
	}
	if mode != TLSModeStartTLS && mode != TLSModeImplicitTLS {
		return "", "", fmt.Errorf("configure SMTP sender: unsupported TLS mode %q", mode)
	}
	return clientName, mode, nil
}

func validateAuthentication(username, password string) error {
	if (username == "") != (password == "") {
		return errors.New(
			"configure SMTP sender: username and password must be supplied together",
		)
	}
	if len(username) > maxCredentialBytes || len(password) > maxCredentialBytes {
		return fmt.Errorf(
			"configure SMTP sender: credentials must not exceed %d bytes",
			maxCredentialBytes,
		)
	}
	return nil
}

func normalizeRetry(config Config) (time.Duration, int, error) {
	timeout := config.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if timeout < 0 || timeout > maxTimeout {
		return 0, 0, fmt.Errorf(
			"configure SMTP sender: timeout must be between 1ns and %s",
			maxTimeout,
		)
	}
	attempts := config.MaxAttempts
	if attempts == 0 {
		attempts = 1
	}
	if attempts < 1 || attempts > maxAttempts {
		return 0, 0, fmt.Errorf(
			"configure SMTP sender: max attempts must be between 1 and %d",
			maxAttempts,
		)
	}
	if config.InitialBackoff < 0 ||
		config.MaxBackoff < config.InitialBackoff ||
		config.Multiplier == 1 {
		return 0, 0, errors.New(
			"configure SMTP sender: retry backoff is invalid",
		)
	}
	return timeout, attempts, nil
}

func validateAddress(address string) (string, error) {
	if address == "" || len(address) > maxAddressBytes {
		return "", fmt.Errorf(
			"configure SMTP sender: address must contain 1 to %d bytes",
			maxAddressBytes,
		)
	}
	if strings.TrimSpace(address) != address ||
		strings.ContainsAny(address, "\x00\r\n\t /") {
		return "", errors.New(
			"configure SMTP sender: address must be an exact host:port",
		)
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil || host == "" {
		return "", errors.New(
			"configure SMTP sender: address must be an exact host:port",
		)
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || port == 0 {
		return "", errors.New(
			"configure SMTP sender: address port must be between 1 and 65535",
		)
	}
	return host, nil
}

func normalizeServerName(configured, host string) (string, error) {
	serverName := configured
	if serverName == "" {
		serverName = host
	}
	if err := validateIdentity("server name", serverName); err != nil {
		return "", err
	}
	return serverName, nil
}

func validateIdentity(label, value string) error {
	if value == "" ||
		len(value) > maxIdentityBytes ||
		strings.TrimSpace(value) != value ||
		strings.ContainsAny(value, "\x00\r\n\t /") {
		return fmt.Errorf(
			"configure SMTP sender: %s must be a bounded host identity",
			label,
		)
	}
	return nil
}

func secureTLSConfig(config *tls.Config, serverName string) (*tls.Config, error) {
	var copied *tls.Config
	if config == nil {
		copied = &tls.Config{MinVersion: tls.VersionTLS12} //nolint:gosec // TLS 1.2 is the explicit compatibility floor.
	} else {
		copied = config.Clone()
		if copied.RootCAs != nil {
			copied.RootCAs = copied.RootCAs.Clone()
		}
		if copied.ClientCAs != nil {
			copied.ClientCAs = copied.ClientCAs.Clone()
		}
	}
	if copied.InsecureSkipVerify {
		return nil, errors.New(
			"configure SMTP sender: TLS certificate verification cannot be disabled",
		)
	}
	if copied.MinVersion == 0 {
		copied.MinVersion = tls.VersionTLS12
	}
	if copied.MinVersion < tls.VersionTLS12 {
		return nil, errors.New(
			"configure SMTP sender: TLS minimum version must be at least 1.2",
		)
	}
	if copied.MaxVersion != 0 && copied.MaxVersion < copied.MinVersion {
		return nil, errors.New(
			"configure SMTP sender: TLS maximum version is below its minimum",
		)
	}
	if copied.Renegotiation != tls.RenegotiateNever {
		return nil, errors.New(
			"configure SMTP sender: TLS renegotiation must remain disabled",
		)
	}
	if copied.ServerName != "" && copied.ServerName != serverName {
		return nil, errors.New(
			"configure SMTP sender: TLS server name conflicts with the configured server name",
		)
	}
	copied.ServerName = serverName
	return copied, nil
}
