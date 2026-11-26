# Feishu 适配器

Feishu 适配器提供长连接事件网关和 OpenAPI 消息回复器。

## Gateway

`Gateway` 通过飞书开放平台 Go SDK WebSocket 客户端订阅 `im.message.receive_v1`。`GatewayConfig` 提供应用凭据、机器人 Open ID 和可选发信人白名单。`Run` 启动连接，`Close` 停止连接。连接重试和错误通过调用方提供的结构化 logger 记录。

消息标准化逻辑只接受用户发送的文本消息，并要求消息 ID、会话元数据、发信人和可解析的文本内容齐全。单聊消息可直接接收；群聊必须提及已配置的机器人 Open ID。发信人白名单非空时，仅允许列出的 Open ID。机器人 mention token 会从请求文本中移除。标准化请求包含 Feishu 渠道、消息 ID、chat ID、发信人 Open ID、文本和接收时间。

网关将通过校验的请求交给调用方提供的 `RequestHandler`，自身不创建工单。

## Messenger

`Messenger` 使用飞书消息回复 API 实现 `ports.MessageReplier`。它只接受 Feishu 渠道，并要求来源消息、回复文本和幂等键。幂等键作为 Feishu reply UUID 发送。适配器返回新回复的消息 ID，并将 API 错误传给调用方。
