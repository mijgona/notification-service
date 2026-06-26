//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"
)

// US1: full delivery path on real infrastructure — POST → relay → worker →
// delivered, with the message actually received by Mailpit (SC-002, FR-002).
func TestDeliveryE2E_Email(t *testing.T) {
	s := newSUT(t, testEnv, defaultChannels(testEnv))
	defer s.stop()

	const recipient = "e2e@example.com"
	id, status, dup := postNotification(t, s.apiBaseURL, map[string]any{
		"idempotency_key": "e2e-email-1",
		"channel":         "email",
		"recipient":       recipient,
		"subject":         "hello",
		"body":            "integration body",
	})
	if status != http.StatusAccepted {
		t.Fatalf("POST status = %d, want 202", status)
	}
	if dup {
		t.Fatal("unexpected duplicate on first POST")
	}

	waitForStatus(t, s.apiBaseURL, id, "delivered", 30*time.Second)

	atts := attemptsFor(t, testEnv, id)
	if len(atts) != 1 || atts[0].Status != "success" {
		t.Fatalf("attempts = %+v, want exactly one 'success'", atts)
	}

	msgs := waitForMailpit(t, testEnv, recipient, 10*time.Second)
	if len(msgs) != 1 {
		t.Fatalf("Mailpit messages to %s = %d, want 1", recipient, len(msgs))
	}
}
