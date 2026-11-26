# Ports

The `ports` package defines contracts between application services and external implementations. Interfaces use `context.Context` for cancellable work and use root `nesko` types for domain data.

## Intake and ticket lifecycle

- `TicketRepository` creates or loads a ticket, reads it, acknowledges a reply, applies compare-and-set transitions, appends human input, records execution heartbeat/progress, and closes its resource.
- `MessageReplier` sends a reply to an inbound message and accepts an idempotency key.
- `TicketWorkflow` exposes assessment, triage, resume, approval, rejection, and execution steps to a caller-owned supervisor.
- `Judge` assesses a ticket and returns `JudgeAssessment`; it does not select a route.

## Execution

- `Executor` runs one `ExecutionInput`, reports `ExecutionEvent` values through `ExecutionReporter`, and returns `ExecutionResult`.
- `ExecutorRouter` selects an `Executor` for each input. `ExecutorRouterFunc` adapts a function to this contract.
- `WorkspaceManager` prepares and releases caller-owned directories around one execution. A successful prepare is always paired with release, including failed execution.

## Durable execution history

- `ExecutionRunRepository` stores run snapshots, ordered events, terminal results, and active-run queries.
- `ExecutionLifecycleRepository` optionally commits ticket and run start/finish transitions atomically.
- `ExecutionRecoveryRepository` atomically interrupts a stale run using a cutoff selected by its caller. The port does not schedule checks or choose a retry action.

SQLite implements the ticket, run, lifecycle, and recovery repository contracts. Feishu implements `MessageReplier`; Pi implements `Executor`; model-backed Judge implementations satisfy `Judge`. GitHub exposes remote GitHub operations directly from its adapter package and does not implement ticket workflow or workspace ports.

No port owns supervisor scheduling, repository selection, retry rules, or automatic merge policy.
