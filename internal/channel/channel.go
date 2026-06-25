// Package channel defines the delivery channel abstraction. Adding a new
// channel (SMS, push, webhook) means implementing this one interface and
// registering it in cmd/worker — nothing else in the service changes.
package channel

import "context"

type Message struct {
	Recipient string
	Subject   string
	Body      string
}

type Channel interface {
	Name() string
	Send(ctx context.Context, msg Message) error
}
