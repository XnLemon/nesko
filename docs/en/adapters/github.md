# GitHub Adapter

`adapters/github.Client` wraps GitHub REST operations with `go-github`. Callers configure a static token or an OAuth token source, an optional HTTP client, and optional API/upload URLs for GitHub Enterprise. The client uses contexts, defaults list pages to 30 items, and caps page size at 100.

## Remote repository work

The client supports repository and authenticated-user lookup, fork creation, issue lookup/list/update, issue comments, and pull-request lookup/list/create/update. Pull-request operations also include changed files and commits, submitted and pending reviews, inline review comments and replies, reviewer requests/removals, branch updates, and explicit merge requests.

Branch updates and merges require the caller to provide the expected PR head SHA. This guards against applying an operation to a head that changed after the caller inspected the PR. Merge method is explicitly selected by the caller.

The adapter returns typed `go-github` response values. Fork creation can be accepted while GitHub prepares the fork; the caller can use the returned repository metadata and retry lookup until it is ready.

## CI and webhooks

Check-run listing and combined commit status lookup expose CI state. GitHub Actions operations include run and attempt details, job listing, short-lived log URLs, workflow dispatch, rerunning workflows or failed jobs, rerunning one job, and cancelling a run. Log URLs should be treated as credentials and omitted from logs.

`WebhookReceiver` implements `http.Handler`. It accepts POST requests, requires and verifies the GitHub SHA-256 signature, parses recognized event types into typed payloads, and passes event type, delivery ID, and payload to a caller handler. The default body limit is 25 MiB. An optional event allowlist returns 204 for unselected events; successful handling returns 202 and handler errors return 500. The receiver does not persist or deduplicate deliveries. The caller uses delivery ID as an idempotency key and should enqueue long-running work.

The caller selects repositories, fork strategy, local workspace, Git operations, CI actions, review policy, and merge timing. The adapter performs only explicit remote API calls and does not implement approval, retry, supervisor, or automatic-merge policy. Local clone/worktree/commit/push operations use the caller's `WorkspaceManager` and Git tooling.
