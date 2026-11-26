# Judge 适配器

`adapters/judge` 包通过模型评估器和两个 completion client 实现 `ports.Judge`。

## ModelJudge

`ModelJudge` 将工单 ID、渠道、请求文本和 `AssessmentContextProvider` 提供的可选上下文发送给 `CompletionClient`。没有项目上下文时，提示词要求模型将依赖仓库信息的风险维度标记为 unknown。序列化输入默认限制为 128 KiB。

系统提示词将工单和仓库上下文视为不可信数据，要求只返回结构化 JSON 评估，并禁止输出处理路线。解码器拒绝未知字段、尾随 JSON、不支持的值和无效证据，再返回 `JudgeAssessment`。Judge 负责评估风险，`service.DecisionPolicy` 选择处理模式。

## Completion 客户端

`OpenAICompatibleClient` 向调用方配置的 endpoint 发送非流式 Chat Completions 请求，可配置模型和 API key。自行创建 HTTP client 时会设置超时，并将响应体限制为 2 MiB。

`OpenAIResponsesClient` 通过调用方提供的 `AccessTokenSource` 调用 OpenAI Responses API。请求设置 `store: false` 并使用流式输出；只有收到 `response.completed` 后才接受文本。未完成、失败、格式错误、超限或不含文本的响应会返回错误。
