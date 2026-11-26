# OpenAI OAuth 适配器

`adapters/openai.OAuthTokenSource` 为 Judge 的 Responses API 客户端等调用方提供 Sign in with ChatGPT 凭据。

## 授权与令牌

`SignIn` 使用 PKCE、随机 state/nonce 和绑定到 `127.0.0.1` 的本机回调生成授权 URL。调用方通过回调接收 URL，并决定如何打开浏览器。适配器校验回调 state 与 issuer，交换授权码，验证 OIDC ID token 与 nonce，并检查授权 scope。重新授权的账号 subject 必须与已保存账号一致。

`AccessToken` 在令牌剩余时间超过配置的刷新窗口时直接返回令牌；否则使用已保存的 refresh token，校验响应和必需 scope，并在返回新 access token 前保存轮换后的凭据。

## 凭据存储

默认文件位于操作系统用户配置目录的 `nesko/openai-oauth.json`；调用方可以指定其他路径。适配器使用文件锁协调多个进程，要求凭据文件是权限不宽于 `0600` 的普通文件，并通过临时文件替换凭据。

此 token source 与 Pi 的 `openai-codex` 登录相互独立。如果两者授权的是同一 ChatGPT 账号，请求会共用该账号的 plan 用量。
