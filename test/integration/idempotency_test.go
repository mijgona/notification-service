//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// US3: intake idempotency — two requests with the same idempotency key create
// exactly one notification and exactly one delivery (FR-008).
func TestIntakeIdempotency(t *testing.T) {
	s := newSUT(t, testEnv, defaultChannels(testEnv))
	defer s.stop()

	const recipient = "idem@example.com"
	body := map[string]any{
		"idempotency_key": "idem-key-1",
		"channel":         "email",
		"recipient":       recipient,
		"subject":         "s",
		"body":            "b",
	}

	id1, st1, dup1 := postNotification(t, s.apiBaseURL, body)
	if st1 != http.StatusAccepted || dup1 {
		t.Fatalf("first POST: status=%d duplicate=%v", st1, dup1)
	}
	id2, st2, dup2 := postNotification(t, s.apiBaseURL, body)
	if st2 != http.StatusAccepted {
		t.Fatalf("second POST: status=%d", st2)
	}
	if id2 != id1 {
		t.Fatalf("duplicate intake produced a different id: %s vs %s", id1, id2)
	}
	if !dup2 {
		t.Fatal("second POST with same key should be marked duplicate")
	}

	if got := countNotifications(t, "idem-key-1"); got != 1 {
		t.Fatalf("notifications with key = %d, want 1", got)
	}

	waitForStatus(t, s.apiBaseURL, id1, "delivered", 30*time.Second)

	atts := attemptsFor(t, testEnv, id1)
	if len(atts) != 1 || atts[0].Status != "success" {
		t.Fatalf("attempts = %+v, want exactly one 'success' (one delivery)", atts)
	}
	if msgs := waitForMailpit(t, testEnv, recipient, 10*time.Second); len(msgs) != 1 {
		t.Fatalf("Mailpit messages = %d, want exactly 1 delivery", len(msgs))
	}
}

// US3: delivery idempotency — a duplicate outbox row for an already-delivered
// notification does not produce a second delivery. The relay starts the
// workflow again, but the Send activity no-ops because the notification is
// already delivered (Principle I/II).
func TestDeliveryIdempotency_DuplicateOutbox(t *testing.T) {
	s := newSUT(t, testEnv, defaultChannels(testEnv))
	defer s.stop()

	const recipient = "deliv-idem@example.com"
	id, _, _ := postNotification(t, s.apiBaseURL, map[string]any{
		"idempotency_key": "deliv-idem-1",
		"channel":         "email",
		"recipient":       recipient,
		"subject":         "s",
		"body":            "b",
	})
	waitForStatus(t, s.apiBaseURL, id, "delivered", 30*time.Second)
	waitForMailpit(t, testEnv, recipient, 10*time.Second)

	// Inject a second outbox row for the same notification (a relay that
	// double-enqueued). The relay will pick it up and start the workflow again.
	if _, err := testPool.Exec(context.Background(),
		`INSERT INTO outbox (notification_id) VALUES ($1)`, id); err != nil {
		t.Fatalf("inject duplicate outbox row: %v", err)
	}

	// Give the relay several ticks to process the duplicate.
	time.Sleep(2 * time.Second)

	atts := attemptsFor(t, testEnv, id)
	if len(atts) != 1 {
		t.Fatalf("attempts = %+v, want still exactly one (no second delivery)", atts)
	}
	if msgs := mailpitMessagesTo(t, testEnv, recipient); len(msgs) != 1 {
		t.Fatalf("Mailpit messages = %d, want still exactly 1 (no double delivery)", len(msgs))
	}
}

func countNotifications(t *testing.T, key string) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM notifications WHERE idempotency_key = $1`, key).Scan(&n); err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	return n
}
