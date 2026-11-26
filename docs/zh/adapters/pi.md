# Pi 适配器

`adapters/pi.Executor` 通过每次工单执行启动 Pi CLI RPC 进程，实现 `ports.Executor`。

## 进程与 Workspace

执行器以 `pi --mode rpc --no-session` 启动 Pi，传入工具列表，并在配置启用前关闭 MCP 和扩展。默认工具是 `read`、`grep`、`find`、`ls`、`bash`、`edit` 和 `write`。Provider、model、环境变量覆盖、心跳间隔、关闭超时和 RPC 记录最大尺寸均可配置。

每次执行的 `ExecutionInput.Workspace.Directory` 优先于配置的备用目录。该目录会成为进程工作目录和提示词边界，但不是操作系统级沙箱。执行器提示词要求 Pi 只在该目录内操作文件，并禁止 commit、push、merge、部署和无关网络调用。

## RPC 与结果映射

执行器向 Pi 发送工单和已校验的决策，等待 prompt response 与 `agent_settled`，再读取最后一条 assistant 文本。工具事件转换为进度事件；等待期间默认每 20 秒发送心跳。RPC 记录和捕获的 stderr 都有尺寸限制。

最终响应必须包含有效的 `NeskoResult` JSON 行，outcome 为 `completed`、`needs_review` 或 `waiting_human`，且摘要非空。缺失或格式错误时映射为 `needs_review`。返回结果前会关闭并等待 Pi 进程退出。
