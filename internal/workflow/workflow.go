package workflow

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

type DeliveryInput struct {
	NotificationID string
}

// DeliveryWorkflow drives one notification to a terminal state.
// Temporal owns the retry schedule: the Send activity is retried with
// exponential backoff, and every attempt is visible both in the workflow
// history (Temporal UI) and in the delivery_attempts table.
func DeliveryWorkflow(ctx workflow.Context, in DeliveryInput) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:        2 * time.Second,
			BackoffCoefficient:     2.0,
			MaximumInterval:        5 * time.Minute,
			MaximumAttempts:        8,
			NonRetryableErrorTypes: []string{ErrTypeBadRequest},
		},
	})

	var a *Activities
	if err := workflow.ExecuteActivity(ctx, a.Send, in).Get(ctx, nil); err != nil {
		// Out of retries, or a non-retryable error: record the terminal
		// state. MarkFailed gets its own short retry policy so that a
		// database blip cannot lose the final status.
		failCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
			StartToCloseTimeout: 10 * time.Second,
			RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 5},
		})
		if ferr := workflow.ExecuteActivity(failCtx, a.MarkFailed, in).Get(failCtx, nil); ferr != nil {
			return ferr
		}
		return err
	}
	return nil
}
