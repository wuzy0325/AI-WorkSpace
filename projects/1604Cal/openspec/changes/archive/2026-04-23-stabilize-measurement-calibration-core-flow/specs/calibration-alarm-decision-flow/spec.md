## ADDED Requirements

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
