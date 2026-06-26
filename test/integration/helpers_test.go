//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/mijgona/notification-service/internal/channel"
	"github.com/mijgona/notification-service/internal/storage"
)

// postNotification posts an intake request and returns the created id, the
// HTTP status, and whether the response marked it a duplicate.
func postNotification(t *testing.T, apiBaseURL string, body map[string]any) (id string, status int, duplicate bool) {
	t.Helper()
	buf, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	resp, err := http.Post(apiBaseURL+"/v1/notifications", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("POST /v1/notifications: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var out struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		Duplicate bool   `json:"duplicate"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	return out.ID, resp.StatusCode, out.Duplicate
}

// waitForStatus polls GET /v1/notifications/{id} until the notification status
// equals want or the timeout elapses (fails clearly on timeout — never hangs).
func waitForStatus(t *testing.T, apiBaseURL, id, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		last = getStatus(t, apiBaseURL, id)
		if last == want {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("notification %s: status %q not reached within %s (last seen %q)", id, want, timeout, last)
}

func getStatus(t *testing.T, apiBaseURL, id string) string {
	t.Helper()
	resp, err := http.Get(apiBaseURL + "/v1/notifications/" + id)
	if err != nil {
		t.Fatalf("GET notification: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var out struct {
		Notification storage.Notification `json:"notification"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	return out.Notification.Status
}

// attemptsFor reads delivery_attempts rows for a notification directly from Postgres.
func attemptsFor(t *testing.T, _ Env, id string) []storage.DeliveryAttempt {
	t.Helper()
	rows, err := testPool.Query(context.Background(),
		`SELECT id, notification_id, attempt_no, status, error, created_at
		   FROM delivery_attempts WHERE notification_id = $1 ORDER BY id`, id)
	if err != nil {
		t.Fatalf("query attempts: %v", err)
	}
	defer rows.Close()

	var out []storage.DeliveryAttempt
	for rows.Next() {
		var a storage.DeliveryAttempt
		if err := rows.Scan(&a.ID, &a.NotificationID, &a.AttemptNo, &a.Status, &a.Error, &a.CreatedAt); err != nil {
			t.Fatalf("scan attempt: %v", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("attempts rows: %v", err)
	}
	return out
}

// MailpitMessage is the minimal shape we assert from the Mailpit search API.
type MailpitMessage struct {
	To      []struct{ Address string } `json:"To"`
	Subject string                     `json:"Subject"`
}

// mailpitMessagesTo queries the Mailpit HTTP API for messages addressed to recipient.
func mailpitMessagesTo(t *testing.T, env Env, recipient string) []MailpitMessage {
	t.Helper()
	u := fmt.Sprintf("%s/api/v1/search?query=%s", env.MailpitAPIBaseURL,
		url.QueryEscape("to:"+recipient))
	resp, err := http.Get(u)
	if err != nil {
		t.Fatalf("mailpit search: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("mailpit search status %d: %s", resp.StatusCode, body)
	}
	var out struct {
		Messages []MailpitMessage `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode mailpit search: %v", err)
	}
	return out.Messages
}

// waitForMailpit polls Mailpit until at least one message to recipient appears.
func waitForMailpit(t *testing.T, env Env, recipient string, timeout time.Duration) []MailpitMessage {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if msgs := mailpitMessagesTo(t, env, recipient); len(msgs) > 0 {
			return msgs
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("no Mailpit message to %s within %s", recipient, timeout)
	return nil
}

// permanentChannel is a test channel that reports every send as a permanent
// failure — the same surface a real channel (e.g. telegram on a 4xx) exposes
// via channel.Permanent. Used to drive the channel-permanent path (FR-010)
// without depending on the real Bot API.
type permanentChannel struct{ name string }

func (c permanentChannel) Name() string { return c.name }
func (c permanentChannel) Send(context.Context, channel.Message) error {
	return channel.Permanent(fmt.Sprintf("%s: status 400: permanent test failure", c.name), nil)
}
