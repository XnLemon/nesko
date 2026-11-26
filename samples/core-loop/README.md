# Core Loop Sample

This sample can either feed a normalized request directly into the core or start a Feishu long-connection gateway. Both modes use the model Judge, default decision policy, Pi executor, and SQLite lifecycle repositories. In Feishu mode, the sample acknowledges and assesses each accepted request, then executes queued tickets. Approval and human follow-up remain caller-owned workflow steps.

Run it from the repository root with `go run ./samples/core-loop`. Set `FEISHU_APP_ID` to start Feishu gateway mode; otherwise it runs once using `NESKO_REQUEST_TEXT`.

| Variable | Required | Description |
| --- | --- | --- |
| `NESKO_REQUEST_TEXT` | Direct mode | Ticket request text. |
| `FEISHU_APP_ID` | Feishu mode | Self-built Feishu app ID. |
| `FEISHU_APP_SECRET` | Feishu mode | Self-built Feishu app secret. |
| `FEISHU_BOT_OPEN_ID` | Feishu mode | Bot Open ID used to accept group messages that mention the bot. |
| `FEISHU_ALLOWED_OPEN_IDS` | No | Comma-separated sender Open IDs allowed to create tickets. |
| `NESKO_JUDGE_MODEL` | Yes | Model name for the Judge. |
| `NESKO_JUDGE_AUTH` | No | `api-key` (default) or `chatgpt-oauth`. |
| `NESKO_JUDGE_ENDPOINT` | For `api-key` | Full OpenAI-compatible Chat Completions endpoint URL. |
| `NESKO_OPENAI_OAUTH_CREDENTIALS_PATH` | No | OAuth credential file path; defaults to the OS user config directory under `nesko/openai-oauth.json`. |
| `PI_WORKING_DIRECTORY` | Yes | Directory used as Pi's process working directory. |
| `NESKO_JUDGE_API_KEY` | No | Bearer token for the Judge endpoint in `api-key` mode. |
| `NESKO_PROJECT_CONTEXT_FILE` | No | Read-only context file for Judge assessment, limited to 96 KiB. Without it, repository-dependent dimensions are assessed as unknown. |
| `NESKO_DATABASE_PATH` | No | SQLite path; defaults to `./data/nesko-sample.sqlite`. |
| `PI_BINARY_PATH` | No | Pi executable path; defaults to `pi`. |
| `PI_PROVIDER` | No | Pi provider. In `chatgpt-oauth` mode it defaults to and must be `openai-codex`. |
| `PI_MODEL` | No | Pi model name; in `chatgpt-oauth` mode it defaults to `NESKO_JUDGE_MODEL`. |
| `PI_TOOLS` | No | Comma-separated Pi tools; defaults to `read,grep,find,ls,bash,edit,write`. |

For `chatgpt-oauth` mode, first run `go run ./samples/openai-oauth-login`. It opens the official ChatGPT sign-in page and stores credentials in the OS user config directory; set `NESKO_OPENAI_OAUTH_CREDENTIALS_PATH` only if you want a different location. Separately sign in to Pi with `/login openai-codex`, using the same ChatGPT account, then set `NESKO_JUDGE_AUTH=chatgpt-oauth` for the core-loop sample. The Judge and Pi have separate OAuth credentials; requests from both apps consume the same account's ChatGPT plan usage.

In `api-key` mode, Pi authentication is inherited from the sample process environment. The configured working directory and prompt are not an operating-system sandbox. Run the sample only with a workspace and credentials appropriate for the configured Pi tools.

Feishu mode runs triage and any automatic execution in the event handler to keep the sample composition small. A personal supervisor should queue long-running work and own scheduling, heartbeat timeouts, retries, and human follow-up.
