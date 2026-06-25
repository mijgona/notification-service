package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/mijgona/notification-service/internal/channel"
	"github.com/mijgona/notification-service/internal/config"
	"github.com/mijgona/notification-service/internal/storage"
	wf "github.com/mijgona/notification-service/internal/workflow"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
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

	activities := &wf.Activities{
		Notifications: storage.NewNotifications(pool),
		Attempts:      storage.NewAttempts(pool),
		Channels: map[string]channel.Channel{
			"telegram": channel.NewTelegram(cfg.TelegramBotToken),
			"email":    channel.NewEmail(cfg.SMTPAddr, cfg.SMTPFrom),
		},
	}

	w := worker.New(tc, cfg.TaskQueue, worker.Options{})
	w.RegisterWorkflow(wf.DeliveryWorkflow)
	w.RegisterActivity(activities)

	log.Info("worker started", "task_queue", cfg.TaskQueue)
	if err := w.Run(worker.InterruptCh()); err != nil {
		log.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}
