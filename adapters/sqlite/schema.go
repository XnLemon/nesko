package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

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
			updated_at TEXT NOT NULL DEFAULT '',
			acknowledgement_message_id TEXT,
			complexity TEXT,
			change_risk TEXT,
			handling_mode TEXT,
			decision_rationale TEXT,
			assessment_json TEXT NOT NULL DEFAULT '',
			execution_id TEXT NOT NULL DEFAULT '',
			last_heartbeat_at TEXT,
			last_progress TEXT NOT NULL DEFAULT '',
			execution_summary TEXT NOT NULL DEFAULT '',
			failure_reason TEXT NOT NULL DEFAULT '',
			UNIQUE(channel, source_message_id)
		);
	`); err != nil {
		return fmt.Errorf("create SQLite tickets table: %w", err)
	}
	if err := r.ensureTicketColumns(ctx); err != nil {
		return err
	}
	if _, err := r.database.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS execution_runs (
			id TEXT PRIMARY KEY,
			ticket_id TEXT NOT NULL,
			instruction TEXT NOT NULL,
			decision_json TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT,
			outcome TEXT NOT NULL DEFAULT '',
			summary TEXT NOT NULL DEFAULT '',
			failure_reason TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS execution_runs_ticket_idx ON execution_runs(ticket_id, started_at);
		CREATE TABLE IF NOT EXISTS execution_run_events (
			sequence INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id TEXT NOT NULL,
			event_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			occurred_at TEXT NOT NULL,
			detail TEXT NOT NULL DEFAULT '',
			UNIQUE(run_id, event_id)
		);
		CREATE INDEX IF NOT EXISTS execution_run_events_run_idx ON execution_run_events(run_id, sequence);
	`); err != nil {
		return fmt.Errorf("create SQLite execution run tables: %w", err)
	}
	if err := r.ensureExecutionRunColumns(ctx); err != nil {
		return err
	}
	if _, err := r.database.ExecContext(ctx, "UPDATE tickets SET updated_at = created_at WHERE updated_at = ''"); err != nil {
		return fmt.Errorf("backfill ticket update timestamps: %w", err)
	}
	if _, err := r.database.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS tickets_created_at_idx ON tickets(created_at)"); err != nil {
		return fmt.Errorf("create SQLite ticket index: %w", err)
	}
	return nil
}

func (r *TicketRepository) ensureExecutionRunColumns(ctx context.Context) error {
	rows, err := r.database.QueryContext(ctx, "PRAGMA table_info(execution_runs)")
	if err != nil {
		return fmt.Errorf("inspect SQLite execution runs table: %w", err)
	}
	existing := make(map[string]struct{})
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read SQLite execution runs columns: %w", err)
		}
		existing[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("read SQLite execution runs columns: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close SQLite execution runs inspection: %w", err)
	}
	if _, ok := existing["decision_json"]; ok {
		return nil
	}
	if _, err := r.database.ExecContext(ctx, "ALTER TABLE execution_runs ADD COLUMN decision_json TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("add SQLite execution run decision snapshot: %w", err)
	}
	return nil
}

// ensureTicketColumns keeps existing ticket identities and idempotency keys intact.
func (r *TicketRepository) ensureTicketColumns(ctx context.Context) error {
	rows, err := r.database.QueryContext(ctx, "PRAGMA table_info(tickets)")
	if err != nil {
		return fmt.Errorf("inspect SQLite tickets table: %w", err)
	}
	existing := make(map[string]struct{})
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read SQLite tickets columns: %w", err)
		}
		existing[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("read SQLite tickets columns: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close SQLite tickets column inspection: %w", err)
	}

	columns := []struct {
		name       string
		definition string
	}{
		{name: "updated_at", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "complexity", definition: "TEXT"},
		{name: "change_risk", definition: "TEXT"},
		{name: "handling_mode", definition: "TEXT"},
		{name: "decision_rationale", definition: "TEXT"},
		{name: "assessment_json", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "execution_id", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "last_heartbeat_at", definition: "TEXT"},
		{name: "last_progress", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "execution_summary", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "failure_reason", definition: "TEXT NOT NULL DEFAULT ''"},
	}
	for _, column := range columns {
		if _, ok := existing[column.name]; ok {
			continue
		}
		if _, err := r.database.ExecContext(ctx, fmt.Sprintf("ALTER TABLE tickets ADD COLUMN %s %s", column.name, column.definition)); err != nil {
			return fmt.Errorf("add SQLite tickets column %s: %w", column.name, err)
		}
	}
	return nil
}
