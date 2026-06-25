package api

import (
	"errors"
	"fmt"
	"strings"
)

type CreateRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	Channel        string `json:"channel"`
	Recipient      string `json:"recipient"`
	Subject        string `json:"subject"`
	Body           string `json:"body"`
}

var validChannels = map[string]bool{
	"telegram": true,
	"email":    true,
}

func (r CreateRequest) Validate() error {
	var problems []string
	if strings.TrimSpace(r.IdempotencyKey) == "" {
		problems = append(problems, "idempotency_key is required")
	}
	if !validChannels[r.Channel] {
		problems = append(problems, fmt.Sprintf("channel must be one of: telegram, email (got %q)", r.Channel))
	}
	if strings.TrimSpace(r.Recipient) == "" {
		problems = append(problems, "recipient is required")
	}
	if strings.TrimSpace(r.Body) == "" {
		problems = append(problems, "body is required")
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}
