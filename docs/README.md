# Documentation / 文档

This README defines the scope and format of this directory. Except for this index and authoring guide, `docs/zh/` and `docs/en/` contain architecture documentation only.

本 README 说明文档目录的范围和格式。除本索引与编写规范外，`docs/zh/` 和 `docs/en/` 仅存放架构文档。

## Scope / 文档范围

Architecture pages describe how the project is implemented today. Do not add plans, proposals, roadmaps, task lists, progress reports, release notes, general user guides, or descriptions of superseded designs.

架构文档只描述项目当前已经实现的结构和行为。不在此目录存放计划、提案、路线图、任务清单、进度记录、发行说明、通用使用指南或已废弃设计的描述。

## Languages and paths / 语言与路径

- Chinese pages live in `docs/zh/`; English pages live in `docs/en/`.
- Every architecture page has a counterpart with the same relative path and filename in the other language directory.
- Keep both versions semantically equivalent and update them in the same change.
- Use Markdown files with lowercase kebab-case names, for example `system-overview.md`.

- 中文文档放在 `docs/zh/`，英文文档放在 `docs/en/`。
- 每篇架构文档都必须在另一语言目录中有相同相对路径和文件名的对应文档。
- 两种语言的内容含义保持一致，并在同一次变更中更新。
- 使用 Markdown，文件名采用小写 kebab-case，例如 `system-overview.md`。

Start from the language index: [`docs/en/index.md`](en/index.md) or [`docs/zh/index.md`](zh/index.md). The pages are split into core domain, ports, service internals, and one page per adapter.

从语言索引开始阅读：[`docs/en/index.md`](en/index.md) 或 [`docs/zh/index.md`](zh/index.md)。文档按 core 领域模型、ports、service 内部流程和各 adapter 分页。

## Current implementation only / 只描述当前实现

- Document behavior and structure that are verifiable in the current source code and configuration.
- Do not describe planned or unimplemented components, work in progress, intermediate migration states, or future behavior.
- Do not use draft, status, TODO, or next-step sections in architecture pages.
- Keep diagrams aligned with the current implementation. Remove or revise obsolete descriptions when the implementation changes.
- Update the relevant Chinese and English pages in the same change as an architecture-affecting implementation change.

- 只记录能从当前源代码和配置中核实的结构与行为。
- 不描述计划中或尚未实现的组件、进行中的工作、中间迁移状态或未来行为。
- 架构文档不设置草稿、状态、TODO 或下一步计划章节。
- 架构图必须与当前实现一致；实现变化后，同步修改或删除过时描述。
- 影响架构的实现变更应在同一次变更中更新对应的中英文文档。

## Page structure / 页面结构

Use only the sections relevant to the documented system. A page may cover:

- Purpose and boundaries
- Components and responsibilities
- Runtime, data, and control flows
- Interfaces and dependencies
- State and data ownership
- Failure handling and trust boundaries

页面只保留与所述系统相关的章节，可按需描述：

- 目标与边界
- 组件及职责
- 运行、数据与控制流
- 接口与依赖
- 状态和数据归属
- 故障处理与信任边界

Link claims to the relevant implementation files or configuration where practical. Keep explanations concise and avoid repeating source code.

条件允许时，为文档中的实现说明链接到相关源文件或配置。说明保持简洁，避免重复粘贴源代码。
