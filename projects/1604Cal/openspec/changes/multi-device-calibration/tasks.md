# 多设备计量（multi-device-calibration）任务清单

> 状态：实现完成，待归档。勾选 = 已完成；未勾选 = 未完成/部分完成（见偏差记录）。

## 1. 领域数据模型

- [x] 1.1 在 `internal/domain/pressure_point.go` 新增 `DevicePointData` 结构体（DeviceID / Collected / Status / CollectTime / SkipReason）与 `PressurePoint.CollectedByDevice map[string]DevicePointData` 字段（保留 `CollectedData` 兼容）
- [x] 1.2 在 `internal/domain/workflow_session.go` 的 `WorkflowSession` 新增 `MeasureDeviceIDs []string` 字段（保留 `MeasureDeviceID` 兼容）
- [x] 1.3 为 `CollectedByDevice` 增加读取辅助：优先多设备结构、缺失回退单设备 `CollectedData`（供报告 / CSV 统一消费）

## 2. 会话绑定集合化（backend session）

- [x] 2.1 `internal/application/session/service.go`：`measureDriver`/`measureDevID` 单槽改为 `measureDrivers map[string]device.MeasureDriver` + `measureDevIDs []string`
- [x] 2.2 `BindingToken.MeasureDeviceID` 新增 `MeasureDeviceIDs []string`；`validateToken` 改为集合成员校验
- [x] 2.3 `BindDevices` / `BindMeasureDevice` 接受 `[]string` 并逐个 `ResolveMeasureDriver`；绑定冲突按集合判定
- [x] 2.4 新增 `MeasureDeviceIDs()` 访问器；`MeasureDriver()` 保留（返回首个设备驱动，兼容旧调用方）
- [x] 2.5 逐设备操作方法加 `deviceID` 参数：`ReadMeasureData`、`ReadValveStatus`、`SetValveStatus`、`CalibrateZero`、`CalibrateFullScale`、`ReadMeasureUnit`、`SetMeasureUnit`、`ReadDeviceInfo`、`ResetDevice`（保留无参版本兼容）
- [x] 2.6 `EventSessionDeviceBound` payload 增加 `measureDeviceIds`

## 3. API / 事件契约（backend）

- [x] 3.1 `internal/api/http/device_session_handler.go`、`calibration_handler.go`：`setDevices` / `setMeasureDevice` 请求支持 `measureDeviceIds []string`（保留单设备字段）
- [x] 3.2 `sessionReadMeasureDataHandler` / `calibrationCollectHandler` 响应增加 `devices map[string]data`（保留 `data`）
- [~] 3.3 `internal/events/event_types.go` 相关事件 payload 文档 / 常量注释增加 `deviceId`；`internal/application/session`、`calibration`、`measurement` 发布事件时携带 `deviceId`
  - 偏差：事件发布已携带 `deviceId`（session/calibration/measurement 各发布点均已加），但 `event_types.go` 常量注释未逐条补充 `deviceId` 说明（事件常量本身无 payload 结构，注释补充价值有限，留待归档时一并处理）

## 4. 标定采集并行化（backend calibration）

- [x] 4.1 `calibration/collector.go` `Collect`：按设备并行采集（WTN1604 走 `CollectCalibrationPoint`，否则 `CollectData`），`sync.WaitGroup` + 每设备结果聚合为 `map[deviceID][]float64`；单设备路径复用现有逻辑
- [x] 4.2 采集结果写入 `pressurePoints[i].CollectedByDevice`；单设备同时回填 `CollectedData`
- [x] 4.3 `EventDataCollected` / `EventPointCompleted` payload 携带 `deviceId` 与对应设备数据
- [x] 4.4 `calibration/service.go`：`StartCalibration` / `EndCalibration` / `ValidateStartPrerequisites` 遍历设备集合（阀门门禁、StartCalibration/EndCalibration 逐设备）
- [x] 4.5 `calibration/service.go` 新增 `skippedDevices map[string]string`（deviceID → reason）；`executePointLoop` 每点过滤被跳过设备
- [x] 4.6 被跳过设备剩余点标记设备级 `skipped` + 原因，保留已完成数据

## 5. 报警 / 决策设备维度（backend calibration + measurement）

- [x] 5.1 `calibration/service.go` `checkAlarm`：逐设备评估超限通道，报警事件携带 `deviceId`；决策支持设备级跳过
- [x] 5.2 `calibration/service.go` `collectPoint` / `handlePointError`：设备失败 → 整批暂停 → `await_alarm_resolution` 等待用户「重试整点 / 跳过该设备 / 停止」
- [x] 5.3 `measurement/alarm.go` `CheckAlarm`：逐设备评估，报警事件携带 `deviceId`；决策支持设备级跳过
- [x] 5.4 `measurement/collector.go`：`ManualCollect` / `prepareCollectStep` / `updatePointCollectedData` 按设备并行采集与写入
- [x] 5.5 前端报警弹窗增加「跳过该设备 + 原因（预设 + 备注）」选项（后端预留 `ResolveSkipDevice(deviceID, reason)`）

## 6. 计量采集并行化（backend measurement）

- [x] 6.1 `measurement/collector.go` `RunAutoCollection` / `ManualCollect`：并行采集所有未跳过设备，`updatePointCollectedData` 写入 `CollectedByDevice`
- [x] 6.2 `measurement/service.go`：`Start` / `StartWorkflow` 校验设备集合；`CollectedRow` 增加 `deviceId`
- [x] 6.3 `EventMeasurementDataUpdated` / `EventMeasurementDataCollected` / `EventMeasurementPointStatus` 携带 `deviceId`
- [x] 6.4 `measurement/service.go` `WriteCSV` / `rowsFromPoints` 支持设备维度数据

## 7. 报告按设备输出（backend report）

- [x] 7.1 `internal/report/report_service.go`：`collectChannelData` / `collectBackwardData` / `collectMeasurementChannelData` / `collectMeasurementChannelByTarget` 支持设备维度聚合（回退旧字段）
- [x] 7.2 `ExportReport` / `ExportMeasurementReport` 按设备 ID 列表分别生成报告文件
- [x] 7.3 报告设备编号元数据从各设备配置派生

## 8. 前端类型 / store / API

- [x] 8.1 `web/src/types/device.ts` / `web/src/api/session.ts` / `calibration.ts` / `measurement.ts`：DTO 支持 `measureDeviceIds` 与设备维度数据
- [x] 8.2 `web/src/stores/deviceStore.ts`：`ModuleDeviceSelection` 增加 `measureDeviceIds`
- [x] 8.3 `web/src/stores/calibration/`：`pushCalibrationConfigAndStart` 收集所有已勾选 connected 设备；`pressurePoints.ts` `collectData` 支持设备维度；`deviceControl.ts` 多选绑定
- [x] 8.4 `web/src/stores/measurement/`：`ensureDevicesBound` / `bindDevices` 支持多设备；`channelData` 按设备
- [x] 8.5 `web/src/shared/events.ts` 事件类型增加 `deviceId`

## 9. 前端 UI

- [x] 9.1 `DeviceSelectionPanel` / `Device1604Panel` / `MeasurementDevicePanel`：单选改多选（`el-checkbox-group`）；选 1 台行为不变
- [x] 9.2 `CalibrationDataView` / 计量数据视图：按设备 tab / 卡片展示各设备通道数据与状态（含「已跳过」标记 + 原因）
- [x] 9.3 报警弹窗增加「跳过该设备」动作与原因输入（共享 `SkipDeviceDialog`：预设原因 + 备注）
- [x] 9.4 报告导出按设备分别触发 / 展示（导出成功提示多文件数量）

## 10. 配置层与回归

- [x] 10.1 `internal/config/app_config.go` 消费 `MeasureDeviceIDs`：绑定恢复优先读取切片
- [x] 10.2 单设备回归验证：选 1 台设备时全流程行为与改造前一致（前端多选组件选 1 台时行为不变；后端单设备路径复用原逻辑）
- [x] 10.3 运行 `make check`（go test + vet、vue typecheck + lint + test）全量通过
- [~] 10.4 更新 `openspec/specs/` 归档本变更规格（`openspec archive` 或 apply 后归档）
  - 偏差：`openspec` CLI 在本机不可用（npm 安装失败），归档动作留待环境就绪后执行；本文件即变更规格的权威记录

## 偏差记录

1. **3.3 事件常量注释**：`event_types.go` 常量注释未逐条补充 `deviceId`；事件发布点已全量携带（含单设备路径 `EventMeasurementDataCollected`、`EventPointCompleted`），归档时补充注释。
2. **10.4 归档**：`openspec archive` 因 CLI 不可用未执行，待环境就绪后归档。
3. **前端多设备实时采样**：`EVENT_MEASUREMENT_DATA_UPDATED` 按设备分行写入 `rows`（`CollectedRow.deviceId`），实时采样表多设备时增加「设备」列；单设备行为不变。
4. **设备状态区展示**：`MeasurementDevicePanel` / `Device1604Panel` 的设备状态区（阀门/单位/校零）仍以首个勾选设备为准，阀门切换按钮整批下发（后端 `SetValveStatusAllDevices` 已支持）；多设备逐台单位设置依赖各设备面板切换查看。

## 评审修复记录（2026-08-25 code-review 后）

1. **跨模块绑定冲突**：校准模块全部绑定调用统一 `moduleName='calibration'`（`deviceControl.ts`），与计量默认 `'measurement'` 区分，后端 `boundBy` 冲突检查恢复生效。
2. **设备失败点状态**：标定 `CollectDevices` 多设备路径有设备失败时不再先置点 completed / point_done，由 `checkAlarm` 转入 `await_alarm_resolution`；`awaitAlarmDecision` 的跳过设备 / 继续分支补点 completed 收尾。计量 `finalizePointCollect` 补多设备成功路径点状态 completed。
3. **计量实时采样**：`startCollectLoop` 改为按设备并行读取；单设备失败独立计数并发布设备级 `point.error`（不整批转 error），前端 `useMeasurementSync` 监听提示。
4. **last-devices 恢复闭环（10.1）**：前端 `fetchLastDevices` + `deviceStore.restoreLastDevices`，设备选择页挂载时恢复勾选，两个设备面板初始勾选优先读上次记录。
5. **单设备双写**：计量 `updatePointCollectedData` 单设备路径同时写 `CollectedData` 与 `CollectedByDevice`；单设备 `EventMeasurementDataCollected`、标定 `EventPointCompleted` 携带 `deviceId`。
6. **报告设备编号**：多设备报告按设备派生编号（设备配置无独立编号字段，以设备 ID 作为每台报告编号）。
7. **跳过原因拼接**：`SkipDeviceDialog` 预设「设备故障/采集超时/人工放弃/其他」+ 可选备注，拼接为「预设 - 备注」（spec 场景「采集超时 - 线缆接触不良」）。
8. **共享门禁实现**：`checkValveCalibrationGate` 抽至 `internal/device/valvegate.go`（标定与计量共用，消除跨包重复）。
9. **bind 合并**：`session.ts` 删除 `bindMeasureDevices`，统一 `bindDevices`（支持单/多设备 + trim 去重）。
10. **字号 token 化**：`SkipDeviceDialog` / `DeviceSelectionPanel` / `MeasurementDataView` 的 10px/13px 字号对齐 DESIGN.md Label（12px/500）层级。

**遗留（判断项，未重构）**：两个 DevicePanel 同构逻辑抽共享 composable、handler 首设备兼容回填与单位同步循环去重——功能正确，属代码味道，后续有需要再处理。