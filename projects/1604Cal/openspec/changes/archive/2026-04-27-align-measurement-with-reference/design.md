## Context

计量工作台当前已实现基本 UI 框架（MeasurementParamsPanel、MeasurementControl、MeasurementDataView）和压力表生成 API，但与参考模块（1605MeassureApp CalibrationView）在以下方面存在差距：

- 无设备管理对话框（添加/编辑/删除设备）
- 无采集控制流程（自动/手动打压、采集、暂停、恢复、停止、重置的逻辑未对接）
- 无导出报告对话框
- 无报警通道选择对话框
- 报警配置变更未自动持久化
- 无单位一致性检查
- 无采集事件监控（SSE 已存在但未对接）
- 数据表目标值编辑未同步 store

## Goals / Non-Goals

**Goals:**
- 补齐上述14项差距，使计量工作台与参考模块业务逻辑和UI完全对齐
- 复用现有组件和API，减少重复开发
- 保持现有后端API不变，仅前端改动

**Non-Goals:**
- 不改动后端 Go 代码（API 已存在）
- 不改动标定模块（Calibration）
- 不引入新的第三方依赖

## Decisions

### D1: 复用现有 DeviceFormDialog 组件
参考模块已有的 `DeviceFormDialog` 组件在项目 `web/src/components/common/` 中已存在。直接在 MeasurementSidebar 中引用，通过 slot 或 props 传递设备列表操作。

### D2: 采集控制流程通过 emit 链在 MeasurementView 集中处理
参考模块的采集控制集中在 CalibrationView 中。我们的架构将 MeasurementControl 作为纯 UI 组件，将其 `start`/`pause`/`resume`/`stop`/`reset`/`select-channel` emit 在 MeasurementView 中处理，避免将业务逻辑分散到子组件。

### D3: 导出报告对话框作为独立组件
新建 `ExportReportDialog.vue`，复用现有 `api/calibration.ts` 中的报告模板选择和 `api/session.ts` 中的读取接口。对话框包含路径选择、报告模板显示、点数和模式信息。

### D4: 报警通道选择对话框作为独立组件
新建 `AlarmChannelSelectDialog.vue`，16通道网格布局，全选/全不选功能。报警通道数据通过 store.alarmConfig.enabledChannels 同步。

### D5: 报警配置自动保存通过 watch + debounce 在 MeasurementView 中实现
参考模块使用 `watch(alarmConfig, ...) → queueAutoSaveAlarmConfig()` 模式。我们在 MeasurementView 中 watch `store.alarmConfig` 的 enabled/soundEnabled/confirmOnAlarm/enabledChannels，防抖 250ms 后调用 `saveAlarmConfig` API。

### D6: 单位一致性检查通过 deviceStore.loadDevices 获取
deviceStore 已有 `checkUnitConsistency` action（由 `fetchUnitConsistency()` API 支持）。生成压力表前和添加/删除设备后调用。

### D7: 进度计算改为 currentPointIndex
参考模块的进度基于 `currentPointIndex / pressurePoints.length`。在 store 中添加 `currentPointIndex` ref，在采集过程中更新。

### D8: 数据表目标值编辑同步到 store
`onTargetChange` 调用 `measurementStore.updatePointTarget(pointId, value)`。在 store 中添加 `updatePointTarget` action。

## Risks / Trade-offs

- [复用组件适配] DeviceFormDialog 可能依赖 deviceStore 的特定 API → 需要确认接口兼容性，否则做适配
- [事件未处理] MeasurementControl 的 emit 事件在 MeasurementView 中需要完整的 handler 链 → 一次性实现避免遗漏
- [SSE 对接] useCollectionEvents composable 可能不存在或需要调整 → 从 reference 项目移植或重新实现
