## Why

计量模块（measurement）代码与《计量业务逻辑说明.md》v5.0 之间存在多处关键差异：
- 报警阈值计算使用满量程基准而非量程引用误差，导致窄量程校准报警失效
- 多次采样错误地做了平均值而非平铺存储，丢失原始采样离散信息
- UI 标签/约束与文档业务术语不一致，增加使用和理解成本
- 稳定时长默认值、设备判稳路径等技术细节偏离文档规范

这些问题均出现在 2026-05-08 code review（M-001 至 M-008）中，属于数据正确性和使用一致性问题，需要在本变更中修复。

## What Changes

### 数据正确性（CRITICAL）
- **报警阈值计算修正**：`|maxPressure| × precisionThreshold` → `(maxPressure - minPressure) × precisionThreshold`，影响标定模块的 `EvaluateMultiChannel` 和报警总控服务
- **多次采样存储策略修正**：`AverageSamples` 取平均 → 平铺存储原始采样数据，计量模块 `ManualCollect` 及相关链条

### 行为一致性（HIGH）
- **稳定时长默认值**：2000ms → 5000ms，校准配置层
- **稳定性监控**：统一走软件判稳，增加 SCPI 设备判稳路径分支
- **UI 标签和约束**：`平均:` → `重复采样:`、`精度:` → `显示精度:`、`精度 Level` → `精度等级`，averageCount 上限 20 → 10
- **精度等级输入**：自定义模式限制为离散可选值

### 架构清理（MEDIUM）
- 统一计量/标定报警模型为一套语义（使用量程引用误差）
- 明确连续采集和按点采集两种模式各自的行为和边界

## Capabilities

### New Capabilities
（无新增 capability - 本次变更修改现有行为，不引入新能力）

### Modified Capabilities
- `measurement-state-machine`: 多次采样存储语义变更（平铺 vs 平均）
- `calibration-alarm-decision-flow`: 报警阈值计算公式变更

## Impact

- **核心算法**：`internal/workflow/alarm_service.go:72` 阈值计算逻辑修改
- **存储语义**：`internal/domain/pressure_point.go:18` `AverageSamples` 调用链移除或改为平铺
- **稳定性**：`internal/application/measurement/collector.go:277-325` 增加设备类型分支
- **配置逻辑**：`collector.go:161` 稳定时长默认值调整
- **UI 组件**：`web/src/components/measurement/MeasurementParamsPanel.vue` 标签/约束修正
- **单元测试**：`service_test.go`、`alarm_service` 测试配套更新
