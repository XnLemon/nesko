# Nesko 架构

这些页面描述当前已经实现的结构。Nesko 分为渠道无关的领域包、应用服务、ports 和 adapters。调用方负责组装这些部分；仓库选择、supervisor、调度、重试策略和 GitHub 合并策略由调用方持有。

## Core 与应用层

- [Core 领域模型](core.md)：请求、工单、决策和执行记录。
- [Ports 契约](ports.md)：服务使用、适配器实现的接口。
- [Service 内部流程](service.md)：接单、Judge 路由、审批、执行和恢复边界。

## Adapters

- [Feishu](adapters/feishu.md)：长连接消息接入与回复。
- [GitHub](adapters/github.md)：仓库、issue、PR、checks、Actions 和 webhook API。
- [Judge](adapters/judge.md)：结构化风险评估与模型客户端。
- [OpenAI OAuth](adapters/openai.md)：供 Judge 请求使用的 Sign in with ChatGPT 凭据。
- [Pi](adapters/pi.md)：通过进程运行的代码执行器。
- [SQLite](adapters/sqlite.md)：工单与执行生命周期持久化。

## 组装边界

`service` 依赖领域类型和 ports。Adapters 围绕外部系统实现这些契约或提供对应能力。Samples 负责组装具体实现。Core 不内置 supervisor，也不会自动创建 Git 分支或 PR、安排重试或合并变更。
