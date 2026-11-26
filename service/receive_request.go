package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"

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
func NewReceiveRequest(tickets ports.TicketRepository, replier ports.MessageReplier, logger Logger) (*ReceiveRequest, error) {
	if tickets == nil {
		return nil, fmt.Errorf("ticket repository is required")
	}
	if replier == nil {
		return nil, fmt.Errorf("message replier is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ReceiveRequest{tickets: tickets, replier: replier, logger: logger}, nil
}

// Handle creates or loads a ticket and sends its acknowledgement.
func (r *ReceiveRequest) Handle(ctx context.Context, request nesko.IncomingRequest) error {
	_, err := r.Receive(ctx, request)
	return err
}

// Receive persists and acknowledges a request, returning its durable ticket.
func (r *ReceiveRequest) Receive(ctx context.Context, request nesko.IncomingRequest) (nesko.Ticket, error) {
	if err := request.Validate(); err != nil {
		return nesko.Ticket{}, err
	}
	ticket, err := r.tickets.CreateOrGet(ctx, request)
	if err != nil {
		return nesko.Ticket{}, fmt.Errorf("create or load ticket: %w", err)
	}
	ticketReference := nesko.FormatTicketReference(ticket)

	if ticket.AcknowledgementMessageID != "" {
		r.logger.Info("duplicate inbound message ignored", "ticket", ticketReference, "message_id", request.MessageID)
		return ticket, nil
	}

	replyMessageID, err := r.replier.ReplyToMessage(ctx, ports.ReplyInput{
		Channel:        request.Channel,
		ConversationID: request.ConversationID,
		MessageID:      request.MessageID,
		Text:           fmt.Sprintf("收到，已记录工单 %s。", ticketReference),
		IdempotencyKey: acknowledgementIdempotencyKey(request.Channel, request.MessageID),
	})
	if err != nil {
		return nesko.Ticket{}, fmt.Errorf("reply to inbound message: %w", err)
	}
	if strings.TrimSpace(replyMessageID) == "" {
		return nesko.Ticket{}, fmt.Errorf("message replier returned an empty reply ID")
	}
	if err := r.tickets.MarkAcknowledged(ctx, ticket.ID, replyMessageID); err != nil {
		return nesko.Ticket{}, fmt.Errorf("save acknowledgement message: %w", err)
	}

	r.logger.Info("inbound request recorded", "ticket", ticketReference, "message_id", request.MessageID)
	ticket.AcknowledgementMessageID = replyMessageID
	return ticket, nil
}

func acknowledgementIdempotencyKey(channel, messageID string) string {
	digest := sha256.Sum256([]byte(channel + "\x00" + messageID))
	return hex.EncodeToString(digest[:])[:40]
}
