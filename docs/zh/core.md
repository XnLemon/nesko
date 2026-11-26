# Core 领域模型

根包 `nesko` 定义渠道无关的数据类型和生命周期规则，不执行网络、数据库、模型或进程 I/O。

## 请求与工单

`IncomingRequest` 包含渠道、来源消息 ID、会话 ID、请求人 ID、文本和接收时间。`Validate` 要求路由与幂等识别字段齐全，且文本非空。当前 Feishu 渠道标识为 `ChannelFeishu`。

`Ticket` 是持久化的工作单元，记录来源请求、确认消息、当前状态、Judge 与策略决策、活跃执行 ID、最近心跳与进度、结果摘要和失败原因。`FormatTicketReference` 生成面向用户的 `TKT-000001` 格式编号。

工单状态包括 `received`、`triaging`、`queued`、`running`、`awaiting_approval`、`needs_review`、`waiting_human`、`completed`、`failed` 和 `cancelled`。`CanTransitionTicket` 定义允许的状态迁移。仓储实现会依据预期当前状态应用迁移。

## 决策模型

`JudgeAssessment` 将复杂度与风险分开记录。风险维度包括兼容性、数据影响、安全、运行影响和影响范围；同时记录可回滚性、置信度、理由和证据。校验会拒绝不支持的枚举值和不完整证据。

`PolicyDecision` 保存 `automatic`、`requires_approval` 或 `human_in_loop` 之一及其理由。`TicketDecision` 组合评估与策略结果。领域层强制自动处理必须同时满足低复杂度、所有风险维度均低、容易回滚、高置信度和至少一项证据。领域层校验这个安全边界，但不选择具体策略。

## 执行记录

`ExecutionInput` 将工单、执行 ID 和可选的调用方 workspace 传给执行器。`ExecutionResult` 返回 `completed`、`needs_review` 或 `waiting_human` 及结果摘要。

`ExecutionRun` 保存每次执行的不可变指令与决策快照，以及状态、时间和终态结果。`ExecutionEvent` 表示执行启动、workspace 生命周期、心跳、进度和终态事件；仓储通过 `ExecutionRunEvent` 增加事件顺序。

Core 领域层不选择仓库、不准备目录、不运行 Git 命令、不调用 GitHub、不调度 supervisor，也不决定是否需要人工审批。这些行为通过服务、ports 和调用方策略组合。
