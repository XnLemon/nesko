package nesko

import "fmt"

// Ticket records one accepted request and its acknowledgement state.
type Ticket struct {
	ID                       string
	Number                   int64
	Channel                  string
	SourceMessageID          string
	ConversationID           string
	RequesterID              string
	RequestText              string
	Status                   string
	CreatedAt                string
	AcknowledgementMessageID string
}

// FormatTicketReference returns the user-facing ticket number.
func FormatTicketReference(ticket Ticket) string {
	return fmt.Sprintf("TKT-%06d", ticket.Number)
}
