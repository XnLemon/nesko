# 飞书消息接入与工单记录

## 组件

- `adapters/feishu.Gateway` 通过飞书开放平台 Go SDK 的长连接接收 `im.message.receive_v1` 事件。`GatewayConfig` 由调用方提供飞书凭据及消息接收策略。
- `normalizeMessage` 检查消息类型、发信人、会话和机器人提及规则，再将消息转换为领域层的 `IncomingRequest`。
- `service.ReceiveRequest` 创建或读取工单，并通过 `ports.MessageReplier` 端口确认接收。
- `adapters/sqlite.TicketRepository` 通过 `database/sql` 和纯 Go SQLite 驱动保存工单；数据库路径由调用方提供。
- `adapters/feishu.Messenger` 通过飞书消息回复接口在原消息下回复工单编号。
- 根包 `nesko` 定义渠道无关的请求与工单类型。调用方负责组装仓储、消息回复器和应用服务。

## Judge 与执行器

`ports.Judge` 只评估工单，不选择执行路线。`JudgeAssessment` 包含复杂度、兼容性/数据/安全/运行影响/影响范围风险、可回滚性、置信度、理由和证据。每项风险与复杂度分开记录。

`adapters/judge.ModelJudge` 通过 `CompletionClient` 调用模型，并校验结构化评估。`OpenAICompatibleClient` 使用调用方提供的 Chat Completions endpoint、模型名和可选 API key。`OpenAIResponsesClient` 通过调用方提供的 `AccessTokenSource` 使用 OpenAI 官方 Responses API；请求设置 `store: false` 和 `stream: true`，通过 `instructions` 传入 Judge 提示词，并且只有收到 `response.completed` 后才接受结果。`adapters/openai.OAuthTokenSource` 实现 Sign in with ChatGPT，包含 PKCE、本机回调、OIDC ID token 校验、plan 使用权限 scope 校验和轮换 refresh token；默认将凭据保存到操作系统用户配置目录下的受保护文件，也允许调用方指定路径。Pi 使用独立的 `openai-codex` OAuth 登录。两边授权为同一个 ChatGPT 账号时，请求共享该账号的 plan 用量。上下文提供器可以为评估附加只读项目材料。模型输入默认最多 128 KiB。没有提供项目上下文时，Judge 会被要求将依赖仓库信息的风险标记为 `unknown`；core 不会自行扫描代码库。

复杂度和风险由接入的 Judge 按分类给出；当前 core 不扫描代码计算分数。低复杂度表示边界清楚、沿用现有模式且容易验证；中复杂度通常涉及多个组件或需要审批的集成；高复杂度通常跨子系统、改变架构或难以验证。低风险表示影响范围小且容易回滚；中风险表示影响可控但需要审批或专门验证；高风险表示可能破坏公共契约、影响安全/生产数据、造成广泛影响或难以回滚。信息不足时使用 `unknown`，并由策略转人工。

`service.DecisionPolicy` 根据评估结果和工单上下文选择处理路线。默认策略仅在低复杂度、所有风险维度均低、容易回滚、置信度高且至少有一条具体证据时允许自动处理。中等复杂度、风险、置信度或可控回滚成本进入 `awaiting_approval`；高复杂度、高风险、难以回滚、低置信度、未知维度或证据不足进入 `waiting_human`。

`service.TicketWorkflow` 通过 `ports.TicketWorkflow` 向调用方 supervisor 暴露分步的 `Assess`、`ResumeAndAssess` 和 `Execute` 方法。`Assess` 保存 Judge 评估和策略决策，并返回 `queued`、`awaiting_approval` 或 `waiting_human`，不会启动执行器。`Triage` 和 `Resume` 是便捷流程，会自动执行进入队列的工单。`ApprovePlan` 只将工单转入 `queued`，何时执行由调用方决定。补充的人类信息会追加到工单请求后再评估。

`ports.ExecutorRouter` 根据执行请求选择具体执行器；`ports.ExecutorRouterFunc` 可将路由函数适配为该接口。可选的 `ports.WorkspaceManager` 为一次执行准备调用方拥有的 workspace，并在执行后释放；目录选择和仓库操作由 manager 实现，core 将选定目录传给执行器。`adapters/pi.Executor` 优先使用每次执行传入的 workspace，否则使用配置的固定工作目录。它启动 `pi --mode rpc --no-session`，默认启用 `read`、`grep`、`find`、`ls`、`bash`、`edit` 和 `write` 工具，并关闭 MCP 与扩展。执行器等待 `agent_settled` 后读取最后一条助手文本，根据 `NeskoResult` 结果行生成终态；缺失或格式错误时按 `needs_review` 处理。Pi 工具事件会转为工单进度，执行等待期间按默认 20 秒间隔发送心跳。

工作目录是 Pi 进程的当前目录与提示词约束，并非操作系统级沙箱。Pi executor 负责本地代码执行与结果回报。工单工作流不编排 GitHub 操作。`adapters/github.Client` 提供仓库查询、fork 创建、issue 与评论操作，以及 PR 操作，包括提交 review、请求 reviewer、更新分支和显式合并。仓库、分支、合并方式和操作时机由调用方选择；该适配器不应用合并或审批策略。本地分支、工作树、commit 和 push 由调用方的 workspace 与 Git 工具负责。

`ports.Executor` 接收带执行 ID 的工单，并通过回调报告心跳和进度。配置 `ports.ExecutionRunRepository` 后，工作流会为每次执行保存不可变的指令与 Judge 决策快照，以及有序的启动、workspace、心跳、进度和终态事件。Run 查询包含最近心跳，SQLite 也提供活跃 Run 查询，供调用方 supervisor 管理。`ports.ExecutionLifecycleRepository` 允许适配器原子地启动工单与 Run，并原子保存 completed、review、waiting-human 和 failed 结果。工单本身保存当前状态、评估和策略决策、当前执行 ID、最近心跳和进度、结果摘要及失败原因。

`ports.ExecutionRecoveryRepository` 提供原子化的过期 Run 中断操作。调用方传入心跳截止时间和原因；SQLite 会在同一事务内重新检查最近心跳，并将 Run 标记为 `interrupted`、对应的运行中工单标记为 `failed`。core 不负责安排检查，也不决定重试还是转人工。未实现 `ExecutionLifecycleRepository` 的适配器会分别写入工单和 Run；若进程在两次写入之间退出，调用方需根据 Run 历史核对状态。

`samples/core-loop` 组合真实 Judge、Pi、SQLite，以及可选的飞书网关。`samples/core-integration` 使用确定性的进程内 Judge、executor 和消息回复器，验证 intake、工作流、生命周期仓储、有序事件及过期 Run 恢复。

工单状态由比较并交换式更新保护，只有允许的状态迁移且当前状态匹配时才会写入。SQLite 初始化会为既有 `tickets` 表添加新的生命周期列，不会重建已有工单表。

## 消息处理

网关只接受用户发送的文本消息。单聊文本可以创建工单；群聊文本必须提及配置的机器人 Open ID。配置 `FEISHU_ALLOWED_OPEN_IDS` 后，只有列表中的发信人会被接受；列表为空时，应用不再执行额外的发信人筛选。

每条请求包含渠道、飞书消息 ID、会话 ID、请求人 Open ID、文本内容和接收时间。机器人及其他非用户消息、非文本消息、无法解析的内容和不符合群聊提及规则的消息不会创建工单。

## 工单存储与确认

SQLite 的 `tickets` 表保存工单编号、内部 ID、来源渠道与消息、会话、请求人、请求文本、状态、创建时间和确认消息 ID。渠道与来源消息 ID 组成唯一键，因此飞书重复投递同一消息时会复用原工单。

新工单的状态为 `received`。确认回复携带基于来源消息 ID 生成的幂等 UUID；回复成功后，数据库保存飞书返回的确认消息 ID。数据库使用 WAL 模式。应用逻辑依赖 `TicketRepository` 与 `MessageReplier` 接口，具体飞书和 SQLite 实现位于适配器目录。
