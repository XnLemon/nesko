# Service Internals

The `service` package implements request intake, decision routing, and ticket execution over ports. It does not contain a channel client, model client, database, Git client, or supervisor scheduler.

## Request intake

`ReceiveRequest.Receive` validates an `IncomingRequest`, calls `TicketRepository.CreateOrGet`, and returns the durable ticket. The repository key is channel plus source message ID. If the ticket already has an acknowledgement ID, the service treats the delivery as a duplicate and does not reply again.

For a new acknowledgement, the service calls `MessageReplier` with the source channel/conversation/message, the formatted ticket reference, and a deterministic idempotency key derived from channel and message ID. It records the returned reply ID only after the send succeeds.

## Assessment and policy

`TicketWorkflow.Assess` moves a `received` or `failed` ticket to `triaging`, asks the configured `Judge` for an assessment, validates it, asks `DecisionPolicy` for a route, validates the combined decision, and stores the decision with its next ticket state.

`DefaultDecisionPolicy` routes unknown dimensions, low confidence, or missing evidence to `waiting_human`. High complexity, high risk, or hard rollback also needs a human. Fully evidenced low-complexity, low-risk, easy-to-reverse, high-confidence work is queued as `automatic`; the remaining supported medium cases enter `awaiting_approval`. The domain independently rejects an unsafe automatic decision.

`Assess` returns a decision without running an executor. `Triage` calls `Execute` only when the result is `queued`. `ResumeAndAssess` appends human input to a `waiting_human` ticket and reassesses it; `Resume` executes if reassessment queues the ticket. `ApprovePlan` only changes `awaiting_approval` to `queued`, leaving execution timing to its caller. `ApproveReview` completes a `needs_review` ticket, while `RejectReview` cancels a ticket at either review gate.

## Execution and persistence

`Execute` requires a queued ticket with a valid stored decision. It starts a run, optionally prepares a workspace, routes the input to an executor, persists heartbeat/progress callbacks, releases the workspace, and maps the executor result to a terminal ticket status. An executor or persistence failure records a failed state when possible.

When `ExecutionLifecycleRepository` is supplied, run and ticket start/finish state are committed atomically. Otherwise, ticket and run writes are separate; callers need reconciliation if a process stops between writes. `ExecutionRunRepository` is optional and adds immutable run snapshots and ordered lifecycle events. `WorkspaceManager` is optional and its release receives the executor result and execution error.

Failure and workspace cleanup persistence use a short context detached from caller cancellation. `ExecutionRecoveryRepository` is exposed to callers for atomic stale-run interruption; this package does not run a scheduler, select a heartbeat timeout, retry an execution, or decide whether interruption needs human input.
