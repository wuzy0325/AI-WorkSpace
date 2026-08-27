# calibration-stability-sse Specification

## Purpose
TBD - created by archiving change stabilize-measurement-calibration-core-flow. Update Purpose after archive.
## Requirements
### Requirement: 稳定性监控 MUST 持续发布进度事件
稳定性监控在等待稳定阶段 MUST 周期性发布 `calibration.stability.progress` 事件，推荐周期为 200ms。事件载荷 MUST 包含 `progress` 字段，取值范围 MUST 为 0 到 100。

#### Scenario: 稳定等待中持续推送
- **WHEN** 系统进入稳定等待阶段且尚未达稳
- **THEN** 系统 MUST 按周期发布进度事件且 `progress` 值单调非降直到 100

#### Scenario: 进度字段范围校验
- **WHEN** 进度事件被发布
- **THEN** 事件中的 `progress` MUST 位于闭区间 [0, 100]

### Requirement: 稳定结果 MUST 通过事件明确通知
稳定性监控 MUST 在达到稳定时发布 `calibration.stability.achieved` 事件；若稳定性在监控过程中丢失，系统 MUST 发布 `calibration.stability.lost` 事件并继续监控直到超时或重新达稳。

#### Scenario: 达稳事件
- **WHEN** 监控判定已满足稳定时间与容差条件
- **THEN** 系统 MUST 发布一次 `calibration.stability.achieved` 事件

#### Scenario: 稳定丢失事件
- **WHEN** 监控过程中从稳定状态回落到不稳定
- **THEN** 系统 MUST 发布 `calibration.stability.lost` 事件

