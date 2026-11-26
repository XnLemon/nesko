# Nesko Core

Nesko is a reusable Go core for building an IM-driven coding agent. It provides a normalized request and ticket domain, the request-intake application service, ports for persistence and messaging, and Feishu and SQLite adapters.

The core receives Feishu text messages, applies sender and group-mention policy, records accepted requests as tickets, and acknowledges them in the source conversation. Applications compose the exported packages according to their own runtime and interaction model.

Architecture documentation is maintained in [`docs/zh/`](docs/zh/) and [`docs/en/`](docs/en/).
