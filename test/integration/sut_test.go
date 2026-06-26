//go:build integration

package integration

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/mijgona/notification-service/internal/api"
	"github.com/mijgona/notification-service/internal/channel"
	"github.com/mijgona/notification-service/internal/outbox"
	"github.com/mijgona/notification-service/internal/storage"
	wf "github.com/mijgona/notification-service/internal/workflow"
)

var queueSeq atomic.Int64

func uniqueQueue() string {
	return "it-queue-" + time.Now().Format("150405.000") + "-" + itoa(queueSeq.Add(1))
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

type sut struct {
	apiBaseURL string
	stop       func()
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// defaultChannels wires the real email channel at Mailpit plus telegram with no
// token (unused by the default scenarios). Permanent-failure tests pass their own map.
func defaultChannels(env Env) map[string]channel.Channel {
	return map[string]channel.Channel{
		"email":    channel.NewEmail(env.MailpitSMTPAddr, "notify@example.com"),
		"telegram": channel.NewTelegram(""),
	}
}

// newSUT composes the production api/relay/worker against the shared real
// dependencies, isolated by a fresh DB (resetState) and a per-SUT task queue
// (FR-007). It returns the API base URL and a stop() that tears everything down.
func newSUT(t *testing.T, env Env, channels map[string]channel.Channel) sut {
	t.Helper()
	resetState(t, env)

	ctx, cancel := context.WithCancel(context.Background())
	log := discardLogger()

	pool, err := pgxpool.New(ctx, env.PostgresURL)
	if err != nil {
		cancel()
		t.Fatalf("newSUT: pgxpool: %v", err)
	}

	notifs := storage.NewNotifications(pool)
	attempts := storage.NewAttempts(pool)

	mux := http.NewServeMux()
	api.NewHandler(notifs, attempts, log).Register(mux)
	srv := httptest.NewServer(mux)

	tcli, err := client.Dial(client.Options{HostPort: env.TemporalHostPort, Logger: log})
	if err != nil {
		srv.Close()
		pool.Close()
		cancel()
		t.Fatalf("newSUT: temporal dial: %v", err)
	}

	taskQueue := uniqueQueue()
	w := worker.New(tcli, taskQueue, worker.Options{})
	w.RegisterWorkflow(wf.DeliveryWorkflow)
	w.RegisterActivity(&wf.Activities{Notifications: notifs, Attempts: attempts, Channels: channels})
	if err := w.Start(); err != nil {
		tcli.Close()
		srv.Close()
		pool.Close()
		cancel()
		t.Fatalf("newSUT: worker start: %v", err)
	}

	relay := outbox.New(storage.NewOutbox(pool), tcli, taskQueue, 200*time.Millisecond, 100, log)
	go func() { _ = relay.Run(ctx) }()

	stop := func() {
		cancel()    // stop the relay loop
		w.Stop()    // drain the worker
		srv.Close() // shut down the API
		tcli.Close()
		pool.Close()
	}
	return sut{apiBaseURL: srv.URL, stop: stop}
}
