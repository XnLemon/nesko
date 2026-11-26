# SQLite Adapter

`adapters/sqlite.TicketRepository` implements the ticket, execution-run, lifecycle, and recovery repository ports with `database/sql` and the pure Go `modernc.org/sqlite` driver.

## Ticket storage

The repository opens the caller-provided database path, enables WAL mode, and limits the connection pool to one open connection. The `tickets` table stores request identity, status, acknowledgement ID, the structured Judge assessment and policy decision, active execution ID, heartbeat/progress, result summary, and failure reason.

The pair `(channel, source_message_id)` is unique. `CreateOrGet` inserts a new `received` ticket or loads the existing ticket for a duplicate delivery. Ticket transitions use the expected current state in the update predicate and reject disallowed transitions. Human input is appended while moving a `waiting_human` ticket back to `triaging`.

Initialization creates missing tables and indexes and adds missing columns to existing ticket and run tables. It preserves existing ticket identities and does not rebuild the tables.

## Execution history and atomic lifecycle

`execution_runs` stores one instruction and decision snapshot per attempt. `execution_run_events` stores events with an increasing sequence and a unique event ID within each run. The repository exposes active runs and ordered event history.

When used through `ExecutionLifecycleRepository`, a single SQLite transaction updates the ticket and run together at start and finish. Heartbeats are recorded against the matching running ticket and execution ID. `ExecutionRecoveryRepository` rechecks the latest heartbeat inside a transaction before marking a stale run `interrupted` and its running ticket `failed`.
