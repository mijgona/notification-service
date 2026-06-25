// Package channel defines the delivery channel abstraction. Adding a new
// channel (SMS, push, webhook) means implementing this one interface and
// registering it in cmd/worker — nothing else in the service changes.
package channel

import (
	"context"
	"errors"
)

type Message struct {
	Recipient string
	Subject   string
	Body      string
}

type Channel interface {
	Name() string
	Send(ctx context.Context, msg Message) error
}

// PermanentError marks a delivery failure that retrying can never resolve
// (e.g. a malformed or blocked recipient). A channel returns it to opt into
// fast-fail: the worker turns it into a non-retryable Temporal error so the
// notification goes straight to failed instead of exhausting the retry budget.
// Any error that is not a PermanentError is treated as transient (retryable).
type PermanentError struct {
	Reason string // human-readable; recorded in delivery_attempts.error
	Err    error  // underlying cause, if any
}

func (e *PermanentError) Error() string {
	if e.Err != nil {
		return e.Reason + ": " + e.Err.Error()
	}
	return e.Reason
}

func (e *PermanentError) Unwrap() error { return e.Err }

// Permanent wraps err as a permanent failure with the given reason.
func Permanent(reason string, err error) error {
	return &PermanentError{Reason: reason, Err: err}
}

// IsPermanent reports whether err, or anything it wraps, is a PermanentError.
// It uses errors.As, so it sees through fmt.Errorf("...: %w", err) wrapping.
func IsPermanent(err error) bool {
	var pe *PermanentError
	return errors.As(err, &pe)
}
