# GitHub 适配器

## 组件与边界

`adapters/github.Client` 通过 `go-github` 封装 GitHub REST API。调用方可以配置静态 token 或 OAuth token source、可选 HTTP client，以及 GitHub Enterprise 使用的可选 API/upload URL。请求携带 context。列表方法提供分页选项，默认每页 30 条，最多 100 条。

客户端提供：

- 仓库查询、当前认证用户查询和创建 fork。
- Issue 查询、列表、更新和 issue 评论。
- PR 查询、列表、创建和更新；变更文件与 commit 查询；已提交和待提交 review 生命周期、行内 review 评论和回复；请求或移除 reviewer；更新分支；以及使用预期 head SHA 保护的显式合并请求。
- Check run 列表和 commit 综合状态查询。
- GitHub Actions run 与 job 查询、日志 URL 获取、workflow dispatch、重跑、失败 job 重跑、单个 job 重跑和取消 run。

适配器返回 `go-github` 的类型化响应对象，不会将 GitHub 的完整响应模型转换为 Nesko 领域记录。API 错误包含操作类型和仓库引用。

`WebhookReceiver` 实现 `http.Handler`。它接受 POST 请求，要求 GitHub SHA-256 签名，使用配置的 secret 校验请求体，将已识别的事件解析为 `go-github` 类型化 payload，并将事件类型、delivery ID 与 payload 传给调用方 handler。请求体默认上限为 25 MiB。可选事件白名单会让未选中的事件返回 204。handler 成功时返回 202，handler 出错时返回 500。receiver 不保存或去重 delivery；调用方使用 delivery ID 作为幂等键。调用方 handler 可以快速将工作放入队列，再由 supervisor 处理事件。

## 所有权与流程

调用方将请求映射到仓库，决定是否创建 fork，准备本地 workspace，并运行分支创建、commit 和 push 等 Git 操作。客户端提供 issue、PR、review、CI 状态和合并所需的远端 GitHub API 操作。工单工作流不会自动调用这些方法。

调用方决定是否 dispatch 或重跑 CI、取消 run、请求 reviewer、提交 review、更新 PR 分支或合并 PR。适配器只执行显式操作，不会根据 CI 状态推断审批，也不应用自动合并或重试策略。认证凭据必须具备各 API endpoint 所需的 GitHub 权限。
