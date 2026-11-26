# GitHub 适配器

`adapters/github.Client` 使用 `go-github` 封装 GitHub REST 操作。调用方配置静态 token 或 OAuth token source、可选 HTTP client，以及 GitHub Enterprise 使用的可选 API/upload URL。客户端请求携带 context，列表默认每页 30 条，最大 100 条。

## 远端仓库操作

客户端支持仓库与当前认证用户查询、创建 fork、issue 查询/列表/更新、issue 评论，以及 PR 查询/列表/创建/更新。PR 操作还包括变更文件与 commit、已提交和待提交 review、行内 review 评论与回复、请求或移除 reviewer、更新分支和显式合并。

更新分支与合并必须由调用方提供预期 PR head SHA，避免对检查之后已变化的 head 执行操作。合并方式由调用方显式选择。

适配器返回 `go-github` 类型化响应。GitHub 可以先接受 fork 请求、稍后才准备好仓库；调用方可使用返回的仓库元数据并重试查询，直到仓库可用。

## CI 与 Webhook

Check run 列表和 commit 综合状态用于读取 CI 状态。GitHub Actions 操作包括 run 与 attempt 详情、job 列表、短时有效的日志 URL、workflow dispatch、重跑 workflow 或失败 job、重跑单个 job 和取消 run。日志 URL 应视作凭据，不应写入日志。

`WebhookReceiver` 实现 `http.Handler`。它接受 POST 请求，要求并校验 GitHub SHA-256 签名，将已识别的事件解析为类型化 payload，并将事件类型、delivery ID 和 payload 交给调用方 handler。请求体默认上限为 25 MiB。可选事件白名单对未选中事件返回 204；handler 成功返回 202，出错返回 500。Receiver 不持久化或去重 delivery。调用方使用 delivery ID 作为幂等键，并应将耗时工作放入队列。

仓库选择、fork 策略、本地 workspace、Git 操作、CI 动作、review 策略和合并时机由调用方决定。适配器只执行显式的远端 API 调用，不实现审批、重试、supervisor 或自动合并策略。本地 clone/worktree/commit/push 使用调用方的 `WorkspaceManager` 和 Git 工具。
