# multi-device-collection Specification

## ADDED Requirements

### Requirement: 压力点数据 MUST 按设备维度存储
压力点的采集数据 MUST 支持按设备维度存储（设备 ID → 该设备的通道数组 + 设备级状态 + 采集时间 + 跳过原因）。单设备路径 MUST 同时回填原有单设备字段以保持兼容。

#### Scenario: 多设备采集后按设备存储
- **WHEN** 对压力点执行多设备采集且 dev-a、dev-b 均成功
- **THEN** 该点的设备维度数据 MUST 包含 dev-a 与 dev-b 各自的通道数组，状态均为 `completed`

#### Scenario: 单设备采集兼容旧字段
- **WHEN** 批次仅含一台计量设备且采集成功
- **THEN** 该点 MUST 同时写入设备维度数据与原有单设备字段

### Requirement: 采集器 MUST 并行采集所有参与设备
在压力点稳定后，采集器 MUST 并行触发所有参与计量设备的采集（每设备独立驱动与上下文），并等待全部返回。采集期间任一设备超时 / 失败 MUST 视为该设备异常，进入报警处理，不阻塞其他设备的采集结果收集。

#### Scenario: 两台设备并行采集
- **WHEN** 压力点稳定且参与设备为 dev-a、dev-b
- **THEN** 系统 MUST 并行向 dev-a、dev-b 发起采集并等待两者全部完成后进入下一压力点

#### Scenario: 单台设备采集回归
- **WHEN** 参与设备仅 dev-a
- **THEN** 系统 MUST 按现有单设备采集逻辑执行

### Requirement: 设备失败 MUST 触发整批暂停与报警
任一参与设备在压力点采集失败、超时或断开时，自动采集 MUST 暂停并发布带 `deviceId` 的报警事件，等待用户决策。用户决策 MUST 支持：重试（整点重采）、跳过该设备（从本批次剩余流程永久移除）、停止。

#### Scenario: 某设备采集失败触发报警
- **WHEN** dev-b 在压力点采集失败
- **THEN** 自动采集 MUST 暂停且报警事件 MUST 携带 `deviceId: "dev-b"`

#### Scenario: 用户选择重试
- **WHEN** 用户在报警后选择重试
- **THEN** 系统 MUST 重新执行当前压力点的全部参与设备采集

#### Scenario: 用户选择跳过设备
- **WHEN** 用户在报警后选择跳过 dev-b
- **THEN** dev-b MUST 从本批次剩余压力点移除，不再参与后续采集，其余设备继续

#### Scenario: 用户选择停止
- **WHEN** 用户在报警后选择停止
- **THEN** 自动采集 MUST 终止并停止压力控制

### Requirement: 被跳过设备 MUST 保留在批次结果中
被跳过的设备 MUST 保留在本批次结果中：其在已完成压力点的数据保留，剩余压力点标记为设备级 `skipped` 并携带跳过原因；跳过原因 MUST 为预设选项加可选备注。

#### Scenario: 跳过后保留已完成数据
- **WHEN** dev-b 在第 3 点被跳过且第 1、2 点已采集成功
- **THEN** 第 1、2 点的 dev-b 数据 MUST 保留，第 3 点及以后标记为 `skipped` 且含跳过原因

#### Scenario: 跳过原因记录
- **WHEN** 用户以预设选项「采集超时」并备注「线缆接触不良」跳过 dev-b
- **THEN** 批次结果中 dev-b 的跳过原因 MUST 为「采集超时 - 线缆接触不良」

### Requirement: 事件 MUST 携带设备 ID
自动采集相关事件（数据采集、点完成、点状态、报警）的 payload MUST 携带 `deviceId`；单设备场景 MUST 携带该设备 ID 以保持兼容。

#### Scenario: 数据采集事件带设备 ID
- **WHEN** dev-a 在某点采集完成
- **THEN** `data.collected` 事件 MUST 携带 `deviceId: "dev-a"` 与该设备数据