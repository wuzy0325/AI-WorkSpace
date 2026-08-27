# measurement-state-machine Specification

## Purpose
TBD - created by archiving change stabilize-measurement-calibration-core-flow. Update Purpose after archive.

## MODIFIED Requirements

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

### Requirement: 多次采样数据 MUST 平铺存储
手动或自动采集指定测点时，每次采样间隔 100ms。多次采样的原始数据 MUST 按通道平铺顺序拼接为单个数组存储，不得做任何均值计算。原始数据的排序规则为：第 1 次采样 16 通道 → 第 2 次采样 16 通道 → … → 第 N 次采样 16 通道。

#### Scenario: 3 次采样 2 通道数据存储
- **WHEN** 对压力点执行 3 次采样，结果为 [1.0, 2.0], [1.1, 2.1], [1.2, 2.2]
- **THEN** CollectedData MUST 为 [1.0, 2.0, 1.1, 2.1, 1.2, 2.2]

### Requirement: 采集服务稳定等待时长默认值 MUST 为 5000ms
当采集配置中未显式指定稳定等待时长时，服务 MUST 使用 5000ms（5 秒）作为默认值。

#### Scenario: 未设置稳定时长时使用默认值
- **WHEN** 采集配置的 StableWaitMs 为空或 0
- **THEN** 服务 MUST 使用 5000ms 作为默认稳定等待时长

### Requirement: 稳定性监控 MUST 支持 SCPI 设备判稳路径
稳定性监控 MUST 根据打压设备类型选择判稳方式：
1. 设备实现了 `StabilityStatusProvider` 接口（如 SCPI 设备）时，优先读取设备返回的稳定标志
2. 其他设备使用软件判稳（读取当前压力，偏差与阈值比较）

两条路径汇合后统一倒计时 stableDuration。

#### Scenario: SCPI 设备使用设备判稳
- **WHEN** 打压设备为 SCPI 设备且已实现 `StabilityStatusProvider` 接口
- **THEN** 稳定性监控 MUST 查询设备返回的 `isStable` 标志（而非自行计算偏差）

#### Scenario: 非 SCPI 设备使用软件判稳
- **WHEN** 打压设备未实现 `StabilityStatusProvider` 接口
- **THEN** 稳定性监控 MUST 读取当前压力并计算偏差，与稳定性阈值比较
