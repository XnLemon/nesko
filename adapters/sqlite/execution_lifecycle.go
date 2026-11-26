package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/xnlemon/nesko"
	"github.com/xnlemon/nesko/ports"
)

// StartExecution atomically starts a queued ticket and creates its run snapshot.
func (r *TicketRepository) StartExecution(ctx context.Context, ticketID, runID string, at time.Time) (nesko.Ticket, error) {
	if strings.TrimSpace(ticketID) == "" || strings.TrimSpace(runID) == "" {
		return nesko.Ticket{}, errors.New("ticket ID and execution run ID are required")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return nesko.Ticket{}, fmt.Errorf("start SQLite execution lifecycle transaction: %w", err)
	}
	defer tx.Rollback()
	ticket, err := scanTicket(tx.QueryRowContext(ctx, "SELECT "+ticketColumns+" FROM tickets WHERE id = ?", ticketID))
	if errors.Is(err, sql.ErrNoRows) {
		return nesko.Ticket{}, fmt.Errorf("%w: %s", nesko.ErrTicketNotFound, ticketID)
	}
	if err != nil {
		return nesko.Ticket{}, fmt.Errorf("load ticket %s before execution: %w", ticketID, err)
	}
	if ticket.Status == nesko.TicketStatusRunning && ticket.ExecutionID == runID {
		if err := verifyStartedRun(ctx, tx, ticket, runID, at); err != nil {
			return nesko.Ticket{}, err
		}
		return ticket, tx.Commit()
	}
	if ticket.Status != nesko.TicketStatusQueued {
		return nesko.Ticket{}, fmt.Errorf("%w: ticket %s is %s", nesko.ErrTicketStatusConflict, ticketID, ticket.Status)
	}
	if ticket.Decision == nil {
		return nesko.Ticket{}, errors.New("queued ticket has no judge decision")
	}
	if err := ticket.Decision.Validate(); err != nil {
		return nesko.Ticket{}, fmt.Errorf("validate ticket decision: %w", err)
	}
	if err := transitionTicketToRunning(ctx, tx, ticketID, runID, at); err != nil {
		return nesko.Ticket{}, err
	}
	run := nesko.ExecutionRun{
		ID:          runID,
		TicketID:    ticket.ID,
		Instruction: ticket.RequestText,
		Decision:    ticket.Decision,
		Status:      nesko.ExecutionRunRunning,
		StartedAt:   at,
	}
	decisionJSON, err := marshalRunDecision(run.Decision)
	if err != nil {
		return nesko.Ticket{}, fmt.Errorf("encode execution run decision: %w", err)
	}
	existing, err := insertExecutionRun(ctx, tx, run, decisionJSON)
	if err != nil {
		return nesko.Ticket{}, err
	}
	if !existing {
		if err := insertRunEvent(ctx, tx, runID, nesko.ExecutionEvent{
			ID:     uuid.NewString(),
			Type:   nesko.ExecutionEventRunStarted,
			At:     at,
			Detail: "execution run started",
		}); err != nil {
			return nesko.Ticket{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nesko.Ticket{}, fmt.Errorf("commit execution start for run %s: %w", runID, err)
	}
	ticket.Status = nesko.TicketStatusRunning
	ticket.ExecutionID = runID
	ticket.UpdatedAt = formatTime(at)
	ticket.LastHeartbeatAt = nil
	ticket.LastProgress = ""
	ticket.ExecutionSummary = ""
	ticket.FailureReason = ""
	return ticket, nil
}

func verifyStartedRun(ctx context.Context, tx *sql.Tx, ticket nesko.Ticket, runID string, at time.Time) error {
	decisionJSON, err := marshalRunDecision(ticket.Decision)
	if err != nil {
		return fmt.Errorf("encode execution run decision: %w", err)
	}
	_, err = insertExecutionRun(ctx, tx, nesko.ExecutionRun{
		ID:          runID,
		TicketID:    ticket.ID,
		Instruction: ticket.RequestText,
		Decision:    ticket.Decision,
		Status:      nesko.ExecutionRunRunning,
		StartedAt:   at,
	}, decisionJSON)
	return err
}

func transitionTicketToRunning(ctx context.Context, tx *sql.Tx, ticketID, runID string, at time.Time) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE tickets SET status = ?, updated_at = ?, execution_id = ?,
			last_heartbeat_at = NULL, last_progress = '', execution_summary = '', failure_reason = ''
		WHERE id = ? AND status = ?
	`, nesko.TicketStatusRunning, formatTime(at), runID, ticketID, nesko.TicketStatusQueued)
	if err != nil {
		return fmt.Errorf("start ticket %s execution: %w", ticketID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check ticket %s execution start: %w", ticketID, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("%w: ticket %s is no longer queued", nesko.ErrTicketStatusConflict, ticketID)
	}
	return nil
}

// CompleteExecution atomically writes the executor result to the ticket and run.
func (r *TicketRepository) CompleteExecution(ctx context.Context, ticketID, runID string, ticketStatus nesko.TicketStatus, finish nesko.ExecutionRunFinish) error {
	runStatus, err := executionRunStatusForTicket(ticketStatus)
	if err != nil {
		return err
	}
	if finish.Status != runStatus {
		return fmt.Errorf("ticket status %s does not match execution run status %s", ticketStatus, finish.Status)
	}
	if err := validateExecutionRunFinish(&finish); err != nil {
		return err
	}
	if finish.Result.Outcome != outcomeForTicketStatus(ticketStatus) {
		return fmt.Errorf("ticket status %s does not match executor outcome %s", ticketStatus, finish.Result.Outcome)
	}
	return r.finishExecutionLifecycle(ctx, ticketID, runID, ticketStatus, finish)
}

// FailExecution atomically marks a running ticket and its run failed.
func (r *TicketRepository) FailExecution(ctx context.Context, ticketID, runID string, finish nesko.ExecutionRunFinish) error {
	if finish.Status != nesko.ExecutionRunFailed {
		return errors.New("failed ticket execution requires a failed run status")
	}
	if err := validateExecutionRunFinish(&finish); err != nil {
		return err
	}
	return r.finishExecutionLifecycle(ctx, ticketID, runID, nesko.TicketStatusFailed, finish)
}

func (r *TicketRepository) finishExecutionLifecycle(ctx context.Context, ticketID, runID string, ticketStatus nesko.TicketStatus, finish nesko.ExecutionRunFinish) error {
	if strings.TrimSpace(ticketID) == "" || strings.TrimSpace(runID) == "" {
		return errors.New("ticket ID and execution run ID are required")
	}
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("start SQLite execution finish transaction: %w", err)
	}
	defer tx.Rollback()
	if err := requireExecutionBelongsToTicket(ctx, tx, ticketID, runID); err != nil {
		return err
	}
	alreadyFinished, err := updateExecutionRun(ctx, tx, runID, finish)
	if err != nil {
		return err
	}
	if !alreadyFinished {
		if err := insertRunEvent(ctx, tx, runID, nesko.ExecutionEvent{
			ID:     uuid.NewString(),
			Type:   terminalRunEvent(finish.Status),
			At:     finish.At,
			Detail: terminalRunDetail(finish),
		}); err != nil {
			return err
		}
	}
	if err := updateTicketExecutionResult(ctx, tx, executionTicketResult{
		ticketID: ticketID,
		runID:    runID,
		status:   ticketStatus,
		at:       finish.At,
		summary:  finish.Result.Summary,
		reason:   finish.FailureReason,
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit execution finish for run %s: %w", runID, err)
	}
	return nil
}

func requireExecutionBelongsToTicket(ctx context.Context, tx *sql.Tx, ticketID, runID string) error {
	var storedTicketID string
	err := tx.QueryRowContext(ctx, "SELECT ticket_id FROM execution_runs WHERE id = ?", runID).Scan(&storedTicketID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s", nesko.ErrExecutionRunNotFound, runID)
	}
	if err != nil {
		return fmt.Errorf("inspect execution run %s: %w", runID, err)
	}
	if storedTicketID != ticketID {
		return fmt.Errorf("%w: run %s belongs to ticket %s", nesko.ErrExecutionRunConflict, runID, storedTicketID)
	}
	return nil
}

type executionTicketResult struct {
	ticketID string
	runID    string
	status   nesko.TicketStatus
	at       time.Time
	summary  string
	reason   string
}

func updateTicketExecutionResult(ctx context.Context, tx *sql.Tx, result executionTicketResult) error {
	updated, err := tx.ExecContext(ctx, `
		UPDATE tickets SET status = ?, updated_at = ?, execution_summary = ?, failure_reason = ?
		WHERE id = ? AND status = ? AND execution_id = ?
	`, result.status, formatTime(result.at), result.summary, result.reason,
		result.ticketID, nesko.TicketStatusRunning, result.runID)
	if err != nil {
		return fmt.Errorf("finish ticket %s execution: %w", result.ticketID, err)
	}
	rowsAffected, err := updated.RowsAffected()
	if err != nil {
		return fmt.Errorf("check ticket %s execution finish: %w", result.ticketID, err)
	}
	if rowsAffected > 0 {
		return nil
	}
	return verifyTicketExecutionResult(ctx, tx, result)
}

func verifyTicketExecutionResult(ctx context.Context, tx *sql.Tx, result executionTicketResult) error {
	var status nesko.TicketStatus
	var executionID, summary, reason string
	err := tx.QueryRowContext(ctx, `
		SELECT status, execution_id, execution_summary, failure_reason FROM tickets WHERE id = ?
	`, result.ticketID).Scan(&status, &executionID, &summary, &reason)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s", nesko.ErrTicketNotFound, result.ticketID)
	}
	if err != nil {
		return fmt.Errorf("inspect finished ticket %s: %w", result.ticketID, err)
	}
	if status == result.status && executionID == result.runID && summary == result.summary && reason == result.reason {
		return nil
	}
	return fmt.Errorf("%w: ticket %s is %s for execution %s", nesko.ErrTicketStatusConflict, result.ticketID, status, executionID)
}

func executionRunStatusForTicket(status nesko.TicketStatus) (nesko.ExecutionRunStatus, error) {
	switch status {
	case nesko.TicketStatusCompleted:
		return nesko.ExecutionRunCompleted, nil
	case nesko.TicketStatusNeedsReview:
		return nesko.ExecutionRunNeedsReview, nil
	case nesko.TicketStatusWaitingHuman:
		return nesko.ExecutionRunWaitingHuman, nil
	default:
		return "", fmt.Errorf("unsupported terminal ticket status %q", status)
	}
}

func outcomeForTicketStatus(status nesko.TicketStatus) nesko.ExecutionOutcome {
	switch status {
	case nesko.TicketStatusCompleted:
		return nesko.ExecutionOutcomeCompleted
	case nesko.TicketStatusNeedsReview:
		return nesko.ExecutionOutcomeNeedsReview
	case nesko.TicketStatusWaitingHuman:
		return nesko.ExecutionOutcomeWaitingHuman
	default:
		return ""
	}
}

var _ ports.ExecutionLifecycleRepository = (*TicketRepository)(nil)
