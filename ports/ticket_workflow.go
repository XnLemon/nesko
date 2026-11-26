package ports

import (
	"context"

	"github.com/xnlemon/nesko"
)

// TicketWorkflow exposes lifecycle steps for caller-owned supervisors.
type TicketWorkflow interface {
	Assess(ctx context.Context, ticketID string) (nesko.Ticket, error)
	Triage(ctx context.Context, ticketID string) (nesko.Ticket, error)
	ResumeAndAssess(ctx context.Context, ticketID, humanInput string) (nesko.Ticket, error)
	Resume(ctx context.Context, ticketID, humanInput string) (nesko.Ticket, error)
	ApprovePlan(ctx context.Context, ticketID string) (nesko.Ticket, error)
	ApproveReview(ctx context.Context, ticketID string) (nesko.Ticket, error)
	RejectReview(ctx context.Context, ticketID string) error
	Execute(ctx context.Context, ticketID string) (nesko.Ticket, error)
}
