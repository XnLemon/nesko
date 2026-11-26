# Feishu Message Intake and Ticket Recording

## Components

- `adapters/feishu.Gateway` receives `im.message.receive_v1` events through the Feishu Open Platform Go SDK long connection. The caller supplies Feishu credentials and message policy through `GatewayConfig`.
- `normalizeMessage` checks message type, sender, conversation, and bot-mention rules before converting a message into the domain `IncomingRequest`.
- `service.ReceiveRequest` creates or loads a ticket and acknowledges receipt through the `ports.MessageReplier` port.
- `adapters/sqlite.TicketRepository` stores tickets through `database/sql` and a pure Go SQLite driver; the caller supplies the database path.
- `adapters/feishu.Messenger` replies to the source message with the ticket reference through the Feishu message API.
- The root `nesko` package defines channel-independent request and ticket types. The caller composes the repository, message replier, and service layer.

## Judge and executor

`ports.Judge` assesses a ticket without choosing its route. `JudgeAssessment` records complexity, compatibility/data/security/operational/blast-radius risks, reversibility, confidence, rationale, and evidence. Complexity and risk remain separate dimensions.

`adapters/judge.ModelJudge` calls a model through `CompletionClient` and validates its structured assessment. `OpenAICompatibleClient` uses a caller-supplied Chat Completions endpoint, model name, and optional API key. `OpenAIResponsesClient` uses the public OpenAI Responses API with a caller-provided `AccessTokenSource`; it sends `store: false` and `stream: true`, uses `instructions` for the Judge prompt, and accepts a result only after `response.completed`. `adapters/openai.OAuthTokenSource` implements Sign in with ChatGPT using PKCE, a loopback callback, OIDC ID-token validation, required plan-usage scope checks, and rotating refresh tokens. It stores credentials in a protected file under the OS user config directory by default; callers can choose another path. Pi uses its separate `openai-codex` OAuth login. When both are authorized with the same ChatGPT account, their requests consume that account's shared plan usage. An assessment context provider can attach read-only project material. Model input is limited to 128 KiB by default. Without project context, the Judge is instructed to mark repository-dependent risks as `unknown`; the core does not scan the repository itself.

The integrated Judge assigns these categorical values; the core does not scan source code to calculate a numeric score. Low complexity means a bounded change that follows an existing pattern and is easy to verify. Medium complexity usually spans components or needs an approved integration. High complexity usually crosses subsystems, changes architecture, or is difficult to verify. Low risk means a small blast radius and easy rollback. Medium risk means a controlled impact that needs approval or targeted validation. High risk may break a public contract, affect security or production data, have broad impact, or be difficult to roll back. Insufficient information is recorded as `unknown` and routed to a human by policy.

`service.DecisionPolicy` selects a route from the assessment and ticket context. The default policy allows automatic handling only when complexity and every risk dimension are low, rollback is easy, confidence is high, and at least one concrete evidence item is present. Medium complexity, risk, confidence, or controlled rollback requires `awaiting_approval`; high complexity, high risk, difficult rollback, low confidence, unknown dimensions, or insufficient evidence requires `waiting_human`.

`service.TicketWorkflow` exposes stepwise `Assess`, `ResumeAndAssess`, and `Execute` methods through `ports.TicketWorkflow` for caller-owned supervisors. `Assess` persists the Judge assessment and policy decision and returns `queued`, `awaiting_approval`, or `waiting_human` without starting the executor. `Triage` and `Resume` are convenience methods that execute automatically queued tickets. `ApprovePlan` only moves a ticket into `queued`; the caller starts execution when appropriate. Additional human input is appended to the ticket request before reassessment.

`ports.ExecutorRouter` selects an executor for each execution request; `ports.ExecutorRouterFunc` adapts a routing function to that interface. An optional `ports.WorkspaceManager` prepares a caller-owned workspace for a run and releases it after execution. The manager chooses directories and repository operations; the core passes the selected directory to the executor. `adapters/pi.Executor` uses the per-run workspace when provided and otherwise falls back to its configured working directory. It starts `pi --mode rpc --no-session`, enables the `read`, `grep`, `find`, `ls`, `bash`, `edit`, and `write` tools by default, and disables MCP and extensions. The executor waits for `agent_settled`, retrieves the last assistant text, and maps a `NeskoResult` line to a terminal outcome; a missing or malformed result maps to `needs_review`. Pi tool events become ticket progress, and the executor sends a heartbeat every 20 seconds by default while waiting.

The working directory sets Pi's process directory and prompt boundary; it is not an operating-system sandbox. The Pi executor performs local coding work and reports its result. The ticket workflow does not orchestrate GitHub operations. `adapters/github.Client` exposes repository, fork, issue, comment, pull-request, review, check, and Actions operations; `WebhookReceiver` validates and parses incoming events. Callers explicitly choose repositories, branches, reruns, dispatches, cancellation, merge method, and timing. This adapter applies no approval, retry, or merge policy. Local branch creation, worktrees, commits, and pushes belong to the caller's workspace and Git tooling. The adapter's current operations are described in [GitHub adapter](github-adapter.md).

`ports.Executor` receives a ticket with an execution ID and reports heartbeats and progress through a callback. When `ports.ExecutionRunRepository` is configured, the workflow stores one immutable instruction and Judge-decision snapshot per run, plus ordered start, workspace, heartbeat, progress, and terminal events. Run queries include the latest heartbeat and SQLite exposes active runs for caller-managed supervision. `ports.ExecutionLifecycleRepository` lets an adapter atomically start a run with its ticket and atomically persist completed, review, waiting-human, and failed outcomes. Ticket state retains the current status, assessment and policy decision, active execution ID, latest heartbeat and progress, result summary, and failure reason.

`ports.ExecutionRecoveryRepository` provides an atomic stale-run interruption operation. The caller supplies the heartbeat cutoff and reason; SQLite checks the latest heartbeat in the same transaction that marks the run `interrupted` and its matching running ticket `failed`. The core does not schedule checks or choose whether the caller should retry or request human input. Adapters that do not provide `ExecutionLifecycleRepository` use separate ticket and run writes and require caller-side reconciliation if the process stops between them.

`samples/core-loop` composes the live Judge, Pi, SQLite, and optional Feishu gateway adapters. `samples/core-integration` exercises intake, workflow, lifecycle storage, ordered events, and stale-run recovery with deterministic in-process Judge, executor, and replier implementations.

Ticket status changes use compare-and-set updates, which require both an allowed transition and the expected current status. SQLite initialization adds lifecycle columns to an existing `tickets` table without rebuilding existing tickets.

## Message handling

The gateway accepts text messages sent by users. Direct-message text can create a ticket; group-chat text must mention the configured bot Open ID. When `FEISHU_ALLOWED_OPEN_IDS` is configured, only listed senders are accepted. An empty list applies no additional sender filter in the gateway.

Each request contains the channel, Feishu message ID, conversation ID, requester Open ID, text, and receipt time. Bot messages, other non-user messages, non-text messages, unparseable content, and group messages that do not satisfy the mention rule do not create tickets.

## Ticket storage and acknowledgement

The SQLite `tickets` table stores the ticket number, internal ID, source channel and message, conversation, requester, request text, status, creation time, and acknowledgement message ID. The channel and source message ID form a unique key, so a repeated Feishu delivery reuses the original ticket.

New tickets have the `received` status. The acknowledgement uses an idempotency UUID derived from the source message ID; after a successful reply, the database stores the acknowledgement message ID returned by Feishu. The database uses WAL mode. Application logic depends on the `TicketRepository` and `MessageReplier` interfaces, with concrete Feishu and SQLite implementations in the adapters directory.
