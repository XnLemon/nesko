package ports

import (
	"context"
	"time"

	"github.com/xnlemon/nesko"
)

// ExecutionLifecycleRepository commits coupled ticket and run transitions atomically.
type ExecutionLifecycleRepository interface {
	StartExecution(ctx context.Context, ticketID, runID string, at time.Time) (nesko.Ticket, error)
	CompleteExecution(ctx context.Context, ticketID, runID string, ticketStatus nesko.TicketStatus, finish nesko.ExecutionRunFinish) error
	FailExecution(ctx context.Context, ticketID, runID string, finish nesko.ExecutionRunFinish) error
}
