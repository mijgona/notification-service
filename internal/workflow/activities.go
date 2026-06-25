package workflow

import (
	"context"
	"errors"
	"fmt"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"

	"github.com/mijgona/notification-service/internal/channel"
	"github.com/mijgona/notification-service/internal/storage"
)

// ErrTypeBadRequest marks errors that will never succeed on retry
// (unknown channel, invalid recipient). It is wired into DeliveryWorkflow's
// RetryPolicy.NonRetryableErrorTypes (see workflow.go), so wrapping a failure
// with temporal.NewNonRetryableApplicationError(msg, ErrTypeBadRequest, err)
// stops the retry loop immediately and drives the notification to failed.
const ErrTypeBadRequest = "BadRequest"

// delivery_attempts.status values. permanent_error marks a fast-failed,
// non-retryable attempt and is distinguishable from a transient error so an
// operator can tell the two apart (see migration 0002).
const (
	attemptSuccess        = "success"
	attemptError          = "error"
	attemptPermanentError = "permanent_error"
)

// notificationStore and attemptStore are the slice of storage that the Send
// activity needs. Depending on interfaces (satisfied by *storage.Notifications
// and *storage.Attempts) keeps the activity unit-testable without a database.
type notificationStore interface {
	GetByID(ctx context.Context, id string) (storage.Notification, error)
	UpdateStatus(ctx context.Context, id, status string) error
}

type attemptStore interface {
	Record(ctx context.Context, notificationID string, attemptNo int32, status, errMsg string) error
}

type Activities struct {
	Notifications notificationStore
	Attempts      attemptStore
	Channels      map[string]channel.Channel
}

// Send loads the notification, pushes it through its channel and records
// the attempt. Returning an error hands control back to Temporal's retry
// policy; returning a non-retryable ErrTypeBadRequest error stops it. Attempt
// numbers come from activity.GetInfo, so the delivery_attempts table matches
// the workflow history one to one.
func (a *Activities) Send(ctx context.Context, in DeliveryInput) error {
	attemptNo := activity.GetInfo(ctx).Attempt

	n, err := a.Notifications.GetByID(ctx, in.NotificationID)
	if err != nil {
		return fmt.Errorf("load notification %s: %w", in.NotificationID, err)
	}
	if n.Status == storage.StatusDelivered {
		return nil // already delivered: replay safety, nothing to do
	}
	if attemptNo == 1 {
		if err := a.Notifications.UpdateStatus(ctx, n.ID, storage.StatusProcessing); err != nil {
			return err
		}
	}

	ch, ok := a.Channels[n.Channel]
	if !ok {
		// An unknown channel can never succeed on retry: record the failed
		// attempt and stop the retry loop immediately.
		reason := fmt.Sprintf("unknown channel %q", n.Channel)
		return a.failPermanently(ctx, n.ID, attemptNo, reason, errors.New(reason))
	}

	sendErr := ch.Send(ctx, channel.Message{
		Recipient: n.Recipient,
		Subject:   n.Subject,
		Body:      n.Body,
	})
	if sendErr != nil {
		if channel.IsPermanent(sendErr) {
			// The channel reported a failure that retrying cannot fix.
			return a.failPermanently(ctx, n.ID, attemptNo, sendErr.Error(), sendErr)
		}
		if rerr := a.Attempts.Record(ctx, n.ID, attemptNo, attemptError, sendErr.Error()); rerr != nil {
			return fmt.Errorf("record failed attempt: %w (send error: %s)", rerr, sendErr)
		}
		return fmt.Errorf("send via %s: %w", n.Channel, sendErr)
	}

	if err := a.Attempts.Record(ctx, n.ID, attemptNo, attemptSuccess, ""); err != nil {
		return fmt.Errorf("record successful attempt: %w", err)
	}
	return a.Notifications.UpdateStatus(ctx, n.ID, storage.StatusDelivered)
}

// failPermanently records a permanent_error attempt and returns a
// non-retryable error so DeliveryWorkflow drives the notification to failed
// after a single attempt.
func (a *Activities) failPermanently(ctx context.Context, id string, attemptNo int32, reason string, cause error) error {
	if rerr := a.Attempts.Record(ctx, id, attemptNo, attemptPermanentError, reason); rerr != nil {
		return fmt.Errorf("record permanent attempt: %w (%s)", rerr, reason)
	}
	return temporal.NewNonRetryableApplicationError(reason, ErrTypeBadRequest, cause)
}

func (a *Activities) MarkFailed(ctx context.Context, in DeliveryInput) error {
	return a.Notifications.UpdateStatus(ctx, in.NotificationID, storage.StatusFailed)
}
