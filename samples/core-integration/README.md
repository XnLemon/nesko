# Core Integration Sample

Run this sample with `go run ./samples/core-integration`. It needs no API key, Feishu account, or Pi installation. It composes request intake, the ticket workflow, a deterministic Judge and executor, and the SQLite repositories in a temporary directory.

The sample checks two complete paths:

- A normalized request is acknowledged, assessed, executed, and recorded as a completed ticket and run with ordered events.
- A stale active run is interrupted through `ExecutionRecoveryRepository`, which atomically marks its ticket failed.

The live `samples/core-loop` sample can also run with Feishu credentials to exercise the Feishu long connection, acknowledgement, Judge, Pi, and SQLite composition.
