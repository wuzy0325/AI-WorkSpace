## Why

当前 1604Cal 的标定 / 计量流程把「计量设备（1604）」绑定为**单一实例**（`session.Service` 只有一个 `measureDriver`/`measureDevID` 槽位）。现场需要在同一压力环境下，用**一台打压设备**同时为**多台 1604** 执行同一批次的计量，以缩短逐台重复计量的操作时间，并保证各 1604 在同一压力点条件下采集、提升批量数据一致性。基础设施（`deviceconnect.activeDrivers`、`DeviceStore`、驱动解析器）已支持同时连接多台 1604，缺的是从绑定 → 采集 → 报警 → 报告一路贯穿「多设备维度」的支持。

## What Changes

- **BREAKING**：会话绑定支持多计量设备。`session.Service` 的单一 `measureDriver`/`measureDevID` 槽位改为按设备 ID 索引的集合；`BindingToken` 携带设备 ID 集合。
- **BREAKING**：标定 / 计量采集按设备维度存储数据。`domain.PressurePoint.CollectedData []float64` 改为按设备维度的结构（设备 ID → 通道数组），下游采集、报警、CSV、报告随之适配。
- 标定 / 计量流程在每个稳定压力点**并行触发**所有参与计量设备采集，全部成功（或异常设备已跳过）后才进入下一压力点。
- 任一设备在压力点失败 / 超时 / 断开时，整批暂停并报警；用户可重试，或**永久跳过**该设备（本批次剩余压力点不再参与）。
- 被跳过的设备仍保留在本批次结果中，标记为「计量未完成 / 已跳过」，保留已完成压力点数据与跳过原因（预设选项 + 可选备注）。
- 标定 / 计量开始前自动发现已连接设备，用户勾选参与设备；选 1 台维持现有单设备流程，选多台进入同步流程。多设备数量不设业务上限，以实际连接数与通信资源为限。
- 每台设备分别保存设备身份、原始数据、计算结果与独立报告。
- 事件（SSE）携带 `deviceId`，前端按设备展示数据。
- 复用 `internal/config/app_config.go` 已预留的 `LastDevicesConfig.MeasureDeviceIDs []string`。

## Capabilities

### New Capabilities
- `multi-measure-binding`: 会话 / API / 前端支持选择并绑定多台计量设备，绑定令牌与持久化携带设备 ID 集合。
- `multi-device-collection`: 标定与计量流程在每个压力点并行采集所有参与计量设备，按设备维度存储与上报数据，并处理失败 / 重试 / 永久跳过。
- `per-device-report`: 报告生成按设备分别输出，携带各自的设备身份与数据。

### Modified Capabilities
- `measurement-state-machine`: 采集服务状态机在「多设备」场景下的暂停 / 恢复 / 完成语义，以及多设备并行采集对状态迁移的影响。
- `calibration-alarm-decision-flow`: 报警判定与用户决策（continue / recollect / skip / stop）从单设备数据改为按设备维度评估，跳过决策作用于设备维度。

## Impact

- **后端**：`internal/application/session/`（绑定、数据读取、阀门、校准、单位、设备信息、复位均需 deviceID）、`internal/application/calibration/`（service / collector / pressure / alarm）、`internal/application/measurement/`（service / collector / workflow / alarm）、`internal/domain/pressure_point.go` 与 `workflow_session.go`、`internal/api/http/`（session / calibration / measurement handler 请求与响应结构）、`internal/events/event_types.go`。
- **报告**：`internal/report/`（collectChannelData / collectBackwardData / 计量通道数据聚合）。
- **前端**：`web/src/api/`、`web/src/stores/`（calibration / measurement / deviceStore / measurement-deviceStore）、`web/src/components/`（DeviceSelectionPanel / Device1604Panel / MeasurementDevicePanel / CalibrationDataView）、`web/src/composables/`（useCalibrationSync / useMeasurementSync）、`web/src/shared/events.ts`、`web/src/types/device.ts`。
- **配置**：`internal/config/app_config.go` 已预留 `MeasureDeviceIDs`，需在绑定 / 恢复流程中消费。
- **不涉及**：`multipress` 模块（它是多**打压**设备，非本需求）；单设备现有流程行为保持兼容。
