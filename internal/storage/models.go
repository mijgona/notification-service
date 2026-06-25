package storage

import "time"

const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusDelivered  = "delivered"
	StatusFailed     = "failed"
)

type Notification struct {
	ID             string    `json:"id"`
	IdempotencyKey string    `json:"idempotency_key"`
	Channel        string    `json:"channel"`
	Recipient      string    `json:"recipient"`
	Subject        string    `json:"subject"`
	Body           string    `json:"body"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

type DeliveryAttempt struct {
	ID             int64     `json:"id"`
	NotificationID string    `json:"notification_id"`
	AttemptNo      int32     `json:"attempt_no"`
	Status         string    `json:"status"`
	Error          string    `json:"error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}
