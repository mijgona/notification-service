package workflow

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/activity"

	"github.com/mijgona/notification-service/internal/channel"
	"github.com/mijgona/notification-service/internal/storage"
)

// ErrTypeBadRequest marks errors that will never succeed on retry
// (unknown channel, invalid recipient). Wrap them with
// temporal.NewNonRetryableApplicationError(msg, ErrTypeBadRequest, err)
// to stop the retry loop immediately.
const ErrTypeBadRequest = "BadRequest"

type Activities struct {
	Notifications *storage.Notifications
	Attempts      *storage.Attempts
	Channels      map[string]channel.Channel
}

// Send loads the notification, pushes it through its channel and records
// the attempt. Returning an error hands control back to Temporal's retry
// policy. Attempt numbers come from activity.GetInfo, so the
// delivery_attempts table matches the workflow history one to one.
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
		// TODO(mijgona): wrap with temporal.NewNonRetryableApplicationError
		// and ErrTypeBadRequest — retrying an unknown channel can't succeed.
		return fmt.Errorf("unknown channel %q", n.Channel)
	}

	sendErr := ch.Send(ctx, channel.Message{
		Recipient: n.Recipient,
		Subject:   n.Subject,
		Body:      n.Body,
	})
	if sendErr != nil {
		if rerr := a.Attempts.Record(ctx, n.ID, attemptNo, "error", sendErr.Error()); rerr != nil {
			return fmt.Errorf("record failed attempt: %w (send error: %s)", rerr, sendErr)
		}
		return fmt.Errorf("send via %s: %w", n.Channel, sendErr)
	}

	if err := a.Attempts.Record(ctx, n.ID, attemptNo, "success", ""); err != nil {
		return fmt.Errorf("record successful attempt: %w", err)
	}
	return a.Notifications.UpdateStatus(ctx, n.ID, storage.StatusDelivered)
}

func (a *Activities) MarkFailed(ctx context.Context, in DeliveryInput) error {
	return a.Notifications.UpdateStatus(ctx, in.NotificationID, storage.StatusFailed)
}
