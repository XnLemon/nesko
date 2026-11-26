# Core Domain

The root `nesko` package defines channel-independent data and lifecycle rules. It performs no network, database, model, or process I/O.

## Request and ticket

`IncomingRequest` contains a channel, source message ID, conversation ID, requester ID, text, and receipt time. `Validate` requires the routing and deduplication fields plus non-empty text. `ChannelFeishu` is the current Feishu channel identifier.

`Ticket` is the durable unit of work. It records the source request, acknowledgement, current status, Judge and policy decision, active execution ID, recent heartbeat and progress, result summary, and failure reason. `FormatTicketReference` produces the user-facing `TKT-000001` form.

Ticket states are `received`, `triaging`, `queued`, `running`, `awaiting_approval`, `needs_review`, `waiting_human`, `completed`, `failed`, and `cancelled`. `CanTransitionTicket` defines the allowed transitions. Repository implementations apply transitions against an expected current state.

## Decision model

`JudgeAssessment` keeps complexity separate from risk. Risk covers compatibility, data impact, security, operational impact, and blast radius; reversibility, confidence, rationale, and evidence are recorded alongside them. Validation rejects unsupported enum values and incomplete evidence.

`PolicyDecision` stores one of `automatic`, `requires_approval`, or `human_in_loop` with a rationale. `TicketDecision` combines the assessment and policy result. The domain enforces that automatic handling requires low complexity, every risk dimension low, easy rollback, high confidence, and at least one evidence item. The domain validates this boundary but does not choose the policy.

## Execution records

`ExecutionInput` passes a ticket, execution ID, and optional caller-owned `Workspace` to an executor. `ExecutionResult` returns `completed`, `needs_review`, or `waiting_human` with a summary.

`ExecutionRun` stores the immutable instruction and decision snapshot for an attempt, plus status, timestamps, and terminal result. `ExecutionEvent` represents run start, workspace lifecycle, heartbeat, progress, and terminal events; repositories add event ordering in `ExecutionRunEvent`.

The core domain does not select repositories, prepare directories, run Git commands, call GitHub, schedule supervision, or decide whether a human approval is required. Those behaviors are composed through services, ports, and caller-owned policy.
