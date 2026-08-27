# multi-measure-binding Specification

## ADDED Requirements

### Requirement: 会话 MUST 支持绑定多台计量设备
会话服务 MUST 支持按设备 ID 集合绑定计量设备，驱动按 `deviceID → MeasureDriver` 维度存储；压力设备保持单一绑定。绑定令牌 MUST 携带计量设备 ID 集合，令牌校验 MUST 校验集合成员关系。

#### Scenario: 绑定多台计量设备
- **WHEN** 调用方提交 `measureDeviceIds=["dev-a","dev-b"]` 且压力设备为 `press-1`
- **THEN** 会话 MUST 为 dev-a、dev-b 分别解析并注册计量驱动，绑定令牌的 `measureDeviceIds` MUST 包含两者

#### Scenario: 绑定单台计量设备
- **WHEN** 调用方提交单个 `measureDeviceIds=["dev-a"]`
- **THEN** 会话 MUST 按现有单设备语义绑定，绑定令牌 `measureDeviceIds` 为 `["dev-a"]`

#### Scenario: 令牌校验拒绝集合外设备
- **WHEN** 调用方携带 `measureDeviceIds` 中存在未绑定设备 ID 的令牌
- **THEN** 会话 MUST 返回绑定过期 / 无效错误

### Requirement: API MUST 支持多计量设备选择
标定与计量的设备设置端点 MUST 接受 `measureDeviceIds`（多设备）参数；单设备 `measureDeviceId` 字段保留用于向后兼容。前端设备选择 MUST 支持多选，选 1 台时行为与现有流程一致。

#### Scenario: 多设备设置请求
- **WHEN** 前端提交 `{measureDeviceIds: ["dev-a","dev-b"], pressureDeviceId: "press-1"}`
- **THEN** 后端 MUST 绑定多台计量设备并返回包含多设备 ID 的会话状态

#### Scenario: 单设备设置请求保持兼容
- **WHEN** 前端仅提交 `{measureDeviceId: "dev-a", pressureDeviceId: "press-1"}`
- **THEN** 后端 MUST 绑定单台计量设备并保持现有响应结构

### Requirement: 绑定冲突 MUST 按设备集合判定
不同模块（标定 / 计量）对同一设备 ID 的绑定冲突判定 MUST 保持在设备集合维度，防止模块间覆盖对方已绑定的设备上下文。

#### Scenario: 跨模块绑定不同设备集合
- **WHEN** 标定模块绑定 `["dev-a"]` 后，计量模块尝试绑定 `["dev-b"]`
- **THEN** 会话 MUST 返回绑定冲突错误

#### Scenario: 同模块更新设备集合
- **WHEN** 同一模块再次提交与当前一致的设备集合
- **THEN** 会话 MUST 允许刷新绑定而不报冲突

### Requirement: 配置层 MUST 消费多计量设备 ID
应用配置的最近设备记录 MUST 支持 `measureDeviceIds`（切片）；绑定恢复流程 MUST 优先读取该切片，缺失时回退单设备 ID。

#### Scenario: 恢复多设备绑定
- **WHEN** 配置记录了 `measureDeviceIds: ["dev-a","dev-b"]`
- **THEN** 恢复流程 MUST 使用该设备集合重新绑定