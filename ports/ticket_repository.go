package ports

import (
	"context"
	"time"

	"github.com/xnlemon/nesko"
)

// TicketRepository owns durable ticket state.
type TicketRepository interface {
	CreateOrGet(ctx context.Context, request nesko.IncomingRequest) (nesko.Ticket, error)
	Get(ctx context.Context, ticketID string) (nesko.Ticket, error)
	MarkAcknowledged(ctx context.Context, ticketID, replyMessageID string) error
	Transition(ctx context.Context, ticketID string, change nesko.TicketTransition) error
	ResumeWithHumanInput(ctx context.Context, ticketID, input string, at time.Time) (nesko.Ticket, error)
	RecordHeartbeat(ctx context.Context, ticketID, runID string, event nesko.ExecutionEvent) error
	Close() error
}
