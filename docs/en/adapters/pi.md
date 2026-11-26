# Pi Adapter

`adapters/pi.Executor` implements `ports.Executor` by starting Pi CLI in RPC mode for each ticket run.

## Process and workspace

The executor starts `pi --mode rpc --no-session`, passes a selected tool list, and disables MCP and extensions unless enabled in configuration. The default tools are `read`, `grep`, `find`, `ls`, `bash`, `edit`, and `write`. Provider, model, environment overrides, heartbeat interval, shutdown timeout, and maximum RPC record size are configurable.

The per-run `ExecutionInput.Workspace.Directory` takes precedence over the configured fallback directory. The directory becomes the process working directory and prompt boundary; it is not an operating-system sandbox. The executor prompt asks Pi to keep file operations inside that directory and forbids commits, pushes, merges, deployment, and unrelated network calls.

## RPC and result mapping

The executor sends the ticket and validated decision to Pi, waits for the prompt response and `agent_settled`, and requests the final assistant text. Tool events become progress events; a heartbeat is sent every 20 seconds by default while waiting. RPC records and captured stderr are size-limited.

The final response must include a valid `NeskoResult` JSON line with `completed`, `needs_review`, or `waiting_human` and a non-empty summary. Missing or malformed result data maps to `needs_review`. The process is closed and waited on before the result is returned.
