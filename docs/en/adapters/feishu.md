# Feishu Adapter

The Feishu adapter provides a long-connection event gateway and an OpenAPI message replier.

## Gateway

`Gateway` subscribes to `im.message.receive_v1` through the Feishu Open Platform Go SDK WebSocket client. `GatewayConfig` supplies app credentials, the bot Open ID, and an optional sender allowlist. `Run` starts the connection; `Close` stops it. Reconnection and connection errors are reported through the supplied structured logger.

The message normalizer accepts user-authored text messages with IDs, chat metadata, sender, and parseable content. Direct messages are accepted. Group messages require a mention of the configured bot Open ID. When the sender allowlist is non-empty, only listed Open IDs pass. Mention tokens are removed from the request text. The normalized request contains the Feishu channel, message ID, chat ID, sender Open ID, text, and receipt time.

The gateway forwards accepted requests to a caller-provided `RequestHandler`; it does not create tickets itself.

## Messenger

`Messenger` implements `ports.MessageReplier` with the Feishu message reply API. It accepts only the Feishu channel and requires a source message, text, and idempotency key. The key is sent as the Feishu reply UUID. The adapter returns the new reply message ID and surfaces API failures to the caller.
