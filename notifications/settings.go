package notifications

import "time"

// @import { ConfigurationProperties } from "github.com/spice-framework/spice/annotation/core"

// Settings selects an instance-owned mail transport and immutable envelope.
// Test transport is the safe zero-network default for the developer workflow.
//
// @ConfigurationProperties(prefix="commerce.mail")
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
