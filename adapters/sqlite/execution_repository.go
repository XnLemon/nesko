package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/xnlemon/nesko"
	"github.com/xnlemon/nesko/ports"
)

const executionRunColumns = `id, ticket_id, instruction, decision_json, status, started_at,
	finished_at, outcome, summary, failure_reason,
	(SELECT occurred_at FROM execution_run_events
	 WHERE run_id = execution_runs.id AND event_type = 'heartbeat'
	 ORDER BY sequence DESC LIMIT 1)`

// StartExecutionRun stores the immutable instruction snapshot for a new attempt.
func (r *TicketRepository) StartExecutionRun(ctx context.Context, run nesko.ExecutionRun) error {
	if err := validateExecutionRun(run); err != nil {
		return err
	}
	decisionJSON, err := marshalRunDecision(run.Decision)
	if err != nil {
		return fmt.Errorf("encode execution run decision: %w", err)
	}
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("start SQLite execution run transaction: %w", err)
	}
	defer tx.Rollback()
	if err := requireTicketForRun(ctx, tx, run); err != nil {
		return err
	}
	existing, err := insertExecutionRun(ctx, tx, run, decisionJSON)
	if err != nil {
		return err
	}
	if existing {
		return tx.Commit()
	}
	if err := insertRunEvent(ctx, tx, run.ID, nesko.ExecutionEvent{
		ID:     uuid.NewString(),
		Type:   nesko.ExecutionEventRunStarted,
		At:     run.StartedAt,
		Detail: "execution run started",
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit execution run %s: %w", run.ID, err)
	}
	return nil
}

func requireTicketForRun(ctx context.Context, tx *sql.Tx, run nesko.ExecutionRun) error {
	var exists int
	err := tx.QueryRowContext(ctx, "SELECT 1 FROM tickets WHERE id = ?", run.TicketID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s", nesko.ErrTicketNotFound, run.TicketID)
	}
	if err != nil {
		return fmt.Errorf("inspect ticket for execution run %s: %w", run.ID, err)
	}
	return nil
}

func insertExecutionRun(ctx context.Context, tx *sql.Tx, run nesko.ExecutionRun, decisionJSON string) (bool, error) {
	inserted, err := tx.ExecContext(ctx, `
		INSERT INTO execution_runs (id, ticket_id, instruction, decision_json, status, started_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, run.ID, run.TicketID, run.Instruction, decisionJSON, run.Status, formatTime(run.StartedAt))
	if err != nil {
		return false, fmt.Errorf("insert execution run %s: %w", run.ID, err)
	}
	rowsInserted, err := inserted.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check execution run %s insertion: %w", run.ID, err)
	}
	if rowsInserted > 0 {
		return false, nil
	}
	return true, verifyExistingExecutionRun(ctx, tx, run, decisionJSON)
}

func verifyExistingExecutionRun(ctx context.Context, tx *sql.Tx, run nesko.ExecutionRun, decisionJSON string) error {
	var ticketID, instruction, storedDecision, startedAt string
	var status nesko.ExecutionRunStatus
	if err := tx.QueryRowContext(ctx, `
		SELECT ticket_id, instruction, decision_json, started_at, status FROM execution_runs WHERE id = ?
	`, run.ID).Scan(&ticketID, &instruction, &storedDecision, &startedAt, &status); err != nil {
		return fmt.Errorf("inspect existing execution run %s: %w", run.ID, err)
	}
	if ticketID != run.TicketID || instruction != run.Instruction || storedDecision != decisionJSON || startedAt != formatTime(run.StartedAt) {
		return fmt.Errorf("%w: run %s was already started with different input", nesko.ErrExecutionRunConflict, run.ID)
	}
	if status != nesko.ExecutionRunRunning {
		return fmt.Errorf("%w: run %s is already %s", nesko.ErrExecutionRunConflict, run.ID, status)
	}
	return nil
}

// RecordExecutionEvent appends an idempotent event while the run is active.
func (r *TicketRepository) RecordExecutionEvent(ctx context.Context, runID string, event nesko.ExecutionEvent) error {
	if strings.TrimSpace(runID) == "" {
		return errors.New("execution run ID is required")
	}
	if !recordableRunEvent(event.Type) {
		return fmt.Errorf("unsupported execution run event type %q", event.Type)
	}
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("start SQLite execution event transaction: %w", err)
	}
	defer tx.Rollback()
	if err := requireRunningExecutionRun(ctx, tx, runID); err != nil {
		return err
	}
	if err := insertRunEvent(ctx, tx, runID, event); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit execution event for run %s: %w", runID, err)
	}
	return nil
}

// FinishExecutionRun records a terminal result and its matching event atomically.
func (r *TicketRepository) FinishExecutionRun(ctx context.Context, runID string, finish nesko.ExecutionRunFinish) error {
	if strings.TrimSpace(runID) == "" {
		return errors.New("execution run ID is required")
	}
	if err := validateExecutionRunFinish(&finish); err != nil {
		return err
	}
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("start SQLite execution finish transaction: %w", err)
	}
	defer tx.Rollback()
	alreadyFinished, err := updateExecutionRun(ctx, tx, runID, finish)
	if err != nil {
		return err
	}
	if alreadyFinished {
		return tx.Commit()
	}
	if err := insertRunEvent(ctx, tx, runID, nesko.ExecutionEvent{
		ID:     uuid.NewString(),
		Type:   terminalRunEvent(finish.Status),
		At:     finish.At,
		Detail: terminalRunDetail(finish),
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit execution finish for run %s: %w", runID, err)
	}
	return nil
}

func updateExecutionRun(ctx context.Context, tx *sql.Tx, runID string, finish nesko.ExecutionRunFinish) (bool, error) {
	result, err := tx.ExecContext(ctx, `
		UPDATE execution_runs SET status = ?, finished_at = ?, outcome = ?, summary = ?, failure_reason = ?
		WHERE id = ? AND status = ?
	`, finish.Status, formatTime(finish.At), finish.Result.Outcome, finish.Result.Summary,
		finish.FailureReason, runID, nesko.ExecutionRunRunning)
	if err != nil {
		return false, fmt.Errorf("finish execution run %s: %w", runID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check execution run %s update: %w", runID, err)
	}
	if rowsAffected > 0 {
		return false, nil
	}
	return verifyFinishedExecutionRun(ctx, tx, runID, finish)
}

func verifyFinishedExecutionRun(ctx context.Context, tx *sql.Tx, runID string, finish nesko.ExecutionRunFinish) (bool, error) {
	var stored executionRunFinishRecord
	err := tx.QueryRowContext(ctx, `
		SELECT status, finished_at, outcome, summary, failure_reason
		FROM execution_runs WHERE id = ?
	`, runID).Scan(&stored.status, &stored.finishedAt, &stored.outcome, &stored.summary, &stored.failureReason)
	if errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("%w: %s", nesko.ErrExecutionRunNotFound, runID)
	}
	if err != nil {
		return false, fmt.Errorf("inspect finished execution run %s: %w", runID, err)
	}
	if stored.matches(finish) {
		return true, nil
	}
	return false, fmt.Errorf("%w: run %s is already %s", nesko.ErrExecutionRunConflict, runID, stored.status)
}

type executionRunFinishRecord struct {
	status        nesko.ExecutionRunStatus
	finishedAt    string
	outcome       string
	summary       string
	failureReason string
}

func (record executionRunFinishRecord) matches(finish nesko.ExecutionRunFinish) bool {
	return record.status == finish.Status && record.finishedAt == formatTime(finish.At) &&
		record.outcome == string(finish.Result.Outcome) && record.summary == finish.Result.Summary && record.failureReason == finish.FailureReason
}

// GetExecutionRun loads one execution attempt by its ID.
func (r *TicketRepository) GetExecutionRun(ctx context.Context, runID string) (nesko.ExecutionRun, error) {
	run, err := scanExecutionRun(r.database.QueryRowContext(ctx, `
		SELECT `+executionRunColumns+`
		FROM execution_runs WHERE id = ?
	`, runID))
	if errors.Is(err, sql.ErrNoRows) {
		return nesko.ExecutionRun{}, fmt.Errorf("%w: %s", nesko.ErrExecutionRunNotFound, runID)
	}
	if err != nil {
		return nesko.ExecutionRun{}, fmt.Errorf("load execution run %s: %w", runID, err)
	}
	return run, nil
}

// ListActiveExecutionRuns returns runs that were active when the repository was last written.
func (r *TicketRepository) ListActiveExecutionRuns(ctx context.Context) ([]nesko.ExecutionRun, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT `+executionRunColumns+`
		FROM execution_runs WHERE status = ? ORDER BY started_at, id
	`, nesko.ExecutionRunRunning)
	if err != nil {
		return nil, fmt.Errorf("query active execution runs: %w", err)
	}
	defer rows.Close()
	runs := make([]nesko.ExecutionRun, 0)
	for rows.Next() {
		run, err := scanExecutionRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan active execution run: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read active execution runs: %w", err)
	}
	return runs, nil
}

func scanExecutionRun(row interface{ Scan(...any) error }) (nesko.ExecutionRun, error) {
	var run nesko.ExecutionRun
	var startedAt string
	var finishedAt, outcome, summary, failureReason, decisionJSON, lastHeartbeatAt sql.NullString
	if err := row.Scan(&run.ID, &run.TicketID, &run.Instruction, &decisionJSON, &run.Status, &startedAt,
		&finishedAt, &outcome, &summary, &failureReason, &lastHeartbeatAt); err != nil {
		return nesko.ExecutionRun{}, err
	}
	parsedStartedAt, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return nesko.ExecutionRun{}, fmt.Errorf("parse execution run start time: %w", err)
	}
	run.StartedAt = parsedStartedAt
	if finishedAt.Valid {
		value, err := time.Parse(time.RFC3339Nano, finishedAt.String)
		if err != nil {
			return nesko.ExecutionRun{}, fmt.Errorf("parse execution run finish time: %w", err)
		}
		run.FinishedAt = &value
	}
	if lastHeartbeatAt.Valid {
		value, err := time.Parse(time.RFC3339Nano, lastHeartbeatAt.String)
		if err != nil {
			return nesko.ExecutionRun{}, fmt.Errorf("parse execution run heartbeat time: %w", err)
		}
		run.LastHeartbeatAt = &value
	}
	if decisionJSON.Valid && decisionJSON.String != "" {
		var decision nesko.TicketDecision
		if err := json.Unmarshal([]byte(decisionJSON.String), &decision); err != nil {
			return nesko.ExecutionRun{}, fmt.Errorf("decode execution run decision: %w", err)
		}
		if err := decision.Validate(); err != nil {
			return nesko.ExecutionRun{}, fmt.Errorf("validate execution run decision: %w", err)
		}
		run.Decision = &decision
	}
	run.Outcome = nesko.ExecutionOutcome(outcome.String)
	run.Summary = summary.String
	run.FailureReason = failureReason.String
	return run, nil
}

// ListExecutionRunEvents returns a run's events in durable insertion order.
func (r *TicketRepository) ListExecutionRunEvents(ctx context.Context, runID string) ([]nesko.ExecutionRunEvent, error) {
	if _, err := r.GetExecutionRun(ctx, runID); err != nil {
		return nil, err
	}
	rows, err := r.database.QueryContext(ctx, `
		SELECT sequence, run_id, event_id, event_type, occurred_at, detail
		FROM execution_run_events WHERE run_id = ? ORDER BY sequence
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("query execution events for run %s: %w", runID, err)
	}
	defer rows.Close()
	events := make([]nesko.ExecutionRunEvent, 0)
	for rows.Next() {
		var event nesko.ExecutionRunEvent
		var eventID, eventType, occurredAt string
		if err := rows.Scan(&event.Sequence, &event.RunID, &eventID, &eventType, &occurredAt, &event.Event.Detail); err != nil {
			return nil, fmt.Errorf("scan execution event for run %s: %w", runID, err)
		}
		event.Event.ID = eventID
		event.Event.Type = nesko.ExecutionEventType(eventType)
		event.Event.At, err = time.Parse(time.RFC3339Nano, occurredAt)
		if err != nil {
			return nil, fmt.Errorf("parse execution event time: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read execution events for run %s: %w", runID, err)
	}
	return events, nil
}

func validateExecutionRun(run nesko.ExecutionRun) error {
	if strings.TrimSpace(run.ID) == "" || strings.TrimSpace(run.TicketID) == "" {
		return errors.New("execution run ID and ticket ID are required")
	}
	if run.Status != nesko.ExecutionRunRunning {
		return errors.New("new execution runs must start in running state")
	}
	if run.StartedAt.IsZero() {
		return errors.New("execution run start time is required")
	}
	if run.Decision != nil {
		if err := run.Decision.Validate(); err != nil {
			return fmt.Errorf("validate execution run decision: %w", err)
		}
	}
	return nil
}

func marshalRunDecision(decision *nesko.TicketDecision) (string, error) {
	if decision == nil {
		return "", nil
	}
	encoded, err := json.Marshal(decision)
	return string(encoded), err
}

func validateExecutionRunFinish(finish *nesko.ExecutionRunFinish) error {
	if finish.At.IsZero() {
		finish.At = time.Now().UTC()
	}
	if finish.Status == nesko.ExecutionRunFailed || finish.Status == nesko.ExecutionRunInterrupted {
		if strings.TrimSpace(finish.FailureReason) == "" {
			return fmt.Errorf("%s execution runs require a failure reason", finish.Status)
		}
		return nil
	}
	expectedOutcome, ok := outcomeForRunStatus(finish.Status)
	if !ok {
		return fmt.Errorf("unsupported terminal execution run status %q", finish.Status)
	}
	if finish.Result.Outcome != expectedOutcome || finish.FailureReason != "" {
		return fmt.Errorf("%s execution runs require a %s outcome without a failure reason", finish.Status, expectedOutcome)
	}
	return nil
}

func outcomeForRunStatus(status nesko.ExecutionRunStatus) (nesko.ExecutionOutcome, bool) {
	switch status {
	case nesko.ExecutionRunCompleted:
		return nesko.ExecutionOutcomeCompleted, true
	case nesko.ExecutionRunNeedsReview:
		return nesko.ExecutionOutcomeNeedsReview, true
	case nesko.ExecutionRunWaitingHuman:
		return nesko.ExecutionOutcomeWaitingHuman, true
	default:
		return "", false
	}
}

func recordableRunEvent(eventType nesko.ExecutionEventType) bool {
	switch eventType {
	case nesko.ExecutionEventWorkspaceReady, nesko.ExecutionEventWorkspaceRelease,
		nesko.ExecutionEventHeartbeat, nesko.ExecutionEventProgress:
		return true
	default:
		return false
	}
}

func requireRunningExecutionRun(ctx context.Context, tx *sql.Tx, runID string) error {
	var status nesko.ExecutionRunStatus
	err := tx.QueryRowContext(ctx, "SELECT status FROM execution_runs WHERE id = ?", runID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s", nesko.ErrExecutionRunNotFound, runID)
	}
	if err != nil {
		return fmt.Errorf("inspect execution run %s: %w", runID, err)
	}
	if status != nesko.ExecutionRunRunning {
		return fmt.Errorf("%w: run %s is %s", nesko.ErrExecutionRunConflict, runID, status)
	}
	return nil
}

func insertRunEvent(ctx context.Context, tx *sql.Tx, runID string, event nesko.ExecutionEvent) error {
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO execution_run_events (run_id, event_id, event_type, occurred_at, detail)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(run_id, event_id) DO NOTHING
	`, runID, event.ID, event.Type, formatTime(event.At), event.Detail)
	if err != nil {
		return fmt.Errorf("insert event for execution run %s: %w", runID, err)
	}
	rowsInserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check execution event for run %s: %w", runID, err)
	}
	if rowsInserted == 0 {
		var eventType, occurredAt, detail string
		if err := tx.QueryRowContext(ctx, `
			SELECT event_type, occurred_at, detail
			FROM execution_run_events WHERE run_id = ? AND event_id = ?
		`, runID, event.ID).Scan(&eventType, &occurredAt, &detail); err != nil {
			return fmt.Errorf("inspect existing event for run %s: %w", runID, err)
		}
		if eventType != string(event.Type) || occurredAt != formatTime(event.At) || detail != event.Detail {
			return fmt.Errorf("%w: event %s has different content", nesko.ErrExecutionRunConflict, event.ID)
		}
	}
	return nil
}

func terminalRunEvent(status nesko.ExecutionRunStatus) nesko.ExecutionEventType {
	return nesko.ExecutionEventType(status)
}

func terminalRunDetail(finish nesko.ExecutionRunFinish) string {
	if finish.FailureReason != "" {
		return finish.FailureReason
	}
	return finish.Result.Summary
}

var _ ports.ExecutionRunRepository = (*TicketRepository)(nil)
