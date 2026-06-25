package workflow

import (
	"context"
	"errors"
	"testing"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	"github.com/mijgona/notification-service/internal/channel"
	"github.com/mijgona/notification-service/internal/storage"
)

// --- shared fakes (used by activities_test.go and workflow_test.go) ---

type fakeNotifications struct {
	n        storage.Notification
	getErr   error
	statuses []string
}

func (f *fakeNotifications) GetByID(_ context.Context, _ string) (storage.Notification, error) {
	return f.n, f.getErr
}

func (f *fakeNotifications) UpdateStatus(_ context.Context, _, status string) error {
	f.statuses = append(f.statuses, status)
	return nil
}

type recordedAttempt struct {
	attemptNo int32
	status    string
	errMsg    string
}

type fakeAttempts struct {
	records []recordedAttempt
}

func (f *fakeAttempts) Record(_ context.Context, _ string, attemptNo int32, status, errMsg string) error {
	f.records = append(f.records, recordedAttempt{attemptNo, status, errMsg})
	return nil
}

type fakeChannel struct {
	name    string
	sendErr error
}

func (c fakeChannel) Name() string                                { return c.name }
func (c fakeChannel) Send(context.Context, channel.Message) error { return c.sendErr }

func newNotification(ch string) storage.Notification {
	return storage.Notification{
		ID:        "11111111-1111-1111-1111-111111111111",
		Channel:   ch,
		Recipient: "42",
		Body:      "hi",
		Status:    storage.StatusPending,
	}
}

// assertNonRetryableBadRequest fails the test unless err is a non-retryable
// ApplicationError of type ErrTypeBadRequest.
func assertNonRetryableBadRequest(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	var appErr *temporal.ApplicationError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *temporal.ApplicationError, got %T: %v", err, err)
	}
	if appErr.Type() != ErrTypeBadRequest {
		t.Fatalf("error type = %q, want %q", appErr.Type(), ErrTypeBadRequest)
	}
	if !appErr.NonRetryable() {
		t.Fatal("expected a non-retryable error")
	}
}

// --- T005: unknown channel fails fast and records permanent_error ---

func TestSend_UnknownChannel_FailsPermanently(t *testing.T) {
	fn := &fakeNotifications{n: newNotification("sms")} // "sms" not registered
	fa := &fakeAttempts{}
	acts := &Activities{Notifications: fn, Attempts: fa, Channels: map[string]channel.Channel{}}

	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestActivityEnvironment()
	env.RegisterActivity(acts)

	_, err := env.ExecuteActivity(acts.Send, DeliveryInput{NotificationID: fn.n.ID})

	assertNonRetryableBadRequest(t, err)
	if len(fa.records) != 1 {
		t.Fatalf("expected exactly 1 attempt recorded, got %d", len(fa.records))
	}
	if got := fa.records[0]; got.status != attemptPermanentError || got.errMsg == "" {
		t.Fatalf("attempt = %+v, want status %q with non-empty reason", got, attemptPermanentError)
	}
}

// --- T009: channel-reported permanent failure fails fast ---

func TestSend_PermanentChannelError_FailsPermanently(t *testing.T) {
	fn := &fakeNotifications{n: newNotification("telegram")}
	fa := &fakeAttempts{}
	acts := &Activities{
		Notifications: fn,
		Attempts:      fa,
		Channels: map[string]channel.Channel{
			"telegram": fakeChannel{name: "telegram", sendErr: channel.Permanent("telegram: status 400: bad chat", nil)},
		},
	}

	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestActivityEnvironment()
	env.RegisterActivity(acts)

	_, err := env.ExecuteActivity(acts.Send, DeliveryInput{NotificationID: fn.n.ID})

	assertNonRetryableBadRequest(t, err)
	if len(fa.records) != 1 || fa.records[0].status != attemptPermanentError {
		t.Fatalf("expected 1 permanent_error attempt, got %+v", fa.records)
	}
}

// --- T009/T014: transient channel error is retryable and recorded as error ---

func TestSend_TransientChannelError_IsRetryable(t *testing.T) {
	fn := &fakeNotifications{n: newNotification("telegram")}
	fa := &fakeAttempts{}
	acts := &Activities{
		Notifications: fn,
		Attempts:      fa,
		Channels: map[string]channel.Channel{
			"telegram": fakeChannel{name: "telegram", sendErr: errors.New("connection reset")},
		},
	}

	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestActivityEnvironment()
	env.RegisterActivity(acts)

	_, err := env.ExecuteActivity(acts.Send, DeliveryInput{NotificationID: fn.n.ID})

	if err == nil {
		t.Fatal("expected an error")
	}
	// A transient error must NOT be a non-retryable BadRequest.
	var appErr *temporal.ApplicationError
	if errors.As(err, &appErr) && appErr.Type() == ErrTypeBadRequest {
		t.Fatalf("transient error must not be classified as %q", ErrTypeBadRequest)
	}
	if len(fa.records) != 1 || fa.records[0].status != attemptError {
		t.Fatalf("expected 1 'error' attempt, got %+v", fa.records)
	}
}

// --- successful delivery records success and marks delivered ---

func TestSend_Success_MarksDelivered(t *testing.T) {
	fn := &fakeNotifications{n: newNotification("telegram")}
	fa := &fakeAttempts{}
	acts := &Activities{
		Notifications: fn,
		Attempts:      fa,
		Channels:      map[string]channel.Channel{"telegram": fakeChannel{name: "telegram"}},
	}

	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestActivityEnvironment()
	env.RegisterActivity(acts)

	if _, err := env.ExecuteActivity(acts.Send, DeliveryInput{NotificationID: fn.n.ID}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fa.records) != 1 || fa.records[0].status != attemptSuccess {
		t.Fatalf("expected 1 success attempt, got %+v", fa.records)
	}
	if len(fn.statuses) == 0 || fn.statuses[len(fn.statuses)-1] != storage.StatusDelivered {
		t.Fatalf("expected final status delivered, got %v", fn.statuses)
	}
}
