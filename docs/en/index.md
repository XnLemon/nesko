# Nesko Architecture

These pages describe the current implementation. Nesko is split into a channel-independent domain package, application services, ports, and adapters. A caller composes these pieces; repository selection, supervision, scheduling, retry policy, and GitHub merge policy remain caller-owned.

## Core and application layers

- [Core domain](core.md): request, ticket, decision, and execution records.
- [Ports](ports.md): contracts used by services and implemented by adapters.
- [Service internals](service.md): intake, Judge routing, approvals, execution, and recovery boundaries.

## Adapters

- [Feishu](adapters/feishu.md): long-connection message intake and replies.
- [GitHub](adapters/github.md): repository, issue, pull-request, checks, Actions, and webhook APIs.
- [Judge](adapters/judge.md): structured risk assessment and model clients.
- [OpenAI OAuth](adapters/openai.md): Sign in with ChatGPT credentials for Judge requests.
- [Pi](adapters/pi.md): process-based coding executor.
- [SQLite](adapters/sqlite.md): ticket and execution lifecycle persistence.

## Composition boundary

`service` depends on the domain types and ports. Adapters implement or expose those contracts around external systems. Samples wire concrete implementations together. The core has no built-in supervisor and does not automatically create Git branches or pull requests, schedule retries, or merge changes.
