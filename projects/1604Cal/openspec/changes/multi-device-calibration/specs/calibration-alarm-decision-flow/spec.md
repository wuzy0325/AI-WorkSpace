# calibration-alarm-decision-flow Specification (delta)

## ADDED Requirements

### Requirement: 报警判定 MUST 按设备维度评估
多设备批次中，报警服务 MUST 对每台参与计量设备的采集数据独立评估超限通道；任一设备触发报警时，报警事件 MUST 携带该设备 ID 及其超限通道。单台设备场景 MUST 保持现有报警判定行为。

#### Scenario: 单台设备超限触发报警
- **WHEN** 批次含 dev-a、dev-b，仅 dev-b 某通道超限
- **THEN** 系统 MUST 触发报警且事件携带 `deviceId: "dev-b"` 与 dev-b 的超限通道

#### Scenario: 单台设备场景保持原行为
- **WHEN** 批次仅含一台计量设备且其某通道超限
- **THEN** 系统 MUST 按现有单设备报警判定触发报警

### Requirement: 报警决策 MUST 支持设备级跳过
报警决策 MUST 在 `continue` / `skip` / `recollect` / `stop` 基础上，支持「跳过指定设备」：用户跳过某台设备后，该设备从本批次剩余压力点移除，其余设备继续；该设备已完成压力点数据保留并标记设备级 `skipped`。

#### Scenario: 设备级跳过决策
- **WHEN** 报警由 dev-b 触发且用户选择跳过 dev-b
- **THEN** dev-b MUST 从剩余压力点移除并标记 `skipped`（含跳过原因），dev-a MUST 继续执行后续压力点

#### Scenario: 重试仍作用于整点
- **WHEN** 报警由 dev-b 触发且用户选择重试
- **THEN** 系统 MUST 重新采集当前压力点的全部参与设备

#### Scenario: 停止终止整批
- **WHEN** 报警由 dev-b 触发且用户选择停止
- **THEN** 自动采集 MUST 终止并停止压力控制