package ports

import "context"

// ReplyInput describes a reply to an inbound channel message.
type ReplyInput struct {
	MessageID      string
	Text           string
	IdempotencyKey string
}

// MessageReplier sends a reply through a messaging channel.
type MessageReplier interface {
	ReplyToMessage(ctx context.Context, input ReplyInput) (string, error)
}
