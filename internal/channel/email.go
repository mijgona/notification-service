package channel

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
)

type Email struct {
	addr string
	from string
}

func NewEmail(addr, from string) *Email {
	return &Email{addr: addr, from: from}
}

func (e *Email) Name() string { return "email" }

// Send delivers the message over plain SMTP. In local development the
// docker-compose Mailpit instance plays the role of the mail server.
func (e *Email) Send(ctx context.Context, msg Message) error {
	_ = ctx // net/smtp has no context support; fine for the demo channel.

	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", e.from)
	fmt.Fprintf(&b, "To: %s\r\n", msg.Recipient)
	fmt.Fprintf(&b, "Subject: %s\r\n", msg.Subject)
	b.WriteString("\r\n")
	b.WriteString(msg.Body)

	if err := smtp.SendMail(e.addr, nil, e.from, []string{msg.Recipient}, []byte(b.String())); err != nil {
		return fmt.Errorf("email: send via %s: %w", e.addr, err)
	}
	return nil
}
