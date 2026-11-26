# SQLite 适配器

`adapters/sqlite.TicketRepository` 使用 `database/sql` 和纯 Go `modernc.org/sqlite` 驱动，实现工单、执行记录、生命周期和恢复仓储端口。

## 工单存储

仓储打开调用方指定的数据库路径，启用 WAL，并将连接池最大连接数设为 1。`tickets` 表保存请求身份、状态、确认消息 ID、结构化 Judge 评估与策略决策、活跃执行 ID、心跳/进度、结果摘要和失败原因。

`(channel, source_message_id)` 组成唯一键。`CreateOrGet` 会插入新的 `received` 工单，或在重复投递时读取现有工单。工单状态更新条件包含预期当前状态，并拒绝不允许的迁移。追加人工输入时，会同时将 `waiting_human` 工单迁移回 `triaging`。

初始化会创建缺失的表和索引，并为已有工单表与执行表补齐缺失列。它保留已有工单身份，不重建表。

## 执行历史与原子生命周期

`execution_runs` 为每次执行保存一份指令和决策快照。`execution_run_events` 以递增 sequence 保存事件，并保证同一 run 内 event ID 唯一。仓储提供活跃执行查询和有序事件历史。

通过 `ExecutionLifecycleRepository` 使用时，启动和结束会在同一个 SQLite 事务中同时更新工单与执行记录。心跳只会写入 execution ID 匹配且仍在 running 的工单。`ExecutionRecoveryRepository` 在事务内重新检查最近心跳，再将过期 run 标记为 `interrupted`，并将对应运行中工单标记为 `failed`。
