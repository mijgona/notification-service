package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const selectNotification = `
	SELECT id, idempotency_key, channel, recipient, subject, body, status, created_at
	FROM notifications`

type Notifications struct {
	pool *pgxpool.Pool
}

func NewNotifications(pool *pgxpool.Pool) *Notifications {
	return &Notifications{pool: pool}
}

// CreateWithOutbox inserts the notification and its outbox row in a single
// transaction — this is the transactional outbox pattern. If the
// idempotency key already exists, the stored notification is returned and
// created is false: no new outbox row is written, so a duplicate request
// can never produce a second delivery.
func (s *Notifications) CreateWithOutbox(ctx context.Context, n Notification) (Notification, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Notification{}, false, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = tx.QueryRow(ctx, `
		INSERT INTO notifications (idempotency_key, channel, recipient, subject, body)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id, status, created_at`,
		n.IdempotencyKey, n.Channel, n.Recipient, n.Subject, n.Body,
	).Scan(&n.ID, &n.Status, &n.CreatedAt)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Conflict: this idempotency key was accepted earlier.
		existing, gerr := s.getByKey(ctx, n.IdempotencyKey)
		if gerr != nil {
			return Notification{}, false, gerr
		}
		return existing, false, nil
	case err != nil:
		return Notification{}, false, fmt.Errorf("insert notification: %w", err)
	}

	if _, err := tx.Exec(ctx, `INSERT INTO outbox (notification_id) VALUES ($1)`, n.ID); err != nil {
		return Notification{}, false, fmt.Errorf("insert outbox row: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Notification{}, false, fmt.Errorf("commit: %w", err)
	}
	return n, true, nil
}

func (s *Notifications) GetByID(ctx context.Context, id string) (Notification, error) {
	return s.scanOne(ctx, selectNotification+` WHERE id = $1`, id)
}

func (s *Notifications) getByKey(ctx context.Context, key string) (Notification, error) {
	return s.scanOne(ctx, selectNotification+` WHERE idempotency_key = $1`, key)
}

func (s *Notifications) UpdateStatus(ctx context.Context, id, status string) error {
	if _, err := s.pool.Exec(ctx,
		`UPDATE notifications SET status = $2, updated_at = now() WHERE id = $1`,
		id, status); err != nil {
		return fmt.Errorf("update notification status: %w", err)
	}
	return nil
}

func (s *Notifications) scanOne(ctx context.Context, query string, arg any) (Notification, error) {
	var n Notification
	err := s.pool.QueryRow(ctx, query, arg).Scan(
		&n.ID, &n.IdempotencyKey, &n.Channel, &n.Recipient,
		&n.Subject, &n.Body, &n.Status, &n.CreatedAt,
	)
	if err != nil {
		return Notification{}, fmt.Errorf("select notification: %w", err)
	}
	return n, nil
}
