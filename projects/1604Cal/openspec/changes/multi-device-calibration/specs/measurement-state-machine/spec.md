# measurement-state-machine Specification (delta)

## ADDED Requirements

### Requirement: 多设备计量场景 MUST 支持并行的暂停与恢复语义
当批次含多台计量设备时，采集服务的暂停 / 恢复 / 完成状态迁移 MUST 与单设备保持一致的合法迁移约束；暂停后恢复 MUST 从当前压力点重新执行全部未跳过设备的采集。

#### Scenario: 多设备暂停后恢复
- **WHEN** 多设备批次在压力点 N 暂停且用户选择恢复
- **THEN** 状态机 MUST 迁移回 `pressuring` 并从压力点 N 重新执行全部未跳过设备的采集

#### Scenario: 多设备完成后状态迁移
- **WHEN** 所有参与设备的全部压力点采集完成
- **THEN** 状态机 MUST 迁移到 `completed` 并发布状态变化事件

### Requirement: 多设备采集过程状态 MUST 反映设备维度进展
多设备批次中，单个压力点的状态（`pressurizing` / `stabilizing` / `collecting` / `completed`）MUST 仍以点为单位推进；设备维度状态（每设备 `completed` / `error` / `skipped`）MUST 通过设备维度数据与事件呈现，不得影响点的状态机迁移合法性。

#### Scenario: 部分设备跳过仍完成该点
- **WHEN** 压力点采集时 dev-b 被用户跳过、dev-a 成功
- **THEN** 该点状态 MUST 迁移为 `completed`，dev-b 在该点标记设备级 `skipped`

#### Scenario: 设备级异常进入报警不迁移完成
- **WHEN** 压力点采集时某设备失败触发报警
- **THEN** 该点 MUST 进入 `await_alarm_resolution` 等待用户决策，不得迁移到 `completed`