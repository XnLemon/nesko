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

// InterruptStaleExecution atomically interrupts a stale run and its running ticket.
func (r *TicketRepository) InterruptStaleExecution(ctx context.Context, runID string, heartbeatBefore, at time.Time, reason string) error {
	finish, err := newInterruptionFinish(runID, heartbeatBefore, at, reason)
	if err != nil {
		return err
	}

	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("start SQLite execution recovery transaction: %w", err)
	}
	defer tx.Rollback()
	if err := interruptStaleExecutionTx(ctx, tx, runID, heartbeatBefore, finish); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit execution recovery for run %s: %w", runID, err)
	}
	return nil
}

func newInterruptionFinish(runID string, heartbeatBefore, at time.Time, reason string) (nesko.ExecutionRunFinish, error) {
	if strings.TrimSpace(runID) == "" {
		return nesko.ExecutionRunFinish{}, errors.New("execution run ID is required")
	}
	if heartbeatBefore.IsZero() {
		return nesko.ExecutionRunFinish{}, errors.New("heartbeat cutoff is required")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	if at.Before(heartbeatBefore) {
		return nesko.ExecutionRunFinish{}, errors.New("interrupt time must be at or after heartbeat cutoff")
	}
	finish := nesko.ExecutionRunFinish{
		Status:        nesko.ExecutionRunInterrupted,
		At:            at,
		FailureReason: strings.TrimSpace(reason),
	}
	if err := validateExecutionRunFinish(&finish); err != nil {
		return nesko.ExecutionRunFinish{}, err
	}
	return finish, nil
}

func interruptStaleExecutionTx(ctx context.Context, tx *sql.Tx, runID string, heartbeatBefore time.Time, finish nesko.ExecutionRunFinish) error {
	ticketID, alreadyInterrupted, err := requireStaleExecutionRun(ctx, tx, runID, heartbeatBefore, finish.FailureReason)
	if err != nil {
		return err
	}
	if alreadyInterrupted {
		return verifyInterruptedTicket(ctx, tx, interruptedTicketUpdate{
			ticketID: ticketID,
			runID:    runID,
			reason:   finish.FailureReason,
		})
	}
	if err := updateInterruptedRun(ctx, tx, runID, finish); err != nil {
		return err
	}
	return markTicketInterrupted(ctx, tx, interruptedTicketUpdate{
		ticketID: ticketID,
		runID:    runID,
		at:       finish.At,
		reason:   finish.FailureReason,
	})
}

func updateInterruptedRun(ctx context.Context, tx *sql.Tx, runID string, finish nesko.ExecutionRunFinish) error {
	alreadyInterrupted, err := updateExecutionRun(ctx, tx, runID, finish)
	if err != nil || alreadyInterrupted {
		return err
	}
	return insertRunEvent(ctx, tx, runID, nesko.ExecutionEvent{
		ID:     uuid.NewString(),
		Type:   nesko.ExecutionEventInterrupted,
		At:     finish.At,
		Detail: finish.FailureReason,
	})
}

func requireStaleExecutionRun(ctx context.Context, tx *sql.Tx, runID string, heartbeatBefore time.Time, reason string) (string, bool, error) {
	var ticketID string
	var status nesko.ExecutionRunStatus
	var startedAt, failureReason string
	err := tx.QueryRowContext(ctx, `
		SELECT ticket_id, status, started_at, failure_reason FROM execution_runs WHERE id = ?
	`, runID).Scan(&ticketID, &status, &startedAt, &failureReason)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, fmt.Errorf("%w: %s", nesko.ErrExecutionRunNotFound, runID)
	}
	if err != nil {
		return "", false, fmt.Errorf("inspect stale execution run %s: %w", runID, err)
	}
	if status == nesko.ExecutionRunInterrupted {
		if failureReason != reason {
			return "", false, fmt.Errorf("%w: run %s was interrupted for a different reason", nesko.ErrExecutionRunConflict, runID)
		}
		return ticketID, true, nil
	}
	if status != nesko.ExecutionRunRunning {
		return "", false, fmt.Errorf("%w: run %s is already %s", nesko.ErrExecutionRunConflict, runID, status)
	}
	lastActivity, err := latestHeartbeat(ctx, tx, runID, startedAt)
	if err != nil {
		return "", false, err
	}
	if lastActivity.After(heartbeatBefore) {
		return "", false, fmt.Errorf("%w: run %s had activity at %s", nesko.ErrExecutionRunNotStale, runID, formatTime(lastActivity))
	}
	return ticketID, false, nil
}

func latestHeartbeat(ctx context.Context, tx *sql.Tx, runID, startedAt string) (time.Time, error) {
	var occurredAt sql.NullString
	err := tx.QueryRowContext(ctx, `
		SELECT occurred_at FROM execution_run_events
		WHERE run_id = ? AND event_type = ? ORDER BY sequence DESC LIMIT 1
	`, runID, nesko.ExecutionEventHeartbeat).Scan(&occurredAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, fmt.Errorf("load last heartbeat for run %s: %w", runID, err)
	}
	value := startedAt
	if occurredAt.Valid {
		value = occurredAt.String
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse last activity for run %s: %w", runID, err)
	}
	return parsed, nil
}

type interruptedTicketUpdate struct {
	ticketID string
	runID    string
	at       time.Time
	reason   string
}

func markTicketInterrupted(ctx context.Context, tx *sql.Tx, update interruptedTicketUpdate) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE tickets SET status = ?, updated_at = ?, failure_reason = ?
		WHERE id = ? AND status = ? AND execution_id = ?
	`, nesko.TicketStatusFailed, formatTime(update.at), update.reason, update.ticketID, nesko.TicketStatusRunning, update.runID)
	if err != nil {
		return fmt.Errorf("interrupt ticket %s for run %s: %w", update.ticketID, update.runID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check interrupted ticket %s update: %w", update.ticketID, err)
	}
	if rowsAffected > 0 {
		return nil
	}
	return verifyInterruptedTicket(ctx, tx, update)
}

func verifyInterruptedTicket(ctx context.Context, tx *sql.Tx, update interruptedTicketUpdate) error {
	var status nesko.TicketStatus
	var executionID, failureReason string
	err := tx.QueryRowContext(ctx, `
		SELECT status, execution_id, failure_reason FROM tickets WHERE id = ?
	`, update.ticketID).Scan(&status, &executionID, &failureReason)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s", nesko.ErrTicketNotFound, update.ticketID)
	}
	if err != nil {
		return fmt.Errorf("inspect interrupted ticket %s: %w", update.ticketID, err)
	}
	if status == nesko.TicketStatusFailed && executionID == update.runID && failureReason == update.reason {
		return nil
	}
	return fmt.Errorf("%w: ticket %s is %s for execution %s", nesko.ErrTicketStatusConflict, update.ticketID, status, executionID)
}

var _ ports.ExecutionRecoveryRepository = (*TicketRepository)(nil)
