package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/xnlemon/nesko"
	"github.com/xnlemon/nesko/ports"
)

// Logger is the structured logging surface used by the intake service.
type Logger interface {
	Info(message string, args ...any)
	Error(message string, args ...any)
}

// ReceiveRequest persists an inbound request and acknowledges it once.
type ReceiveRequest struct {
	tickets ports.TicketRepository
	replier ports.MessageReplier
	logger  Logger
}

// NewReceiveRequest wires the ticket and reply ports for message intake.
func NewReceiveRequest(tickets ports.TicketRepository, replier ports.MessageReplier, logger Logger) *ReceiveRequest {
	return &ReceiveRequest{tickets: tickets, replier: replier, logger: logger}
}

// Handle creates or loads a ticket and sends its acknowledgement.
func (r *ReceiveRequest) Handle(ctx context.Context, request nesko.IncomingRequest) error {
	ticket, err := r.tickets.CreateOrGet(ctx, request)
	if err != nil {
		return fmt.Errorf("create or load ticket: %w", err)
	}
	ticketReference := nesko.FormatTicketReference(ticket)

	if ticket.AcknowledgementMessageID != "" {
		r.logger.Info("duplicate Feishu message ignored", "ticket", ticketReference, "message_id", request.MessageID)
		return nil
	}

	replyMessageID, err := r.replier.ReplyToMessage(ctx, ports.ReplyInput{
		MessageID:      request.MessageID,
		Text:           fmt.Sprintf("收到，已记录工单 %s。", ticketReference),
		IdempotencyKey: acknowledgementIdempotencyKey(request.MessageID),
	})
	if err != nil {
		return fmt.Errorf("reply to Feishu message: %w", err)
	}
	if err := r.tickets.MarkAcknowledged(ctx, ticket.ID, replyMessageID); err != nil {
		return fmt.Errorf("save acknowledgement message: %w", err)
	}

	r.logger.Info("Feishu request recorded", "ticket", ticketReference, "message_id", request.MessageID)
	return nil
}

func acknowledgementIdempotencyKey(messageID string) string {
	digest := sha256.Sum256([]byte("feishu-ack:" + messageID))
	return hex.EncodeToString(digest[:])[:40]
}
