## ADDED Requirements

### Requirement: 计量采集服务 MUST 使用完整状态机控制流程
计量采集服务 MUST 支持以下状态并以状态机方式管理：`idle`、`pressuring`、`stabilizing`、`collecting`、`completed`、`error`、`paused`。服务 MUST 仅允许预定义合法迁移，非法迁移 MUST 返回错误且不得改变当前状态。

#### Scenario: 服务初始化状态
- **WHEN** 服务实例被创建且未开始采集
- **THEN** 当前状态 MUST 为 `idle`

#### Scenario: 合法状态迁移
- **WHEN** 当前状态为 `pressuring` 且请求切换到 `stabilizing`
- **THEN** 服务 MUST 完成状态切换并发布 `measurement.state_changed` 事件

#### Scenario: 非法状态迁移被拒绝
- **WHEN** 当前状态为 `idle` 且请求直接切换到 `collecting`
- **THEN** 服务 MUST 返回迁移错误且状态保持 `idle`

#### Scenario: 暂停后恢复迁移
- **WHEN** 当前状态为 `paused` 且请求切换到 `collecting` 或 `pressuring`
- **THEN** 服务 MUST 按迁移表允许恢复并发布状态变化事件
