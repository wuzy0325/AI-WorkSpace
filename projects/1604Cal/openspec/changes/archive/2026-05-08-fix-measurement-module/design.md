## Context

计量模块（measurement service + alarm service + frontend）在 code review 中发现 6 项 Critical/High 级差异，涉及报警阈值算法、采样存储语义、UI 标签/约束、稳定性默认值等多个层面。

**当前状态**：
- 报警阈值：`EvaluateMultiChannel` 使用 `|maxPressure| × precisionThreshold / 100`，应改为 `(maxPressure - minPressure) × precisionThreshold`
- 多次采样：`ManualCollect` 调用 `AverageSamples` 取平均，应改为平铺存储原始数据
- 稳定时长默认值：代码为 2000ms，文档为 5000ms
- 稳定性监控：无条件走软件判稳，未利用 SCPI 设备判稳能力
- UI 标签/约束与文档不一致

**约束**：
- 不引入新依赖
- 向后兼容 API 格式（部分字段值变更不影响接口契约）
- 所有修改必须有配套单元测试

## Goals / Non-Goals

**Goals:**
- 所有 Critical/High 问题的代码与文档对齐
- 报警阈值正确性修复：不遗漏窄量程超限
- 采样存储语义一致：避免原始数据丢失
- UI 术语和约束与业务术语表对齐

**Non-Goals:**
- 不重构连续采集模式（`startCollectLoop`）与按点采集模式的关系——留待后续架构清理
- 不改动 CSV 导出格式（兼容即可）
- 不新增 UI 交互能力

## Decisions

### Decision 1: 报警阈值采用量程引用误差
- **选择**：`(maxPressure - minPressure) × precisionThreshold`
- **理由**：这是计量行业通行做法，窄量程校准下满量程基准会大幅低估偏差
- **影响**：`EvaluateMultiChannel` 需要增加 `minPressure` 参数，调用方 `checkAlarm` 传入 `config.MinPressure`
- **替代方案**：使用 `|target| × precisionThreshold` 对低量程同样有效，但量程引用误差更符合标准

### Decision 2: 多次采样平铺存储
- **选择**：移除 `AverageSamples` 调用，改为 `samples` 拼接为一个长数组（平铺）
- **理由**：保持原始采样数据完整性，文档明确要求不做平均
- **影响**：`ManualCollect` 的 `samples` 不再求平均；报告层如需平均值可自行计算
- **注意**：此改动会改变 `CollectedData` 数组长度（从通道数变为 通道数 × 重复采样数），前端通道数据展示和报表必须兼容

### Decision 3: 稳定性监控增加 SCPI 设备分支
- **选择**：在 `waitForMeasurementStability` 中检查 `PressureDriver` 是否实现 `StabilityCapable` 接口，是则走设备判稳
- **理由**：SCPI 设备通过 `OUTPut:STABility?` 命令直接返回稳定标志，比软件计算更准确及时
- **影响**：仅影响 SCPI 设备（ConST 系列），不影响 WTN1604/模拟设备

### Decision 4: 精度等级 UI 输入限制为离散值
- **选择**：自定义模式也受限制，仅允许从 `[0.0001, 0.0002, 0.0005, 0.001, 0.002]` 中选择
- **理由**：文档 CC-005 明确为离散可选值，自由输入会破坏标准化
- **影响**：删除自定义复选框，改为固定下拉选择

## Risks / Trade-offs

- **[Risk] 平铺存储改变 `CollectedData` 语义** → `PressurePoint.CollectedData` 目前是 `float64[]`，平铺后长度翻倍。前端展示和报告生成需确认兼容性。**Mitigation**: 报告层在读取时按 repeatCount 切片后取平均（保持报告端行为不变）
- **[Risk] SCPI 判稳分支增加设备耦合** → 需要 `PressureDriver` 接口增加 `HasStabilityStatus()`，或通过 type assertion 判断。**Mitigation**: 使用 Go 接口断言 `if ssd, ok := driver.(StabilityStatusProvider); ok { ... }`，不破坏现有接口
