package storage

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OutboxRow struct {
	ID             int64
	NotificationID string
}

type Outbox struct {
	pool *pgxpool.Pool
}

func NewOutbox(pool *pgxpool.Pool) *Outbox {
	return &Outbox{pool: pool}
}

// Claim opens a transaction and locks up to limit unprocessed rows with
// FOR UPDATE SKIP LOCKED, so several relay instances can run in parallel
// without ever picking the same row. The caller must finish with
// MarkProcessed and Commit, or Rollback.
func (s *Outbox) Claim(ctx context.Context, limit int) (pgx.Tx, []OutboxRow, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("begin tx: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT id, notification_id
		FROM outbox
		WHERE processed_at IS NULL
		ORDER BY id
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, nil, fmt.Errorf("select outbox batch: %w", err)
	}
	defer rows.Close()

	var batch []OutboxRow
	for rows.Next() {
		var r OutboxRow
		if err := rows.Scan(&r.ID, &r.NotificationID); err != nil {
			_ = tx.Rollback(ctx)
			return nil, nil, fmt.Errorf("scan outbox row: %w", err)
		}
		batch = append(batch, r)
	}
	if err := rows.Err(); err != nil {
		_ = tx.Rollback(ctx)
		return nil, nil, fmt.Errorf("iterate outbox rows: %w", err)
	}
	return tx, batch, nil
}

func (s *Outbox) MarkProcessed(ctx context.Context, tx pgx.Tx, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx,
		`UPDATE outbox SET processed_at = now() WHERE id = ANY($1)`, ids); err != nil {
		return fmt.Errorf("mark outbox processed: %w", err)
	}
	return nil
}
