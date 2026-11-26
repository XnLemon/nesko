package ports

import (
	"context"

	"github.com/xnlemon/nesko"
)

// TicketRepository owns durable ticket state.
type TicketRepository interface {
	CreateOrGet(ctx context.Context, request nesko.IncomingRequest) (nesko.Ticket, error)
	MarkAcknowledged(ctx context.Context, ticketID, replyMessageID string) error
	Close() error
}
