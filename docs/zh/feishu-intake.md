# 飞书消息接入与工单记录

## 组件

- `adapters/feishu.Gateway` 通过飞书开放平台 Go SDK 的长连接接收 `im.message.receive_v1` 事件。`GatewayConfig` 由调用方提供飞书凭据及消息接收策略。
- `normalizeMessage` 检查消息类型、发信人、会话和机器人提及规则，再将消息转换为领域层的 `IncomingRequest`。
- `application.ReceiveRequest` 创建或读取工单，并通过 `ports.MessageReplier` 端口确认接收。
- `adapters/sqlite.TicketRepository` 通过 `database/sql` 和纯 Go SQLite 驱动保存工单；数据库路径由调用方提供。
- `adapters/feishu.Messenger` 通过飞书消息回复接口在原消息下回复工单编号。
- 根包 `nesko` 定义渠道无关的请求与工单类型。调用方负责组装仓储、消息回复器和应用服务。

## 消息处理

网关只接受用户发送的文本消息。单聊文本可以创建工单；群聊文本必须提及配置的机器人 Open ID。配置 `FEISHU_ALLOWED_OPEN_IDS` 后，只有列表中的发信人会被接受；列表为空时，应用不再执行额外的发信人筛选。

每条请求包含渠道、飞书消息 ID、会话 ID、请求人 Open ID、文本内容和接收时间。机器人及其他非用户消息、非文本消息、无法解析的内容和不符合群聊提及规则的消息不会创建工单。

## 工单存储与确认

SQLite 的 `tickets` 表保存工单编号、内部 ID、来源渠道与消息、会话、请求人、请求文本、状态、创建时间和确认消息 ID。渠道与来源消息 ID 组成唯一键，因此飞书重复投递同一消息时会复用原工单。

新工单的状态为 `received`。确认回复携带基于来源消息 ID 生成的幂等 UUID；回复成功后，数据库保存飞书返回的确认消息 ID。数据库使用 WAL 模式。应用逻辑依赖 `TicketRepository` 与 `MessageReplier` 接口，具体飞书和 SQLite 实现位于适配器目录。
