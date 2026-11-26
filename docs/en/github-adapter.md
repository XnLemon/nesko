# GitHub Adapter

## Components and boundaries

`adapters/github.Client` wraps the GitHub REST API through `go-github`. Callers configure a static token or an OAuth token source, an optional HTTP client, and optional API/upload URLs for GitHub Enterprise. Requests carry a context. List methods expose page options, default to 30 items, and cap page size at 100.

The client provides:

- Repository lookup, authenticated-user lookup, and fork creation.
- Issue lookup, listing, updating, and issue comments.
- Pull-request lookup, listing, creation, and updates; changed files and commits; submitted and pending review lifecycle, inline review comments and replies; reviewer requests and removals; branch updates; and explicit merge requests guarded by the expected head SHA.
- Check-run listing and combined commit status lookup.
- GitHub Actions run and job lookup, log URL retrieval, workflow dispatch, reruns, failed-job reruns, individual-job reruns, and run cancellation.

The adapter returns the typed response objects from `go-github`; it does not normalize GitHub's full response model into Nesko domain records. API errors include the operation and repository reference.

`WebhookReceiver` is an `http.Handler`. It accepts POST requests, requires the GitHub SHA-256 signature, validates the request body with the configured secret, parses recognized event types into typed `go-github` payloads, and passes the event type, delivery ID, and payload to the caller's handler. The default request-body limit is 25 MiB. An optional event allowlist returns 204 for events that are not selected. A successful handler returns 202; a handler error returns 500. The receiver does not persist or deduplicate deliveries; callers use the delivery ID as their idempotency key. The caller's handler can enqueue work and return promptly while a supervisor processes the event.

## Ownership and flow

The caller maps an incoming request to a repository, chooses whether to create a fork, prepares a local workspace, and runs Git operations such as branch creation, commits, and pushes. The client supplies remote GitHub API operations for issues, PRs, review, CI state, and merge. The workflow service does not invoke these methods automatically.

The caller decides whether to dispatch or rerun CI, cancel a run, request reviewers, submit a review, update a PR branch, or merge a PR. The adapter executes only the explicit operation and does not infer approval from CI state or apply an automatic merge/retry policy. Authentication must have the GitHub permissions required by each endpoint.
