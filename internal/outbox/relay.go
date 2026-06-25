// Package outbox implements the relay half of the transactional outbox
// pattern: it claims pending rows and turns each one into a Temporal
// workflow start.
package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"

	"github.com/mijgona/notification-service/internal/storage"
	wf "github.com/mijgona/notification-service/internal/workflow"
)

type Relay struct {
	outbox    *storage.Outbox
	temporal  client.Client
	taskQueue string
	interval  time.Duration
	batchSize int
	log       *slog.Logger
}

func New(outbox *storage.Outbox, tc client.Client, taskQueue string, interval time.Duration, batchSize int, log *slog.Logger) *Relay {
	return &Relay{
		outbox:    outbox,
		temporal:  tc,
		taskQueue: taskQueue,
		interval:  interval,
		batchSize: batchSize,
		log:       log,
	}
}

// Run polls the outbox until ctx is cancelled.
func (r *Relay) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := r.drain(ctx); err != nil {
				r.log.Error("relay tick", "error", err)
			}
		}
	}
}

// drain claims a locked batch, starts one DeliveryWorkflow per row and
// marks the rows processed in the same transaction. WorkflowID equals the
// notification ID, so if we crash after starting workflows but before
// committing, the retried start is rejected by Temporal as a duplicate —
// delivery stays effectively exactly-once.
func (r *Relay) drain(ctx context.Context) error {
	tx, batch, err := r.outbox.Claim(ctx, r.batchSize)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if len(batch) == 0 {
		return nil
	}

	processed := make([]int64, 0, len(batch))
	for _, row := range batch {
		_, err := r.temporal.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
			ID:        "delivery-" + row.NotificationID,
			TaskQueue: r.taskQueue,
		}, wf.DeliveryWorkflow, wf.DeliveryInput{NotificationID: row.NotificationID})

		if err != nil && !isAlreadyStarted(err) {
			r.log.Error("start workflow", "notification_id", row.NotificationID, "error", err)
			continue // leave the row unprocessed; the next tick retries it
		}
		processed = append(processed, row.ID)
	}

	if err := r.outbox.MarkProcessed(ctx, tx, processed); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit outbox batch: %w", err)
	}
	r.log.Info("outbox batch processed", "count", len(processed))
	return nil
}

func isAlreadyStarted(err error) bool {
	var already *serviceerror.WorkflowExecutionAlreadyStarted
	return errors.As(err, &already)
}
