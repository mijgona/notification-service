//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/mijgona/notification-service/internal/channel"
)

// US3: permanent failures fast-fail to `failed` with a single permanent_error
// attempt and no retries (FR-010). Two cases: an unknown channel (service-level)
// and a channel that reports the failure as permanent.
func TestPermanentFailure(t *testing.T) {
	cases := []struct {
		name     string
		channels map[string]channel.Channel
	}{
		// "email" is intentionally NOT registered → unknown channel.
		{"unknown_channel", map[string]channel.Channel{}},
		// "email" registered but always reports a permanent failure.
		{"channel_permanent", map[string]channel.Channel{"email": permanentChannel{name: "email"}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newSUT(t, testEnv, tc.channels)
			defer s.stop()

			id, _, _ := postNotification(t, s.apiBaseURL, map[string]any{
				"idempotency_key": "perm-" + tc.name,
				"channel":         "email",
				"recipient":       "perm@example.com",
				"subject":         "s",
				"body":            "b",
			})

			waitForStatus(t, s.apiBaseURL, id, "failed", 30*time.Second)

			atts := attemptsFor(t, testEnv, id)
			if len(atts) != 1 {
				t.Fatalf("attempts = %+v, want exactly one (no retries)", atts)
			}
			if atts[0].Status != "permanent_error" {
				t.Fatalf("attempt status = %q, want permanent_error", atts[0].Status)
			}
			if atts[0].Error == "" {
				t.Fatal("permanent_error attempt must carry a reason")
			}
		})
	}
}
