# Judge Adapter

The `adapters/judge` package implements `ports.Judge` with a model-backed assessor and two completion clients.

## ModelJudge

`ModelJudge` sends the ticket ID, channel, request text, and optional context from an `AssessmentContextProvider` to a `CompletionClient`. Without project context, its prompt asks the model to mark repository-dependent dimensions unknown. The serialized input is limited to 128 KiB by default.

The system prompt treats ticket and repository context as untrusted data, requires a single structured JSON assessment, and forbids returning a handling route. The decoder rejects unknown fields, trailing JSON, unsupported values, and invalid evidence before returning a `JudgeAssessment`. The Judge assesses risk; `service.DecisionPolicy` selects the handling mode.

## Completion clients

`OpenAICompatibleClient` sends a non-streaming Chat Completions request to a caller-configured endpoint, model, and optional API key. It applies a timeout when it creates the HTTP client and caps response bodies at 2 MiB.

`OpenAIResponsesClient` calls the OpenAI Responses API with a caller-provided `AccessTokenSource`. Requests use `store: false` and streaming output; the client accepts text only after `response.completed`. Incomplete, failed, malformed, oversized, or text-free responses return errors.
