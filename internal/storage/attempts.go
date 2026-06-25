package storage

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Attempts struct {
	pool *pgxpool.Pool
}

func NewAttempts(pool *pgxpool.Pool) *Attempts {
	return &Attempts{pool: pool}
}

func (s *Attempts) Record(ctx context.Context, notificationID string, attemptNo int32, status, errMsg string) error {
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO delivery_attempts (notification_id, attempt_no, status, error)
		VALUES ($1, $2, $3, $4)`,
		notificationID, attemptNo, status, errMsg); err != nil {
		return fmt.Errorf("insert delivery attempt: %w", err)
	}
	return nil
}

func (s *Attempts) ListByNotification(ctx context.Context, notificationID string) ([]DeliveryAttempt, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, notification_id, attempt_no, status, error, created_at
		FROM delivery_attempts
		WHERE notification_id = $1
		ORDER BY id`, notificationID)
	if err != nil {
		return nil, fmt.Errorf("select delivery attempts: %w", err)
	}
	defer rows.Close()

	var out []DeliveryAttempt
	for rows.Next() {
		var a DeliveryAttempt
		if err := rows.Scan(&a.ID, &a.NotificationID, &a.AttemptNo, &a.Status, &a.Error, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan delivery attempt: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
