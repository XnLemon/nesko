package nesko

import (
	"errors"
	"fmt"
	"time"
)

// TicketStatus describes the current point in the ticket lifecycle.
type TicketStatus string

const (
	TicketStatusReceived         TicketStatus = "received"
	TicketStatusTriaging         TicketStatus = "triaging"
	TicketStatusQueued           TicketStatus = "queued"
	TicketStatusRunning          TicketStatus = "running"
	TicketStatusAwaitingApproval TicketStatus = "awaiting_approval"
	TicketStatusNeedsReview      TicketStatus = "needs_review"
	TicketStatusWaitingHuman     TicketStatus = "waiting_human"
	TicketStatusCompleted        TicketStatus = "completed"
	TicketStatusFailed           TicketStatus = "failed"
	TicketStatusCancelled        TicketStatus = "cancelled"
)

var (
	ErrInvalidTicketTransition = errors.New("invalid ticket transition")
	ErrTicketNotFound          = errors.New("ticket not found")
	ErrTicketStatusConflict    = errors.New("ticket status conflict")
)

// Ticket records one request, its handling decision, and its execution state.
type Ticket struct {
	ID                       string
	Number                   int64
	Channel                  string
	SourceMessageID          string
	ConversationID           string
	RequesterID              string
	RequestText              string
	Status                   TicketStatus
	CreatedAt                string
	UpdatedAt                string
	AcknowledgementMessageID string
	Decision                 *TicketDecision
	ExecutionID              string
	LastHeartbeatAt          *time.Time
	LastProgress             string
	ExecutionSummary         string
	FailureReason            string
}

// TicketTransition describes one compare-and-set lifecycle change.
type TicketTransition struct {
	From             TicketStatus
	To               TicketStatus
	At               time.Time
	Decision         *TicketDecision
	ClearDecision    bool
	ExecutionID      string
	FailureReason    string
	ClearFailure     bool
	ExecutionSummary string
}

// CanTransitionTicket reports whether a lifecycle transition is allowed.
func CanTransitionTicket(from, to TicketStatus) bool {
	switch from {
	case TicketStatusReceived, TicketStatusFailed:
		return to == TicketStatusTriaging || to == TicketStatusCancelled
	case TicketStatusTriaging:
		return to == TicketStatusQueued || to == TicketStatusAwaitingApproval ||
			to == TicketStatusWaitingHuman || to == TicketStatusFailed || to == TicketStatusCancelled
	case TicketStatusQueued:
		return to == TicketStatusRunning || to == TicketStatusFailed || to == TicketStatusCancelled
	case TicketStatusRunning:
		return to == TicketStatusCompleted || to == TicketStatusNeedsReview ||
			to == TicketStatusWaitingHuman || to == TicketStatusFailed || to == TicketStatusCancelled
	case TicketStatusNeedsReview:
		return to == TicketStatusCompleted || to == TicketStatusCancelled
	case TicketStatusAwaitingApproval:
		return to == TicketStatusQueued || to == TicketStatusCancelled
	case TicketStatusWaitingHuman:
		return to == TicketStatusTriaging || to == TicketStatusCancelled
	default:
		return false
	}
}

// FormatTicketReference returns the user-facing ticket number.
func FormatTicketReference(ticket Ticket) string {
	return fmt.Sprintf("TKT-%06d", ticket.Number)
}
