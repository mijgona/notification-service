package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr         string
	DatabaseURL      string
	TemporalHostPort string
	TaskQueue        string
	RelayInterval    time.Duration
	RelayBatchSize   int
	TelegramBotToken string
	SMTPAddr         string
	SMTPFrom         string
}

func Load() Config {
	return Config{
		HTTPAddr:         getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:      getenv("DATABASE_URL", "postgres://notify:notify@localhost:5432/notify?sslmode=disable"),
		TemporalHostPort: getenv("TEMPORAL_HOSTPORT", "localhost:7233"),
		TaskQueue:        getenv("TEMPORAL_TASK_QUEUE", "notifications"),
		RelayInterval:    getdur("RELAY_INTERVAL", 2*time.Second),
		RelayBatchSize:   getint("RELAY_BATCH_SIZE", 100),
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		SMTPAddr:         getenv("SMTP_ADDR", "localhost:1025"),
		SMTPFrom:         getenv("SMTP_FROM", "notify@example.com"),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getint(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getdur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
