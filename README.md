# Nesko Core

Nesko is a reusable Go core for building an IM-driven coding agent. It provides a normalized request domain, typed ticket lifecycle, request-intake and ticket-workflow services, a policy-controlled handling route, persistence and messaging, and Feishu, SQLite, model-Judge, and Pi-executor adapters.

The core receives Feishu text messages, applies sender and group-mention policy, records accepted requests as tickets, and acknowledges them in the source conversation. Its ticket workflow separates Judge risk assessment from policy routing, then routes eligible requests through an `ExecutorRouter`. Callers can drive the workflow step by step, provide a `WorkspaceManager` for per-run directories, and use execution lifecycle and recovery ports to track or reconcile work without embedding a supervisor policy in the core. `ExecutionRunRepository` stores immutable instruction/decision snapshots and ordered events. The Judge supports OpenAI-compatible Chat Completions with API keys and the OpenAI Responses API with a caller-provided OAuth token source. `adapters/openai` provides Sign in with ChatGPT OAuth; Pi keeps its own `openai-codex` login. When both are authorized with the same ChatGPT account, their requests use the account's shared plan usage. `adapters/github` exposes repository, fork, issue, comment, and pull-request operations through the GitHub REST API. Applications supply project context, choose the executor route, and can replace the default policy. Automatic handling requires low complexity, low risks, easy rollback, high confidence, and concrete evidence.

Architecture documentation is maintained in [`docs/zh/`](docs/zh/) and [`docs/en/`](docs/en/).

The [`samples/core-loop/`](samples/core-loop/) directory shows a live caller composition for ticket intake, Judge assessment, policy routing, Pi execution, and SQLite persistence. [`samples/core-integration/`](samples/core-integration/) runs an offline end-to-end example of intake, assessment, execution, event history, and stale-run recovery.

The [`samples/github-adapter/`](samples/github-adapter/) directory shows read-only repository and pull-request inspection through the GitHub adapter.
