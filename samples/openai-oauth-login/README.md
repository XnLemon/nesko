# OpenAI OAuth Login Sample

This sample signs in to ChatGPT for the Judge's Responses API adapter. Run it directly:

```sh
go run ./samples/openai-oauth-login
```

The sample opens the system browser and listens on a loopback callback. It saves credentials with owner-only permissions under the OS user config directory at `nesko/openai-oauth.json`. Set `NESKO_OPENAI_OAUTH_CREDENTIALS_PATH` to override that location. Use the same ChatGPT account when signing in to Pi with `/login openai-codex`; the two clients keep separate OAuth credentials while sharing the account's plan usage.
