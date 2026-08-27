# calibration-alarm-decision-flow Specification

## Purpose
TBD - created by archiving change stabilize-measurement-calibration-core-flow. Update Purpose after archive.
## Requirements
### Requirement: 报警决策 MUST 进行合法性校验
报警服务 MUST 仅接受 `continue`、`skip`、`recollect`、`stop` 四类决策。其他输入 MUST 返回校验错误并不得进入采集执行流程。

#### Scenario: 提交合法决策
- **WHEN** 操作员提交 `continue`、`skip`、`recollect` 或 `stop`
- **THEN** 系统 MUST 接受该决策并将其传递给采集流程

#### Scenario: 提交非法决策
- **WHEN** 操作员提交未定义决策值
- **THEN** 系统 MUST 返回非法决策错误且不改变当前采集状态

### Requirement: 自动采集循环 MUST 按决策执行分支动作
在存在待处理报警时，采集循环 MUST 阻塞等待决策，并按以下语义执行：`continue` 继续后续流程；`skip` 标记当前点为 `skipped` 并前进；`recollect` 重新采集当前点；`stop` 终止自动采集。

#### Scenario: skip 决策
- **WHEN** 当前点出现报警且决策为 `skip`
- **THEN** 当前点 MUST 被标记为 `skipped` 且流程 MUST 前进到下一点

#### Scenario: recollect 决策
- **WHEN** 当前点出现报警且决策为 `recollect`
- **THEN** 系统 MUST 保持在当前点并重新执行采集

#### Scenario: stop 决策
- **WHEN** 当前点出现报警且决策为 `stop`
- **THEN** 自动采集 MUST 终止并发布 `calibration.alarm.resolved` 事件

### Requirement: 报警阈值 MUST 使用量程引用误差
多通道报警判定 MUST 使用量程引用误差公式计算允许偏差：`(maxPressure - minPressure) × precisionThreshold`。当量程为 0 时，降级使用 `|target| × precisionThreshold` 作为容差。

#### Scenario: 窄量程正常触发报警
- **WHEN** 校准量程为 1 MPa（min=9.5, max=10.5）且 precisionThreshold=0.001（0.1%）
- **THEN** 允许偏差 MUST 为 0.001 MPa 而非 0.0105 MPa

#### Scenario: 量程为零时使用目标值作为基准
- **WHEN** maxPressure 等于 minPressure（量程为 0）
- **THEN** 允许偏差 MUST 降级为 `|targetPressure| × precisionThreshold`
