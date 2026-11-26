# Ports 契约

`ports` 包定义应用服务与外部实现之间的契约。接口使用 `context.Context` 支持取消，并使用根包 `nesko` 的类型承载领域数据。

## 接单与工单生命周期

- `TicketRepository` 创建或读取工单、确认回复、按比较并交换方式迁移状态、追加人工输入、记录执行心跳/进度，并释放仓储资源。
- `MessageReplier` 回复来源消息并接收幂等键。
- `TicketWorkflow` 向调用方 supervisor 暴露评估、分流、恢复、审批、拒绝和执行步骤。
- `Judge` 评估工单并返回 `JudgeAssessment`，不选择处理路线。

## 执行

- `Executor` 运行一次 `ExecutionInput`，通过 `ExecutionReporter` 报告 `ExecutionEvent`，并返回 `ExecutionResult`。
- `ExecutorRouter` 为每次输入选择 `Executor`；`ExecutorRouterFunc` 将函数适配到该契约。
- `WorkspaceManager` 在一次执行前准备调用方拥有的目录，并在执行后释放。成功 prepare 后总会调用 release，包括执行失败的情况。

## 持久化执行历史

- `ExecutionRunRepository` 保存执行快照、有序事件、终态结果和活跃执行查询。
- `ExecutionLifecycleRepository` 可选地原子提交工单与执行的启动和结束状态。
- `ExecutionRecoveryRepository` 根据调用方选择的截止时间原子中断过期执行。该端口不负责安排检查或选择重试操作。

SQLite 实现工单、执行、生命周期和恢复仓储契约。Feishu 实现 `MessageReplier`；Pi 实现 `Executor`；模型 Judge 实现满足 `Judge`。GitHub 在 adapter 包直接提供远端 GitHub 操作，不实现工单工作流或 workspace 端口。

没有 port 拥有 supervisor 调度、仓库选择、重试规则或自动合并策略。
