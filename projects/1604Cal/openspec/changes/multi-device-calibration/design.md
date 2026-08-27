## Context

当前标定 / 计量链路将「计量设备（1604）」建模为会话中的**单一实例**：

- `session.Service` 持有单个 `measureDriver` + `measureDevID`（`internal/application/session/service.go`）。
- `BindingToken` 只携带一个 `MeasureDeviceID`。
- `domain.PressurePoint.CollectedData []float64` 是单设备、单通道数组。
- `domain.WorkflowSession.MeasureDeviceID string` 是单一设备身份。
- 标定 `calibration.Service`、计量 `measurement.Service` 的采集 / 拟合 / 报警 / 暂停恢复全部围绕 `sessionService.MeasureDriver()` 单例驱动。
- 前端 store（`stores/calibration`、`stores/measurement`、`stores/deviceStore`）均为单 measure 设备选择；`DeviceSelectionPanel` / `Device1604Panel` 为单选框。
- 报告 `internal/report/` 从 `session.Points[].CollectedData`（单设备）聚合通道数据。

同时，基础设施已具备多设备能力：`deviceconnect.Service.activeDrivers` 是按 `deviceID` 的 map、`DeviceStore` 按 ID、`DriverResolver` 可逐 ID 解析驱动、`internal/config/app_config.go` 已预留 `LastDevicesConfig.MeasureDeviceIDs []string`。因此本设计的核心不是「能不能连多台」，而是把「绑定 → 采集 → 报警 → 报告」从单设备维度改为设备集合维度。

本需求与现有 `multipress` 模块无关：`multipress` 管理的是多台**打压**设备，本需求是**一台打压 + 多台 1604 计量**。

## Goals / Non-Goals

**Goals:**

- 标定 / 计量开始前可勾选多台已连接的 1604 作为本批次计量设备；选 1 台时保持现有单设备流程。
- 每个稳定压力点，并行触发所有参与计量设备的采集；全部成功（或失败设备已跳过）后进入下一压力点。
- 任一设备失败 / 超时 / 断开 → 整批暂停并报警；用户可重试或**永久跳过**该设备（本批次剩余压力点不再参与）。
- 被跳过设备保留在批次结果中，标记「计量未完成 / 已跳过」，保留已完成数据与跳过原因（预设选项 + 可选备注）。
- 每台设备分别保存身份、原始数据、计算结果与独立报告。
- 事件 / SSE / 前端按设备维度展示。
- 配置层消费已预留的 `MeasureDeviceIDs`。

**Non-Goals:**

- 不改动 `multipress`（多打压设备）模块。
- 不引入「多台打压设备」或「不同设备使用不同压力点配置」。
- 不重构现有的单设备状态机语义；多设备是单设备流程的并行扩展（1 台 = 原逻辑）。
- 不实现设备热插拔式「中途新增」——设备集合在批次开始时锁定。

## Decisions

### D1: 会话绑定改为设备集合（map 维度）

`session.Service` 的 `measureDriver`/`measureDevID` 单槽改为：

```go
measureDrivers map[string]device.MeasureDriver  // deviceID -> driver
measureDevIDs  []string                          // 有序，保持用户勾选顺序
pressureDriver device.PressureDriver
pressureDevID  string
```

`BindDevices(measureDevIDs []string, pressureDevID string, moduleName string)`、`BindMeasureDevice(measureDevIDs []string, moduleName string)` 逐个解析并注册驱动。`BindingToken.MeasureDeviceID string` 改为 `MeasureDeviceIDs []string`，`validateToken` 改为集合成员校验（token 的每个 ID 都必须在当前会话集合中）。

**为什么：** 采集、阀门、单位、设备信息等操作最终都要落到具体设备 ID；map + 有序切片能同时满足「按 ID 取驱动」和「保持用户顺序」。

**备选方案：** 引入独立的 `MultiMeasureSession` 复合对象。→ 复杂度更高，且与现有 `WorkflowCoordinator` 单活语义、`ValidateStartPrerequisites` 门禁冲突；map 方案改动面更小、更贴合现有结构。

### D2: 数据模型增加设备维度

- `domain.PressurePoint.CollectedData []float64` **保留**（兼容单设备 / 旧数据），新增：

```go
CollectedByDevice map[string]DevicePointData `json:"collectedByDevice,omitempty"`

type DevicePointData struct {
    DeviceID    string    `json:"deviceId"`
    Collected   []float64 `json:"collected"`
    Status      string    `json:"status"`      // completed | error | skipped
    CollectTime string    `json:"collectTime,omitempty"`
    SkipReason  string    `json:"skipReason,omitempty"`
}
```

- `domain.WorkflowSession.MeasureDeviceID string` 保留（兼容），新增 `MeasureDeviceIDs []string`。

**为什么：** 保留原字段可避免一次性破坏所有消费方；`CollectedByDevice` 承载多设备数据与「每设备状态 / 跳过原因」。单设备路径继续读写 `CollectedData`，多设备路径读写 `CollectedByDevice`，迁移时优先读多设备结构、缺失则回退单设备。

**备选方案：** 直接把 `CollectedData` 改造成 `map[string][]float64`。→ 会对 CSV、报告、前端 `channelValues`、`softwareFit` 等所有读点造成一次性破坏，且 `[]float64` 无法表达「跳过原因 / 设备级状态」。增量结构更稳。

### D3: 采集器并行化（calibration + measurement）

标定 `calibration/collector.go` 与计量 `measurement/collector.go` 的采集核心改为：

1. 在锁内快照当前参与设备 ID 集合（已绑定且未被跳过）。
2. 对每个设备启动 goroutine：`CollectData(ctx, channels)` 或 `CollectCalibrationPoint(...)`（WTN1604），多样本平均、按精度截断。
3. 使用 `sync.WaitGroup` + 每设备结果通道聚合，收集 `map[deviceID][]float64`。
4. 单设备路径（`len==1`）直接复用现有 `Collect` 单设备逻辑，避免回归。

采集结果写入 `pressurePoints[pointIndex-1].CollectedByDevice`，同时单设备时回填 `CollectedData`。

**为什么：** 「并行触发 + 等待全部完成」正是需求的核心；`multipress` 的 `pollDevicesConcurrently` 已是同构先例（map + 每设备 mutex + WaitGroup）。每设备独立 `ctx`（带超时）保证单台卡死不阻塞整批——超时视为该设备失败进入报警。

**风险→缓解：** 并发采集时驱动连接是否线程安全。→ 每设备驱动在会话层已是独立实例（`measureDrivers` map），驱动内部的命令队列负责串行化；采集只发生在压力稳定后，无并发写同一驱动的风险。

### D4: 失败 / 报警 / 永久跳过改为设备维度

- 标定 `checkAlarm` 与计量 `CheckAlarm` 改为**逐设备**评估：对每台设备的 `CollectedByDevice[devID].Collected` 独立判定超限通道，任一设备触发即报警，事件带 `deviceId`。
- 报警决策扩展：`continue`（全部继续）/ `recollect`（重采该点所有设备）/ `skip`（**跳过该设备**，从本批剩余流程移除）/ `stop`。新增决策语义由前端弹窗体现（含跳过原因预设选项 + 可选备注）。
- `Service` 维护 `skippedDevices map[string]string`（deviceID → skipReason）。`executePointLoop` 在每点开始时过滤掉 `skippedDevices` 中的设备；这些设备的点在 `CollectedByDevice` 中标记 `skipped` + 原因。
- 跳过的设备仍出现在 `EventCalibrationPointStatus` 与批次结果中（状态 `skipped`）。

**为什么：** 需求明确「整批暂停 + 用户可重试或永久跳过单台」。「跳过」作用于设备而非整点，是区别于现有 `PointStatusSkipped` 的关键。

### D5: 报告按设备输出

- `internal/report/report_service.go`：`ExportReport` 接受设备 ID 列表；对每台设备分别生成一份报告（或同一报告内按设备分节，取决于模板能力——先实现「每设备一个文件」）。
- `collectChannelData` / `collectBackwardData` / `collectMeasurementChannelData` / `collectMeasurementChannelByTarget` 增加 `deviceID` 维度：从 `CollectedByDevice[devID].Collected` 聚合；单设备回退到 `CollectedData`。
- 会话身份用 `MeasureDeviceIDs`；「设备编号」从各设备配置派生。

**为什么：** 需求要求「分别保存各自的结果与报告」。逐设备独立文件是与现有单文件模板改动最小的路径。

### D6: API / 事件契约扩展（向后兼容）

- `calibrationSetDevicesRequest` / `sessionSetDevicesRequest`：`measureDeviceId` 保留（单设备），新增 `measureDeviceIds []string`（多设备，优先）。
- `calibrationCollect` / `sessionReadMeasureData`：响应 `data` 保留，新增 `devices map[deviceID]data`。
- 事件：`EventDataCollected`、`EventPointCompleted`、`EventMeasurementDataUpdated`、`EventMeasurementDataCollected`、`EventCalibrationPointStatus` 的 payload 增加 `deviceId` 字段（单设备时为该设备 ID，兼容旧前端）。
- `EventSessionDeviceBound` payload 的 `measureDeviceId` 保留，新增 `measureDeviceIds`。

**为什么：** 前后端可独立发布；旧前端读单设备字段不中断。

### D7: 前端按设备维度选择与展示

- `stores/deviceStore.ts` 的 `ModuleDeviceSelection` 增加 `measureDeviceIds: string[]`（保留 `measureDeviceId` 兼容）。
- `DeviceSelectionPanel` / `Device1604Panel` / `MeasurementDevicePanel`：单选改多选（`el-checkbox-group` / 多选 `el-select`）；选 1 台时行为不变。
- `CalibrationDataView` / 计量数据视图：按设备分 tab / 分卡片展示各设备的通道数据与状态（含「已跳过」标记与原因）。
- `stores/calibration/index.ts` 的 `pushCalibrationConfigAndStart`：`measureDevices.find(connected)` 改为收集所有已勾选且 connected 的设备；选 1 台调用现有 `setDevices`，选多台调用新多设备 API。

**为什么：** 前端是多设备能力的第一现场（勾选、并行状态、跳过操作、报告导出），必须与后端契约对齐。

## Risks / Trade-offs

- [并发采集对驱动 / 命令队列的影响] → 每设备独立驱动实例 + 会话级 map；采集仅在压力稳定后触发，单设备回归路径复用现有逻辑。若现场发现某设备驱动非并发安全，在采集器内对该设备串行化（已有 `multipress` 先例）。
- [数据模型双字段（CollectedData + CollectedByDevice）漂移] → 写入路径统一：多设备写 `CollectedByDevice`，单设备同时写两处；读取统一「多设备优先、缺失回退单设备」，并在 spec 中明确该优先级。
- [报警 / 跳过语义变复杂] → 通过逐设备报警事件携带 `deviceId`，前端弹窗明确「重试整点 / 跳过该设备 / 停止」三种动作 + 跳过原因，spec 覆盖决策状态机。
- [报告多文件 vs 模板改动] → 先做「每设备一个文件」，若现场要求单文件分节再扩展；避免一次改动模板引擎。
- [BREAKING 风险] → 所有 DTO / 事件字段向后兼容（旧字段保留），前后端独立演进；`make check` 全量门禁 + 现有 `openspec/specs/` 回归用例兜底。

## Migration Plan

1. 后端：session 绑定集合化 → 数据模型加字段 → 采集器并行化 → 报警 / 跳过设备维度 → API / 事件契约 → 报告。
2. 前端：类型 / store → 多选 UI → 数据视图 → 流程接线。
3. 单设备回归：1 台设备走新代码时行为与旧逻辑一致（spec 用「选 1 台 = 原流程」约束）。
4. 回滚：旧版本前端读旧字段仍可用；后端可回退到单槽实现（`MeasureDeviceIDs` 长度 ≤1 时走原路径）。

## Open Questions

- 报告是否需要在单文件内按设备分节（vs 每设备一文件）？→ 默认每设备一文件，现场反馈后再定。
- 跳过原因预设选项文案（「设备断开 / 采集超时 / 数据异常 / 人工放弃」）是否需要配置化？→ 默认硬编码预设 + 可选备注。