# per-device-report Specification

## ADDED Requirements

### Requirement: 报告 MUST 按计量设备分别生成
报告服务 MUST 对批次内的每台参与计量设备分别生成独立报告，报告内容取自该设备的设备维度采集数据。单台设备场景 MUST 保持现有报告生成行为。

#### Scenario: 多设备批次生成多份报告
- **WHEN** 批次含 dev-a、dev-b 且两者均有采集数据
- **THEN** 系统 MUST 分别生成 dev-a 与 dev-b 的报告，各自只包含对应设备的数据

#### Scenario: 单设备批次保持兼容
- **WHEN** 批次仅含 dev-a
- **THEN** 系统 MUST 按现有单设备报告逻辑生成

### Requirement: 报告数据聚合 MUST 支持设备维度
报告通道数据聚合函数 MUST 从设备维度数据（设备 ID → 通道数组）聚合；单设备场景回退到原有单设备字段。被跳过设备的已完成压力点数据 MUST 参与聚合，未完成压力点 MUST 被排除。

#### Scenario: 按设备聚合通道数据
- **WHEN** dev-a 在第 1、2 点成功、第 3 点被跳过
- **THEN** dev-a 报告的正程通道数据 MUST 包含第 1、2 点，排除第 3 点

#### Scenario: 单设备回退到旧字段
- **WHEN** 数据来自单设备旧格式（仅 `CollectedData`）
- **THEN** 报告 MUST 使用该字段聚合

### Requirement: 报告身份信息 MUST 使用设备集合
报告中的计量设备身份 MUST 使用会话的设备 ID 集合；「设备编号」等元数据 MUST 从各参与设备的配置分别派生，而非共享单一编号。

#### Scenario: 报告携带各自设备编号
- **WHEN** dev-a、dev-b 参与同一批次
- **THEN** 各自的报告 MUST 使用 dev-a、dev-b 各自的设备编号元数据