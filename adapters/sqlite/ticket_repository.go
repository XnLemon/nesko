package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/xnlemon/nesko"
)

// TicketRepository stores tickets in a SQLite database.
type TicketRepository struct {
	database *sql.DB
}

// NewTicketRepository opens the database and ensures the ticket schema exists.
func NewTicketRepository(databasePath string) (*TicketRepository, error) {
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	database.SetMaxOpenConns(1)

	repository := &TicketRepository{database: database}
	if err := repository.initialize(context.Background()); err != nil {
		_ = database.Close()
		return nil, err
	}
	return repository, nil
}

func (r *TicketRepository) initialize(ctx context.Context) error {
	if _, err := r.database.ExecContext(ctx, "PRAGMA journal_mode = WAL"); err != nil {
		return fmt.Errorf("enable SQLite WAL mode: %w", err)
	}
	if _, err := r.database.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS tickets (
			number INTEGER PRIMARY KEY AUTOINCREMENT,
			id TEXT NOT NULL UNIQUE,
			channel TEXT NOT NULL,
			source_message_id TEXT NOT NULL,
			conversation_id TEXT NOT NULL,
			requester_id TEXT NOT NULL,
			request_text TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			acknowledgement_message_id TEXT,
			UNIQUE(channel, source_message_id)
		);
	`); err != nil {
		return fmt.Errorf("create SQLite tickets table: %w", err)
	}
	if _, err := r.database.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS tickets_created_at_idx ON tickets(created_at)"); err != nil {
		return fmt.Errorf("create SQLite ticket index: %w", err)
	}
	return nil
}

func (r *TicketRepository) CreateOrGet(ctx context.Context, request nesko.IncomingRequest) (nesko.Ticket, error) {
	_, err := r.database.ExecContext(ctx, `
		INSERT OR IGNORE INTO tickets (
			id, channel, source_message_id, conversation_id, requester_id,
			request_text, status, created_at
		) VALUES (?, ?, ?, ?, ?, ?, 'received', ?)
	`,
		uuid.NewString(),
		request.Channel,
		request.MessageID,
		request.ConversationID,
		request.RequesterID,
		request.Text,
		request.ReceivedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
	)
	if err != nil {
		return nesko.Ticket{}, fmt.Errorf("insert ticket: %w", err)
	}

	var ticket nesko.Ticket
	var acknowledgementMessageID sql.NullString
	err = r.database.QueryRowContext(ctx, `
		SELECT number, id, channel, source_message_id, conversation_id,
		       requester_id, request_text, status, created_at, acknowledgement_message_id
		FROM tickets
		WHERE channel = ? AND source_message_id = ?
	`, request.Channel, request.MessageID).Scan(
		&ticket.Number,
		&ticket.ID,
		&ticket.Channel,
		&ticket.SourceMessageID,
		&ticket.ConversationID,
		&ticket.RequesterID,
		&ticket.RequestText,
		&ticket.Status,
		&ticket.CreatedAt,
		&acknowledgementMessageID,
	)
	if err != nil {
		return nesko.Ticket{}, fmt.Errorf("load ticket for message %s: %w", request.MessageID, err)
	}
	if acknowledgementMessageID.Valid {
		ticket.AcknowledgementMessageID = acknowledgementMessageID.String
	}
	return ticket, nil
}

func (r *TicketRepository) MarkAcknowledged(ctx context.Context, ticketID, replyMessageID string) error {
	result, err := r.database.ExecContext(ctx, `
		UPDATE tickets
		SET acknowledgement_message_id = COALESCE(NULLIF(?, ''), acknowledgement_message_id)
		WHERE id = ?
	`, replyMessageID, ticketID)
	if err != nil {
		return fmt.Errorf("update ticket acknowledgement: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check ticket acknowledgement update: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("ticket %s does not exist", ticketID)
	}
	return nil
}

// Close releases the database connection pool.
func (r *TicketRepository) Close() error {
	return r.database.Close()
}
