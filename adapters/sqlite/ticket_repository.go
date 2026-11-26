package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/xnlemon/nesko"
)

const ticketColumns = `number, id, channel, source_message_id, conversation_id,
	requester_id, request_text, status, created_at, updated_at,
	acknowledgement_message_id, complexity, change_risk, handling_mode,
	decision_rationale, COALESCE(assessment_json, ''), COALESCE(execution_id, ''), last_heartbeat_at,
	COALESCE(last_progress, ''), COALESCE(execution_summary, ''),
	COALESCE(failure_reason, '')`

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

func (r *TicketRepository) CreateOrGet(ctx context.Context, request nesko.IncomingRequest) (nesko.Ticket, error) {
	if err := request.Validate(); err != nil {
		return nesko.Ticket{}, err
	}
	receivedAt := request.ReceivedAt
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}
	createdAt := formatTime(receivedAt)
	_, err := r.database.ExecContext(ctx, `
		INSERT OR IGNORE INTO tickets (
			id, channel, source_message_id, conversation_id, requester_id,
			request_text, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		uuid.NewString(), request.Channel, request.MessageID, request.ConversationID,
		request.RequesterID, request.Text, nesko.TicketStatusReceived, createdAt, createdAt,
	)
	if err != nil {
		return nesko.Ticket{}, fmt.Errorf("insert ticket: %w", err)
	}

	ticket, err := scanTicket(r.database.QueryRowContext(ctx,
		"SELECT "+ticketColumns+" FROM tickets WHERE channel = ? AND source_message_id = ?",
		request.Channel, request.MessageID,
	))
	if err != nil {
		return nesko.Ticket{}, fmt.Errorf("load ticket for message %s: %w", request.MessageID, err)
	}
	return ticket, nil
}

// Get loads a ticket by its internal ID.
func (r *TicketRepository) Get(ctx context.Context, ticketID string) (nesko.Ticket, error) {
	ticket, err := scanTicket(r.database.QueryRowContext(ctx,
		"SELECT "+ticketColumns+" FROM tickets WHERE id = ?", ticketID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nesko.Ticket{}, fmt.Errorf("%w: %s", nesko.ErrTicketNotFound, ticketID)
	}
	if err != nil {
		return nesko.Ticket{}, fmt.Errorf("load ticket %s: %w", ticketID, err)
	}
	return ticket, nil
}

// MarkAcknowledged stores the message ID returned by the channel adapter.
func (r *TicketRepository) MarkAcknowledged(ctx context.Context, ticketID, replyMessageID string) error {
	result, err := r.database.ExecContext(ctx, `
		UPDATE tickets
		SET acknowledgement_message_id = COALESCE(NULLIF(?, ''), acknowledgement_message_id)
		WHERE id = ?
	`, replyMessageID, ticketID)
	if err != nil {
		return fmt.Errorf("update ticket acknowledgement: %w", err)
	}
	return requireUpdatedRow(result, ticketID)
}

// Transition applies an allowed status change only if the expected status is current.
func (r *TicketRepository) Transition(ctx context.Context, ticketID string, change nesko.TicketTransition) error {
	if err := validateTransition(change); err != nil {
		return err
	}
	if change.At.IsZero() {
		change.At = time.Now().UTC()
	}

	complexity, risk, mode, rationale, assessmentJSON, err := decisionValues(change.Decision)
	if err != nil {
		return fmt.Errorf("encode ticket assessment: %w", err)
	}
	decisionIsSet := change.Decision != nil
	resetRunState := change.To == nesko.TicketStatusTriaging || change.To == nesko.TicketStatusRunning
	result, err := r.database.ExecContext(ctx, `
		UPDATE tickets SET
			status = ?,
			updated_at = ?,
			complexity = CASE WHEN ? != 0 THEN NULL WHEN ? != 0 THEN ? ELSE complexity END,
			change_risk = CASE WHEN ? != 0 THEN NULL WHEN ? != 0 THEN ? ELSE change_risk END,
			handling_mode = CASE WHEN ? != 0 THEN NULL WHEN ? != 0 THEN ? ELSE handling_mode END,
			decision_rationale = CASE WHEN ? != 0 THEN NULL WHEN ? != 0 THEN ? ELSE decision_rationale END,
			assessment_json = CASE WHEN ? != 0 THEN '' WHEN ? != 0 THEN ? ELSE assessment_json END,
			execution_id = CASE WHEN ? != '' THEN ? WHEN ? != 0 THEN '' ELSE execution_id END,
			last_heartbeat_at = CASE WHEN ? != 0 THEN NULL ELSE last_heartbeat_at END,
			last_progress = CASE WHEN ? != 0 THEN '' ELSE last_progress END,
			execution_summary = CASE WHEN ? != 0 THEN '' WHEN ? != '' THEN ? ELSE execution_summary END,
			failure_reason = CASE WHEN ? != 0 THEN '' WHEN ? != '' THEN ? ELSE failure_reason END
		WHERE id = ? AND status = ?
	`,
		change.To, formatTime(change.At),
		boolInt(change.ClearDecision), boolInt(decisionIsSet), complexity,
		boolInt(change.ClearDecision), boolInt(decisionIsSet), risk,
		boolInt(change.ClearDecision), boolInt(decisionIsSet), mode,
		boolInt(change.ClearDecision), boolInt(decisionIsSet), rationale,
		boolInt(change.ClearDecision), boolInt(decisionIsSet), assessmentJSON,
		change.ExecutionID, change.ExecutionID, boolInt(resetRunState),
		boolInt(resetRunState), boolInt(resetRunState),
		boolInt(resetRunState), change.ExecutionSummary, change.ExecutionSummary,
		boolInt(change.ClearFailure), change.FailureReason, change.FailureReason,
		ticketID, change.From,
	)
	if err != nil {
		return fmt.Errorf("transition ticket %s: %w", ticketID, err)
	}
	if err := requireUpdatedRow(result, ticketID); err != nil {
		return r.transitionConflict(ctx, ticketID, change.From, err)
	}
	return nil
}

func validateTransition(change nesko.TicketTransition) error {
	if !nesko.CanTransitionTicket(change.From, change.To) {
		return fmt.Errorf("%w: %s to %s", nesko.ErrInvalidTicketTransition, change.From, change.To)
	}
	if change.Decision != nil {
		if err := change.Decision.Validate(); err != nil {
			return fmt.Errorf("validate ticket decision: %w", err)
		}
		if change.From != nesko.TicketStatusTriaging || change.To != change.Decision.Policy.TriageStatus() {
			return fmt.Errorf("%w: decision route %s does not match transition %s to %s",
				nesko.ErrInvalidTicketTransition, change.Decision.Policy.Mode, change.From, change.To)
		}
	}
	if change.ClearDecision && change.Decision != nil {
		return errors.New("a transition cannot set and clear a decision at once")
	}
	if change.To == nesko.TicketStatusRunning && change.ExecutionID == "" {
		return errors.New("execution ID is required when a ticket starts running")
	}
	return nil
}

// ResumeWithHumanInput appends user context and atomically moves a waiting ticket back to triage.
func (r *TicketRepository) ResumeWithHumanInput(ctx context.Context, ticketID, input string, at time.Time) (nesko.Ticket, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nesko.Ticket{}, errors.New("human input must not be empty")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	result, err := r.database.ExecContext(ctx, `
		UPDATE tickets SET
			request_text = request_text || char(10) || char(10) || 'Human input:' || char(10) || ?,
			status = ?, updated_at = ?,
			complexity = NULL, change_risk = NULL, handling_mode = NULL, decision_rationale = NULL,
			assessment_json = '',
			execution_id = '', last_heartbeat_at = NULL, last_progress = '',
			execution_summary = '', failure_reason = ''
		WHERE id = ? AND status = ?
	`, input, nesko.TicketStatusTriaging, formatTime(at), ticketID, nesko.TicketStatusWaitingHuman)
	if err != nil {
		return nesko.Ticket{}, fmt.Errorf("resume ticket %s: %w", ticketID, err)
	}
	if err := requireUpdatedRow(result, ticketID); err != nil {
		return nesko.Ticket{}, r.transitionConflict(ctx, ticketID, nesko.TicketStatusWaitingHuman, err)
	}
	return r.Get(ctx, ticketID)
}

// RecordHeartbeat updates the last heartbeat and optional progress for an active run.
func (r *TicketRepository) RecordHeartbeat(ctx context.Context, ticketID, runID string, event nesko.ExecutionEvent) error {
	if event.Type != nesko.ExecutionEventHeartbeat && event.Type != nesko.ExecutionEventProgress {
		return fmt.Errorf("unsupported execution event type %q", event.Type)
	}
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	result, err := r.database.ExecContext(ctx, `
		UPDATE tickets SET
			last_heartbeat_at = CASE WHEN ? = ? THEN ? ELSE last_heartbeat_at END, updated_at = ?,
			last_progress = CASE WHEN ? = ? THEN ? ELSE last_progress END
		WHERE id = ? AND status = ? AND execution_id = ?
	`, event.Type, nesko.ExecutionEventHeartbeat, formatTime(event.At), formatTime(event.At), event.Type, nesko.ExecutionEventProgress,
		event.Detail, ticketID, nesko.TicketStatusRunning, runID)
	if err != nil {
		return fmt.Errorf("record execution heartbeat for ticket %s: %w", ticketID, err)
	}
	if err := requireUpdatedRow(result, ticketID); err != nil {
		return r.transitionConflict(ctx, ticketID, nesko.TicketStatusRunning, err)
	}
	return nil
}

// Close releases the database connection pool.
func (r *TicketRepository) Close() error {
	return r.database.Close()
}

func requireUpdatedRow(result sql.Result, ticketID string) error {
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check ticket %s update: %w", ticketID, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("%w: %s", nesko.ErrTicketNotFound, ticketID)
	}
	return nil
}

func (r *TicketRepository) transitionConflict(ctx context.Context, ticketID string, expected nesko.TicketStatus, updateErr error) error {
	if errors.Is(updateErr, nesko.ErrTicketNotFound) {
		var exists int
		err := r.database.QueryRowContext(ctx, "SELECT 1 FROM tickets WHERE id = ?", ticketID).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %s", nesko.ErrTicketNotFound, ticketID)
		}
		if err != nil {
			return fmt.Errorf("check ticket %s state: %w", ticketID, err)
		}
		var current nesko.TicketStatus
		if err := r.database.QueryRowContext(ctx, "SELECT status FROM tickets WHERE id = ?", ticketID).Scan(&current); err != nil {
			return fmt.Errorf("load ticket %s state: %w", ticketID, err)
		}
		return fmt.Errorf("%w: ticket %s is %s, expected %s", nesko.ErrTicketStatusConflict, ticketID, current, expected)
	}
	return updateErr
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
