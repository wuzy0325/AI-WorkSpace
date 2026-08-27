# calibration-point-status-resume Specification

## Purpose
TBD - created by archiving change stabilize-measurement-calibration-core-flow. Update Purpose after archive.
## Requirements
### Requirement: 压力点状态 MUST 全生命周期可追踪
标定服务 MUST 为每个压力点维护显式状态，至少包含 `pending`、`pressurizing`、`stabilizing`、`collecting`、`completed`、`error`。每次状态变化 MUST 发布 `calibration.point_status` 事件，事件载荷 MUST 包含压力点索引、目标压力与当前状态。

#### Scenario: 压力点正常采集状态流转
- **WHEN** 系统开始采集某压力点并顺利完成
- **THEN** 该点状态 MUST 按 `pressurizing -> stabilizing -> collecting -> completed` 顺序更新并逐步推送事件

#### Scenario: 采集中出现错误
- **WHEN** 某压力点在打压、稳定或采集阶段发生错误
- **THEN** 该点状态 MUST 被置为 `error` 并推送对应状态事件

### Requirement: 暂停恢复 MUST 从中断位置继续
标定服务 MUST 在暂停时记录当前压力点索引与状态，并在恢复时从未完成的当前点继续，而不是回到首点重采。

#### Scenario: 暂停后恢复当前点
- **WHEN** 采集在第 N 个压力点暂停且该点状态非 `completed`
- **THEN** 恢复后 MUST 从第 N 个压力点继续执行后续阶段

#### Scenario: 暂停发生在点完成后
- **WHEN** 暂停时第 N 个点已为 `completed`
- **THEN** 恢复后 MUST 从第 N+1 个未完成压力点开始

