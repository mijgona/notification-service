package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Telegram struct {
	token  string
	client *http.Client
}

func NewTelegram(token string) *Telegram {
	return &Telegram{
		token:  token,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *Telegram) Name() string { return "telegram" }

// Send delivers the message via the Telegram Bot API. Recipient is a chat ID.
func (t *Telegram) Send(ctx context.Context, msg Message) error {
	if t.token == "" {
		return fmt.Errorf("telegram: bot token is not configured")
	}

	payload, err := json.Marshal(map[string]string{
		"chat_id": msg.Recipient,
		"text":    msg.Body,
	})
	if err != nil {
		return fmt.Errorf("telegram: marshal payload: %w", err)
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("telegram: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Surface Telegram's error description. 4xx responses are client
		// errors (bad chat_id, bot blocked, …) that retrying cannot fix, so
		// classify them as permanent; 5xx and transport errors stay transient.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		desc := string(bytes.TrimSpace(body))
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return Permanent(
				fmt.Sprintf("telegram: status %d: %s", resp.StatusCode, desc),
				nil,
			)
		}
		return fmt.Errorf("telegram: unexpected status %d: %s", resp.StatusCode, desc)
	}
	return nil
}
