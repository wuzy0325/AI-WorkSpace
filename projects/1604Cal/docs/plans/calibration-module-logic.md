# Cal1604 标定模块（Calibration）业务逻辑梳理

> 文档生成日期：2026-04-17  
> 基于代码路径：`web/src/views/CalibrationView.vue`、`web/src/stores/calibration/index.ts`、`internal/application/calibration/service.go`、`internal/api/http/session_handler.go`、`internal/workflow/session_machine.go` 及相关组件。

---

## 1. 模块概述

### 1.1 职责与定位

标定模块（Calibration）是 Cal1604 项目的核心功能之一，用于完成**压力计量设备（WTN1604）的多点校准流程**。其职责包括：

- **设备连接管理**：连接 1604 计量设备与打压设备（压力控制器）。
- **通道配置**：选择需要采集的通道（1~16 通道）。
- **压力点管理**：根据量程范围生成等分压力点，支持单程/回程模式。
- **自动/手动采集**：支持自动循环打压-稳定-采集，也支持手动逐点控制。
- **数据拟合**：采集完成后执行线性拟合，并将系数写入设备（WTN1604 硬件拟合）或软件拟合。
- **状态同步**：通过 SSE + 轮询实时同步会话状态、压力值、稳定状态、计量数据。

### 1.2 页面布局

页面采用**侧边栏 + 主工作区**布局：

- **侧边栏**：设备连接（1604 + 打压）、通道选择、启动条件检查。
- **主工作区**：流程进度条、参数配置、控制按钮、标定数据表格、报告模板选择。

---

## 2. 核心业务流程

```
设备连接 → 通道选择 → 启动校准 → 数据采集 → 数据拟合 → 结束/重置
```

### 2.1 设备连接

1. **1604 计量设备**：
   - 前端通过 `Device1604Panel` 选择设备，调用 `deviceStore.connectMeasureDevice(deviceId)`。
   - 连接成功后，调用 `setCalibrationMeasureDevice(deviceId)` 将驱动绑定到校准服务。
   - 读取阀门状态、单位、设备信息（带重试机制）。

2. **打压设备**：
   - 前端通过 `PressDevicePanel` 选择设备，调用 `multipressRegister(deviceId)`。
   - 后端通过多设备打压模块创建驱动、建立 TCP 连接、注册到压力控制模块。
   - 前端手动同步 `deviceStore.updateDeviceStatus(deviceId, 'connected')`。

### 2.2 通道选择

- 通过 `ChannelMatrix` 组件选择 1~16 通道，支持全选/清空。
- 选择后调用 `setCalibrationChannels(channels)` 同步到后端。

### 2.3 启动校准

- 点击"开始"按钮，前端先调用 `setCalibrationDevices(measureDevId, pressureDevId)` 通知后端绑定双设备。
- 再调用 `POST /sessions/start`，后端执行：
  1. 状态机从 `idle/stopped/completed` → `ready`。
  2. 设置 1604 阀门为 `calibration` 模式。
  3. 若为 WTN1604，发送硬件校准开始命令（`StartCalibration`）。
  4. 若控制模式为 `auto`，启动后台 goroutine 自动采集循环（`RunAutoCollection`）。

### 2.4 数据采集

#### 自动模式

后端 `RunAutoCollection` 按压力点列表依次执行：

```
pressurizing（打压） → stabilizing（稳定等待） → collecting（采集） → point_done（单点完成）
```

- `Pressurize`：设置目标压力，启动压力控制，等待稳定（`waitForStability`），读取实际压力。
- `Collect`：多次采样求平均（`averageCount` 次），计算各通道平均值，标记点为 `completed`。
- `checkAlarm`：采集后做简单阈值检查（偏差 > 5% 触发报警），进入 `await_alarm_resolution`，等待用户决策 `continue` 或 `retry`。

#### 手动模式

前端表格每行根据 `status` 显示不同操作按钮：

| 状态 | 可操作 |
|------|--------|
| `pending` | 打压 |
| `pressurizing` / `stabilizing` | 确认压力、采集 |
| `completed` | 重新采集 |

前端分别调用：
- `pressurize(pointId)` → `POST /calibration/pressurize`
- `confirmPressure(pointId)`（纯前端状态变更）
- `collectData(pointId)` → `POST /calibration/collect`

### 2.5 数据拟合

- 点击"拟合"按钮，调用 `POST /calibration/fit`。
- 后端 `Service.Fit`：
  - 状态机迁移到 `fitting`。
  - 若计量设备为 `WTN1604Driver`，调用硬件拟合（`PerformFitting`）并保存系数（`SaveCoefficients`）。
  - 否则执行**软件最小二乘线性拟合**（`softwareFit`），计算斜率 `a` 和截距 `b`。
  - 完成后迁移到 `completed`。

### 2.6 结束 / 重置

- **停止**：`POST /sessions/stop`，状态机 → `stopped`，结束校准（阀门切回测量、停止压力、停止自动采集 goroutine）。
- **结束（前端）**：`endCalibration()`，若会话仍在运行则先 stop，然后清空前端状态（通道、压力点、步骤重置为 `DEVICE_CONNECT`）。
- **重置采集数据**（计量模块专用）：`resetCollection()`，仅重置测点状态为 `pending`，保留配置。

---

## 3. 前端状态管理（Pinia Store）

Store 文件：`web/src/stores/calibration/index.ts`

### 3.1 State

| 字段 | 类型 | 说明 |
|------|------|------|
| `currentStep` | `CalibrationStep` | 当前流程步骤（0~5） |
| `selectedChannels` | `number[]` | 已选通道 |
| `pressurePoints` | `PressurePoint[]` | 压力点列表 |
| `calibrationParams` | `CalibrationParams` | 校准参数配置 |
| `isCollecting` | `boolean` | 是否采集中 |
| `currentCollectingPoint` | `number` | 当前采集点索引 |
| `sessionState` | `SessionState` | 后端会话状态 |
| `currentPressure` | `number` | 当前压力值（轮询） |
| `isStable` | `boolean` | 是否稳定（轮询） |
| `channelData` | `number[]` | 实时计量数据（轮询） |
| `valveStatus` | `string` | 阀门状态 |
| `measureUnit` | `string` | 计量单位 |
| `deviceInfo` | `Record<string, string>` | 设备信息 |

### 3.2 Getters

| Getter | 说明 |
|--------|------|
| `device1604Connected` | 是否有状态为 `connected` 的计量设备 |
| `pressDeviceConnected` | 是否有状态为 `connected` 的打压设备 |
| `channelsSelected` | 已选通道数 > 0 |
| `hasCollectedData` | 是否存在 `completed` 状态的压力点 |
| `canStartCalibration` | 三条件同时满足（1604、打压、通道） |
| `isRunning` | 会话状态是否为运行中（pressurizing/stabilizing/collecting/...） |

### 3.3 Actions

| Action | 说明 |
|--------|------|
| `syncSessionState(state)` | 同步后端会话状态，并映射到 `currentStep` |
| `fetchCurrentSessionState()` | 主动拉取后端会话状态 |
| `connectDevice1604(deviceId)` | 连接计量设备并绑定到校准服务 |
| `disconnectDevice1604(deviceId)` | 断开计量设备 |
| `connectPressDevice(deviceId)` | 注册打压设备到 multipress 服务 |
| `disconnectPressDevice(deviceId)` | 注销打压设备 |
| `setSelectedChannels(channels)` | 设置通道并同步后端 |
| `generatePressurePoints(opts)` | 设置配置并生成压力点（当前在计量模块调用） |
| `startCalibration()` | 开始校准流程 |
| `pauseCalibration()` | 暂停 |
| `resumeCalibration()` | 恢复 |
| `stopCalibration()` | 停止 |
| `pressurize(pointId)` | 对某点执行打压 |
| `confirmPressure(pointId)` | 确认压力（前端状态变更） |
| `collectData(pointId)` | 采集某点数据 |
| `fitData()` | 执行拟合 |
| `endCalibration()` | 结束并重置前端状态 |
| `refreshPressure()` / `refreshStability()` / `refreshMeasureData()` | 轮询实时数据 |

---

## 4. 后端核心逻辑

### 4.1 服务层（`internal/application/calibration/service.go`）

`Service` 是校准流程的编排核心，主要职责：

- **设备绑定**：`SetMeasureDevice`、`SetDevices`，优先复用已连接的驱动实例（通过 `driverProvider`）。
- **配置管理**：`SetConfig`、`SetChannels`、`GeneratePressurePoints`。
- **单点控制**：`Pressurize`、`Collect`、`Fit`。
- **自动采集**：`RunAutoCollection` 在后台 goroutine 中循环执行 `collectPoint`。
- **报警处理**：`checkAlarm` + `ResolveAlarm` + `RetryPoint`。
- **生命周期控制**：`StartCalibration`、`EndCalibration`、`PauseAutoCollection`、`ResumeAutoCollection`、`StopAutoCollection`。
- **实时读取**：`ReadCurrentPressure`、`ReadStability`、`ReadMeasureData`、`ReadValveStatus`、`ReadMeasureUnit`、`ReadDeviceInfo`。

#### 自动采集 goroutine 控制

- 使用 `autoCollectionCtx / autoCollectionCancel` 控制取消。
- 使用 `autoCollectionMu` 保护取消函数，防止重复启动或竞态。
- 停止时同时关闭 `alarmCh`，解除可能阻塞的报警等待。

### 4.2 状态机（`internal/workflow/session_machine.go`）

`SessionMachine` 基于读写锁保护的状态迁移表，定义了所有合法的会话状态转换：

```go
idle → ready
ready → pressurizing | stopped
pressurizing → stabilizing | paused | error | stopped
stabilizing → collecting | await_manual_collect | paused | error | stopped
collecting → point_done | await_alarm_resolution | paused | error | stopped
point_done → pressurizing | fitting | stopped
fitting → completed | error | stopped
completed → ready | stopped
paused → pressurizing | stopped
error → recovering | stopped
stopped → ready
...
```

非法迁移会被拦截并返回错误。

### 4.3 API 处理层

#### 会话相关（`internal/api/http/session_handler.go`）

| 接口 | 方法 | 说明 |
|------|------|------|
| `/sessions/state` | GET | 获取当前会话状态 |
| `/sessions/start` | POST | 启动校准（调用 `StartCalibration`） |
| `/sessions/pause` | POST | 暂停自动采集 + 状态机 → `paused` |
| `/sessions/resume` | POST | 恢复自动采集 + 状态机 → `pressurizing` |
| `/sessions/stop` | POST | 状态机 → `stopped` + 结束校准（`EndCalibration`） |

#### 校准相关（`internal/api/http/calibration_handler.go`）

| 接口 | 方法 | 说明 |
|------|------|------|
| `/calibration/devices` | POST | 设置双设备 |
| `/calibration/measure-device` | POST | 仅设置计量设备 |
| `/calibration/config` | POST | 设置校准配置 |
| `/calibration/channels` | POST | 设置通道 |
| `/calibration/channels/list` | GET | 获取通道 |
| `/calibration/points/generate` | POST | 生成压力点 |
| `/calibration/points` | GET | 获取压力点列表 |
| `/calibration/pressurize` | POST | 打压 |
| `/calibration/collect` | POST | 采集 |
| `/calibration/fit` | POST | 拟合 |
| `/calibration/pressure` | GET | 读取当前压力 |
| `/calibration/stability` | GET | 读取稳定状态 |
| `/calibration/measure-data` | GET | 读取计量数据 |
| `/calibration/valve` | GET/POST | 读取/设置阀门状态 |
| `/calibration/measure-unit` | GET/POST | 读取/设置单位 |
| `/calibration/device-info` | GET | 读取设备信息 |
| `/calibration/reset` | POST | 复位计量设备 |
| `/calibration/alarm` | POST | 确认报警决策 |
| `/calibration/retry` | POST | 重试某点 |

---

## 5. 关键数据结构与类型

### 5.1 前端类型（`web/src/stores/calibration/index.ts`）

```ts
export enum CalibrationStep {
  DEVICE_CONNECT = 0,
  CHANNEL_SELECT = 1,
  START_CALIBRATION = 2,
  DATA_COLLECTION = 3,
  DATA_FITTING = 4,
  COMPLETED = 5
}

export interface PressurePoint {
  id: string
  index: number
  targetPressure: number
  status: 'pending' | 'pressurizing' | 'stabilizing' | 'collecting' | 'completed' | 'error'
  collectedData?: number[]
  actualPressure?: number
}

export interface CalibrationParams {
  minValue: number
  maxValue: number
  points: number        // 2~6
  precision: number     // 0~4
  averageCount: number  // 1~100
  stableTime: number    // 1/3/5/10 秒
  precisionLevel: string // '0.01' | '0.05' | '0.1' | '0.2'
}
```

### 5.2 后端类型（`internal/application/calibration/service.go`）

```go
type PressurePoint struct {
    Index          int       `json:"index"`
    TargetPressure float64   `json:"targetPressure"`
    Status         string    `json:"status"` // pending, pressurizing, stabilizing, collecting, completed, error
    CollectedData  []float64 `json:"collectedData,omitempty"`
    ActualPressure float64   `json:"actualPressure,omitempty"`
}

type CalibrationConfig struct {
    Channels       []int   `json:"channels"`
    PressurePoints int     `json:"pressurePoints"`
    AverageCount   int     `json:"averageCount"`
    MinPressure    float64 `json:"minPressure"`
    MaxPressure    float64 `json:"maxPressure"`
    StableWaitMs   int     `json:"stableWaitMs"`
    ControlMode    string  `json:"controlMode"`  // auto | manual
    PressureMode   string  `json:"pressureMode"` // single | roundTrip
}

type CalibrationResult struct {
    Success       bool                `json:"success"`
    State         domain.SessionState `json:"state"`
    CollectedData map[int][]float64   `json:"collectedData,omitempty"`
    Error         string              `json:"error,omitempty"`
}
```

### 5.3 会话状态（`internal/domain/session_state.go`）

```go
type SessionState string

const (
    SessionStateIdle                 SessionState = "idle"
    SessionStateReady                SessionState = "ready"
    SessionStatePressurizing         SessionState = "pressurizing"
    SessionStateStabilizing          SessionState = "stabilizing"
    SessionStateCollecting           SessionState = "collecting"
    SessionStatePointDone            SessionState = "point_done"
    SessionStateFitting              SessionState = "fitting"
    SessionStateCompleted            SessionState = "completed"
    SessionStatePaused               SessionState = "paused"
    SessionStateStopped              SessionState = "stopped"
    SessionStateAwaitManualCollect   SessionState = "await_manual_collect"
    SessionStateAwaitAlarmResolution SessionState = "await_alarm_resolution"
    SessionStateRecovering           SessionState = "recovering"
    SessionStateError                SessionState = "error"
)
```

---

## 6. 前后端交互接口

### 6.1 SSE 实时事件（`createEventStream`）

前端通过 `EventSource` 连接 `/api/v1/events/stream`，监听以下事件：

| 事件类型 | 说明 |
|----------|------|
| `session.state.changed` | 会话状态变更，前端调用 `syncSessionState(data.state)` |
| `device.status.changed` | 设备状态变更，刷新设备列表 |

### 6.2 轮询机制（`CalibrationView.vue`）

页面挂载后启动两个定时器：

- **校准数据轮询**（2 秒）：会话运行中时，并行拉取
  - `refreshPressure()` → `GET /calibration/pressure`
  - `refreshStability()` → `GET /calibration/stability`
  - `refreshMeasureData()` → `GET /calibration/measure-data`

- **设备列表刷新**（5 秒）：`deviceStore.loadDevices(true)`

### 6.3 关键 API 汇总

| 功能 | 前端函数 | 后端 Handler | HTTP |
|------|----------|--------------|------|
| 获取会话状态 | `fetchSessionState()` | `sessionStateHandler` | GET `/sessions/state` |
| 开始校准 | `triggerSessionAction('start')` | `sessionStartHandler` | POST `/sessions/start` |
| 暂停校准 | `triggerSessionAction('pause')` | `sessionPauseHandler` | POST `/sessions/pause` |
| 恢复校准 | `triggerSessionAction('resume')` | `sessionResumeHandler` | POST `/sessions/resume` |
| 停止校准 | `triggerSessionAction('stop')` | `sessionStopHandler` | POST `/sessions/stop` |
| 设置双设备 | `setCalibrationDevices()` | `calibrationSetDevicesHandler` | POST `/calibration/devices` |
| 设置计量设备 | `setCalibrationMeasureDevice()` | `calibrationSetMeasureDeviceHandler` | POST `/calibration/measure-device` |
| 设置配置 | `setCalibrationConfig()` | `calibrationSetConfigHandler` | POST `/calibration/config` |
| 设置通道 | `setCalibrationChannels()` | `calibrationSetChannelsHandler` | POST `/calibration/channels` |
| 生成压力点 | `generatePressurePoints()` | `calibrationGeneratePointsHandler` | POST `/calibration/points/generate` |
| 获取压力点 | `getPressurePoints()` | `calibrationGetPointsHandler` | GET `/calibration/points` |
| 打压 | `pressurize(index)` | `calibrationPressurizeHandler` | POST `/calibration/pressurize` |
| 采集 | `collectData(index)` | `calibrationCollectHandler` | POST `/calibration/collect` |
| 拟合 | `fitData()` | `calibrationFitHandler` | POST `/calibration/fit` |
| 读取压力 | `readCurrentPressure()` | `calibrationReadPressureHandler` | GET `/calibration/pressure` |
| 读取稳定 | `readStability()` | `calibrationReadStabilityHandler` | GET `/calibration/stability` |
| 读取计量数据 | `readMeasureData()` | `calibrationReadMeasureDataHandler` | GET `/calibration/measure-data` |
| 读取阀门 | `readCalibrationValve()` | `calibrationValveHandler` | GET `/calibration/valve` |
| 设置阀门 | `setCalibrationValve(status)` | `calibrationValveHandler` | POST `/calibration/valve` |
| 读取单位 | `readCalibrationMeasureUnit()` | `calibrationReadMeasureUnitHandler` | GET `/calibration/measure-unit` |
| 设置单位 | `setCalibrationMeasureUnit(unit)` | `calibrationReadMeasureUnitHandler` | POST `/calibration/measure-unit` |
| 读取设备信息 | `readCalibrationDeviceInfo()` | `calibrationReadDeviceInfoHandler` | GET `/calibration/device-info` |
| 复位设备 | `resetCalibrationDevice()` | `calibrationResetDeviceHandler` | POST `/calibration/reset` |
| 确认报警 | - | `calibrationResolveAlarmHandler` | POST `/calibration/alarm` |
| 重试点 | - | `calibrationRetryPointHandler` | POST `/calibration/retry` |

---

## 7. 当前已移除 / 废弃的功能说明

### 7.1 生成测点按钮（`PressurePointList.vue` 中废弃）

- 早期版本中，`PressurePointList` 组件提供独立的"生成压力点"按钮和点个数输入框。
- **当前主页面（`CalibrationView.vue`）已不再使用该组件**。标定模块在点击「开始」后，由后端 `StartCalibration` 根据当前 `CalibrationConfig` **隐式生成**压力点；计量模块则通过前端显式调用 `generatePressurePoints()` 生成。
- `PressurePointList.vue` 中的状态枚举（`pending_press`、`pending_confirm`、`pending_collect`）与当前 store 中的状态（`pending`、`pressurizing`、`stabilizing`、`collecting`、`completed`）已不一致，说明该组件处于**未使用或待清理**状态。

### 7.2 控制模式与打压模式

- `CalibrationView.vue` 中保留了 `controlMode`（auto/manual）和 `pressureMode`（single/return）的 UI 绑定，但**这两个变量是局部 `ref`，并未传入 `startCalibration()`**。
- Store 中的 `generatePressurePoints` 支持接收 `controlMode` 和 `pressureMode`，但标定页面未调用该函数（由后端隐式生成），计量页面虽调用但仅影响前端生成的配置。
- 更深层的问题是：`startCalibration()` 的 HTTP 请求并未携带 `controlMode` / `pressureMode` 参数，后端 `sessionStartHandler` 目前也未从请求体中读取这些字段。这是一个**前端有 UI、未传参，后端未接收的双向断层**。

### 7.3 报告模板选择

- 页面支持选择报告模板（`selectReportTemplate`），但模板选择结果仅显示文件名，**未与后续报告生成流程直接关联**（当前代码中无进一步使用 `templateFilename` 的逻辑）。

---

## 8. 组件清单

| 组件 | 路径 | 职责 |
|------|------|------|
| `CalibrationView.vue` | `web/src/views/` | 标定模块主页面，整合所有子组件、SSE、轮询、导出 |
| `Device1604Panel.vue` | `web/src/components/calibration/` | 1604 设备选择、连接、阀门/单位控制 |
| `PressDevicePanel.vue` | `web/src/components/calibration/` | 打压设备选择、连接、目标压力设定 |
| `ChannelMatrix.vue` | `web/src/components/calibration/` | 16 通道矩阵选择 |
| `ProgressIndicator.vue` | `web/src/components/calibration/` | 校准流程步骤条（6 步） |
| `CalibrationControlPanel.vue` | `web/src/components/calibration/` | 开始/拟合/结束控制（当前页面未直接使用，为独立组件） |
| `CalibrationDataTable.vue` | `web/src/components/calibration/` | 标定数据表格 + CSV 导出（独立封装，但标定页面当前内联了表格逻辑，未直接使用该组件） |
| `PressurePointList.vue` | `web/src/components/calibration/` | 压力点列表（含生成按钮，当前页面未使用） |

---

## 9. 总结

标定模块当前已形成**前后端分离、状态机驱动、SSE + 轮询双通道同步**的完整架构：

- **前端**：Pinia Store 集中管理状态，视图层以 `CalibrationView` 为核心，组件职责清晰。
- **后端**：`calibration.Service` 作为领域编排层，通过 `SessionMachine` 保证状态安全，自动采集 goroutine 可控可取消。
- **交互**：RESTful API 负责命令下发，SSE 负责状态推送，轮询负责实时数据刷新。

需要注意的遗留问题：
1. `PressurePointList.vue` 与当前状态模型不一致，建议清理或同步更新。
2. `controlMode` / `pressureMode` 在前端页面有 UI 但参数未实际传递，需补全链路。
3. 报告模板选择后未接入后续生成流程，需补充模板应用逻辑。
4. **标定模块与计量模块职责边界模糊，共用同一 Store 与后端状态机，存在严重耦合**（详见下节）。

---

## 10. 标定模块与计量模块的边界问题

### 10.1 现状：两模块高度耦合

当前计量模块（`MeasurementView.vue`）与标定模块（`CalibrationView.vue`）**并未在业务逻辑层清晰拆分**，而是共用同一套 `useCalibrationStore` 和后端 `calibration.Service`：

| 问题 | 说明 |
|------|------|
| **共用 Store** | 计量模块直接 `import { useCalibrationStore }`，没有独立的 `useMeasurementStore` |
| **语义污染** | 计量模块的「开始采集」按钮底层调用的是 `startCalibration()`，名称与业务不符 |
| **共用状态机** | 后端只有一个全局 session，计量与标定共用同一套 `SessionMachine` 状态迁移 |
| **视图层重复** | 两个页面的 sidebar、表格、操作按钮、SSE、轮询、报告模板选择等代码大量重复 |
| **专属逻辑混杂** | 计量模块需要 `generatePressurePoints`、`resetCollection`；标定模块需要 `fitData`、`endCalibration`——全部塞在同一个 store 中 |

### 10.2 具体表现

**前端层面**：
- `MeasurementView.vue` 直接调用标定 store 的 action：`startCalibration`、`pauseCalibration`、`resumeCalibration`、`stopCalibration`、`pressurize`、`confirmPressure`、`collectData`
- 两个视图都内联了几乎相同的 `tableData` computed、`getPointStatusType`、`getPointStatusText`、SSE 监听、轮询逻辑
- 计量模块的 `controlMode` / `pressureMode` 通过 `generatePressurePoints({ controlMode, pressureMode })` 传入，但标定模块的同名局部变量却未传入

**后端层面**：
- `calibration.Service` 同时承载「校准」和「计量采集」两套流程
- 计量模块不需要 `fitting` 步骤，但状态机中仍然存在；标定模块的 `fitting` 又可能被计量模块误触发
- 两个模块同时操作时，会话状态会互相覆盖

### 10.3 带来的风险

1. **维护困难**：修改计量逻辑时极易误伤标定逻辑
2. **测试困难**：无法单独测试计量模块的完整业务流程
3. **扩展困难**：后续若增加检定、核查模块，只能继续往 `calibration` store 里堆砌
4. **认知负担**：新成员看到计量模块调用 `startCalibration()`，会直接产生误解

### 10.4 改进建议

#### 短期（最小改动）

1. **重命名通用 action**：将 `startCalibration` / `pauseCalibration` / `resumeCalibration` / `stopCalibration` 改为 `startSession` / `pauseSession` / `resumeSession` / `stopSession`，去除 `Calibration` 语义污染。
2. **提取公共 composable**：将 `tableData`、状态映射、SSE 监听、轮询逻辑提取为 `useCalibrationCommon()`，供两视图共用。
3. **计量模块禁用标定专属 action**：确保计量视图不调用 `fitData`、`endCalibration`。

#### 长期（推荐）

1. **前端拆分 Store**：
   - 创建 `useMeasurementStore`，仅包含计量所需状态（如单位一致性检查、报警设置、采集进度）
   - 将设备连接、通道选择等通用逻辑下沉到 `useDeviceControlStore` 或独立 composable
2. **后端拆分 Service**：
   - 计量模块拥有独立的 `measurement.Service` 和会话实例
   - 标定模块保留 `calibration.Service`
   - 压力控制、设备驱动读写等底层能力下沉到公共基础设施层（如 `internal/infrastructure/pressure`）
3. **状态机实例化**：每个模块拥有独立的 `SessionMachine` 实例，避免状态互相覆盖
