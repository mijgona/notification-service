package workflow

import (
	"context"
	"errors"
	"testing"

	"go.temporal.io/sdk/testsuite"

	"github.com/mijgona/notification-service/internal/channel"
	"github.com/mijgona/notification-service/internal/storage"
)

// A non-retryable (permanent) failure drives the workflow to a single Send
// attempt, then MarkFailed, then a terminal workflow error.
func TestDeliveryWorkflow_PermanentFailure_SingleAttempt(t *testing.T) {
	fn := &fakeNotifications{n: newNotification("sms")} // unknown channel -> permanent
	fa := &fakeAttempts{}
	acts := &Activities{Notifications: fn, Attempts: fa, Channels: map[string]channel.Channel{}}

	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()
	env.RegisterActivity(acts)

	env.ExecuteWorkflow(DeliveryWorkflow, DeliveryInput{NotificationID: fn.n.ID})

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if env.GetWorkflowError() == nil {
		t.Fatal("expected a terminal workflow error for a permanent failure")
	}
	if len(fa.records) != 1 || fa.records[0].status != attemptPermanentError {
		t.Fatalf("expected exactly 1 permanent_error attempt (no retries), got %+v", fa.records)
	}
	if last := lastStatus(fn.statuses); last != storage.StatusFailed {
		t.Fatalf("expected terminal status %q, got %v", storage.StatusFailed, fn.statuses)
	}
}

// A transient failure is retried up to the workflow's MaximumAttempts (8)
// before the notification is marked failed.
func TestDeliveryWorkflow_TransientFailure_RetriesThenFails(t *testing.T) {
	fn := &fakeNotifications{n: newNotification("telegram")}
	fa := &fakeAttempts{}
	acts := &Activities{
		Notifications: fn,
		Attempts:      fa,
		Channels: map[string]channel.Channel{
			"telegram": alwaysTransientChannel{},
		},
	}

	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()
	env.RegisterActivity(acts)

	env.ExecuteWorkflow(DeliveryWorkflow, DeliveryInput{NotificationID: fn.n.ID})

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if env.GetWorkflowError() == nil {
		t.Fatal("expected a terminal workflow error after retries exhausted")
	}
	// Transient errors must be retried: more than one attempt, all 'error'.
	if len(fa.records) < 2 {
		t.Fatalf("expected the transient failure to be retried, got %d attempt(s)", len(fa.records))
	}
	for _, r := range fa.records {
		if r.status != attemptError {
			t.Fatalf("expected all transient attempts to be %q, got %+v", attemptError, fa.records)
		}
	}
	if last := lastStatus(fn.statuses); last != storage.StatusFailed {
		t.Fatalf("expected terminal status %q, got %v", storage.StatusFailed, fn.statuses)
	}
}

type alwaysTransientChannel struct{}

func (alwaysTransientChannel) Name() string { return "telegram" }
func (alwaysTransientChannel) Send(_ context.Context, _ channel.Message) error {
	return errors.New("temporary downstream failure")
}

func lastStatus(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[len(s)-1]
}
