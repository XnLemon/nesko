# Feishu Message Intake and Ticket Recording

## Components

- `adapters/feishu.Gateway` receives `im.message.receive_v1` events through the Feishu Open Platform Go SDK long connection. The caller supplies Feishu credentials and message policy through `GatewayConfig`.
- `normalizeMessage` checks message type, sender, conversation, and bot-mention rules before converting a message into the domain `IncomingRequest`.
- `application.ReceiveRequest` creates or loads a ticket and acknowledges receipt through the `ports.MessageReplier` port.
- `adapters/sqlite.TicketRepository` stores tickets through `database/sql` and a pure Go SQLite driver; the caller supplies the database path.
- `adapters/feishu.Messenger` replies to the source message with the ticket reference through the Feishu message API.
- The root `nesko` package defines channel-independent request and ticket types. The caller composes the repository, message replier, and application service.

## Message handling

The gateway accepts text messages sent by users. Direct-message text can create a ticket; group-chat text must mention the configured bot Open ID. When `FEISHU_ALLOWED_OPEN_IDS` is configured, only listed senders are accepted. An empty list applies no additional sender filter in the application.

Each request contains the channel, Feishu message ID, conversation ID, requester Open ID, text, and receipt time. Bot messages, other non-user messages, non-text messages, unparseable content, and group messages that do not satisfy the mention rule do not create tickets.

## Ticket storage and acknowledgement

The SQLite `tickets` table stores the ticket number, internal ID, source channel and message, conversation, requester, request text, status, creation time, and acknowledgement message ID. The channel and source message ID form a unique key, so a repeated Feishu delivery reuses the original ticket.

New tickets have the `received` status. The acknowledgement uses an idempotency UUID derived from the source message ID; after a successful reply, the database stores the acknowledgement message ID returned by Feishu. The database uses WAL mode. Application logic depends on the `TicketRepository` and `MessageReplier` interfaces, with concrete Feishu and SQLite implementations in the adapters directory.
