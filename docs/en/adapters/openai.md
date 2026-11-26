# OpenAI OAuth Adapter

`adapters/openai.OAuthTokenSource` implements Sign in with ChatGPT credentials for callers such as the Judge's Responses API client.

## Authorization and tokens

`SignIn` builds an authorization URL using PKCE, random state and nonce values, and a loopback callback bound to `127.0.0.1`. The caller receives the URL through a callback and decides how to open it. The adapter validates callback state and issuer, exchanges the code, verifies the OIDC ID token and nonce, and checks the granted scopes. A saved account subject must match the newly authorized account.

`AccessToken` returns a token while it is outside the configured refresh window. Otherwise it uses the saved refresh token, validates the response and required scopes, and stores the rotated credentials before returning the new access token.

## Credential storage

The default file is under the operating system user config directory at `nesko/openai-oauth.json`; callers can supply a different path. The adapter uses a file lock for cross-process coordination, requires a regular credential file with permissions no broader than `0600`, and replaces credentials through a temporary file.

This token source is separate from Pi's `openai-codex` login. If both authorize the same ChatGPT account, requests use that account's shared plan allowance.
