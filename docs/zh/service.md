# Service 内部设计

`service` 包基于 ports 实现请求接入、决策路由和工单执行。包内不包含渠道客户端、模型客户端、数据库、Git 客户端或 supervisor 调度器。

## 请求接入

`ReceiveRequest.Receive` 校验 `IncomingRequest`，调用 `TicketRepository.CreateOrGet` 并返回持久化工单。仓储按渠道和来源消息 ID 识别工单。如果工单已有确认消息 ID，service 将其作为重复投递处理，不会再次回复。

新确认会调用 `MessageReplier`，传入来源渠道/会话/消息、格式化后的工单编号，以及根据渠道与消息 ID 生成的确定性幂等键。只有发送成功后才保存回复返回的消息 ID。

## 评估与策略

`TicketWorkflow.Assess` 将 `received` 或 `failed` 工单迁移到 `triaging`，调用配置的 `Judge` 获取评估并校验，再由 `DecisionPolicy` 选择路线；校验组合后的决策后，将决策与下一工单状态一起保存。

`DefaultDecisionPolicy` 将未知维度、低置信度或缺少证据的任务送入 `waiting_human`。高复杂度、高风险或难以回滚也需要人工处理。证据完整、复杂度和所有风险均低、容易回滚且高置信度的任务进入 `queued`，决策为 `automatic`；其余可处理的中等情况进入 `awaiting_approval`。领域层还会独立拒绝不安全的自动决策。

`Assess` 只返回决策，不运行执行器。`Triage` 仅在结果为 `queued` 时调用 `Execute`。`ResumeAndAssess` 将人工补充内容追加到 `waiting_human` 工单并重新评估；`Resume` 在重新评估后进入队列时执行。`ApprovePlan` 只将 `awaiting_approval` 改为 `queued`，执行时机由调用方决定。`ApproveReview` 将 `needs_review` 工单标记为完成；`RejectReview` 会取消处于任一 review gate 的工单。

## 执行与持久化

`Execute` 要求工单处于 queued 且保存了有效决策。它启动执行记录，可选地准备 workspace，将输入路由到执行器，持久化心跳/进度回调，释放 workspace，并将执行结果映射为工单终态。执行器或持久化失败时，会尽力记录失败状态。

配置 `ExecutionLifecycleRepository` 后，工单与执行记录的启动/结束状态原子提交。否则工单和执行记录分开写入；进程在两次写入之间退出时，调用方需要核对状态。`ExecutionRunRepository` 为可选项，提供不可变执行快照和有序生命周期事件。`WorkspaceManager` 也是可选项，其 release 会收到执行结果和执行错误。

失败状态和 workspace 清理记录使用不受调用方取消影响的短时 context。Service 将 `ExecutionRecoveryRepository` 暴露给调用方以原子中断过期执行；本包不运行调度器、不选择心跳超时、不自动重试，也不决定中断后是否需要人工介入。
