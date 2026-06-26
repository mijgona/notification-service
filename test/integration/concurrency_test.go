//go:build integration

package integration

import (
	"context"
	"sync"
	"testing"

	"github.com/mijgona/notification-service/internal/storage"
)

// US3: concurrent outbox claim — several relay-style workers claiming with
// FOR UPDATE SKIP LOCKED never process the same row twice and together process
// every row exactly once (FR-009).
func TestOutboxConcurrentClaim(t *testing.T) {
	resetState(t, testEnv)
	ctx := context.Background()
	ob := storage.NewOutbox(testPool)

	const total = 50
	for i := 0; i < total; i++ {
		seedOutboxRow(t, i)
	}

	const workers = 4
	var (
		mu   sync.Mutex
		seen = make(map[int64]int)
		wg   sync.WaitGroup
	)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				tx, batch, err := ob.Claim(ctx, 5)
				if err != nil {
					t.Errorf("claim: %v", err)
					return
				}
				if len(batch) == 0 {
					_ = tx.Rollback(ctx)
					return
				}
				ids := make([]int64, 0, len(batch))
				mu.Lock()
				for _, r := range batch {
					seen[r.ID]++
					ids = append(ids, r.ID)
				}
				mu.Unlock()
				if err := ob.MarkProcessed(ctx, tx, ids); err != nil {
					t.Errorf("mark processed: %v", err)
					_ = tx.Rollback(ctx)
					return
				}
				if err := tx.Commit(ctx); err != nil {
					t.Errorf("commit: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	if len(seen) != total {
		t.Fatalf("processed %d distinct rows, want %d", len(seen), total)
	}
	for id, count := range seen {
		if count != 1 {
			t.Fatalf("row %d processed %d times, want exactly once", id, count)
		}
	}

	var remaining int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM outbox WHERE processed_at IS NULL`).Scan(&remaining); err != nil {
		t.Fatalf("count unprocessed: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("unprocessed outbox rows = %d, want 0", remaining)
	}
}

func seedOutboxRow(t *testing.T, i int) {
	t.Helper()
	ctx := context.Background()
	var notifID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO notifications (idempotency_key, channel, recipient, body)
		 VALUES ($1, 'email', 'conc@example.com', 'b') RETURNING id`,
		"conc-key-"+itoa(int64(i))).Scan(&notifID); err != nil {
		t.Fatalf("seed notification: %v", err)
	}
	if _, err := testPool.Exec(ctx,
		`INSERT INTO outbox (notification_id) VALUES ($1)`, notifID); err != nil {
		t.Fatalf("seed outbox: %v", err)
	}
}
