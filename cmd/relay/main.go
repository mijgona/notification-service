package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/client"

	"github.com/mijgona/notification-service/internal/config"
	"github.com/mijgona/notification-service/internal/outbox"
	"github.com/mijgona/notification-service/internal/storage"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("connect to postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	tc, err := client.Dial(client.Options{HostPort: cfg.TemporalHostPort})
	if err != nil {
		log.Error("connect to temporal", "error", err)
		os.Exit(1)
	}
	defer tc.Close()

	r := outbox.New(storage.NewOutbox(pool), tc, cfg.TaskQueue, cfg.RelayInterval, cfg.RelayBatchSize, log)
	log.Info("relay started", "interval", cfg.RelayInterval.String(), "batch_size", cfg.RelayBatchSize)
	if err := r.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("relay stopped", "error", err)
		os.Exit(1)
	}
}
