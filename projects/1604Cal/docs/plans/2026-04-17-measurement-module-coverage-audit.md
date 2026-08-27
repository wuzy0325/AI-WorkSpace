# 计量模块业务流程落地核对（持续更新）

> 目标：对照参考模块 `1605MeassureApp`，逐项核对当前工程 `Cal1604` 的业务流程落地情况，覆盖 UI 流程与底层命令链路。
>
> 说明：本文件按“边查边写”方式持续追加，避免会话中断导致结果丢失。

---

## 0. 核对范围与证据路径

- 参考模块（旧）：`C:\Users\wuzhy\Documents\D\SVN\SoftWare\AI Engineering\Measurement\1604 Measurement\1605MeassureApp`
- 当前工程（新）：`C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\Ai Agent\1604Cal_go\Cal1604`
- 参考业务文档：`C:\Users\wuzhy\Documents\D\SVN\SoftWare\AI Engineering\Measurement\1604 Measurement\1605MeassureApp\docs\业务流程说明.md`

## 1. 过程日志（增量写入）

### 2026-04-17 第1轮

- 已确认参考模块与当前工程均可访问。
- 已定位参考业务总文档（含设备管理、自动采集、手动采集、报告导出、协议命令、状态机、错误处理）。
- 已定位当前工程核心目录：`internal/`（Go 后端域层与基础设施）、`web/src/`（Vue3 前端）。
- 下一步将先做“参考模块业务清单抽取”，再逐条映射到当前工程实现证据。

### 2026-04-17 第2轮（初步证据）

- 参考模块 UI 入口已定位：`src/renderer/views/` 下包含 `DeviceManagement.vue`、`DataCollection.vue`、`CalibrationConfig.vue`、`ReportExport.vue`、`PressureWorkbenchView.vue` 等页面。
- 参考模块底层能力已定位：`src/main/services/CollectionService.ts`、`src/main/services/ReportService.ts`、`src/main/device/services/*`、`src/main/device/adapters/*`。
- 参考模块命令语义已定位：压力侧 `SET_PRESSURE/GET_PRESSURE/SET_UNIT/GET_UNIT`，计量侧 `SET_VALVE_STATUS/GET_UNIT/SET_UNIT`，并包含 SCPI `*IDN?` 等命令策略。
- 在当前工程 `internal/**/*.go` 中按上述命令字全文检索，暂未发现同名命令字（结果为 0）；初判当前工程命令实现路径可能采用“二进制协议/抽象驱动”或尚未落地，需要继续深挖 `internal/infrastructure/driver` 与 `internal/workflow`。

### 2026-04-17 第3轮（覆盖矩阵 + 命令映射）

- 已完成“旧流程 -> 新实现”逐项对照矩阵，覆盖设备管理、单位检查、自动/手动采集、报警、报告导出、状态机与容错。
- 已补充 WTN1604 与 SCPI 系列（ConST811A/820/860/SPC4000）命令级证据映射。
- 已形成缺口分级与整改优先级（P0/P1/P2），可直接转化为开发任务。

### 2026-04-17 第4轮（P0-1 + P1-1 修复完成）

- 已完成后端自动采集闭环编排：
  - `internal/application/calibration/service.go` 新增 `RunAutoCollection`、`collectPoint`、`checkAlarm`、`ResolveAlarm`、`RetryPoint`、`PauseAutoCollection`、`ResumeAutoCollection`、`StopAutoCollection`。
  - `StartCalibration` 在 `ControlMode == "auto"` 时自动启动采集循环；`EndCalibration` 自动取消后台 goroutine。
  - 新增 API 路由：`POST /api/v1/calibration/resolve-alarm`、`POST /api/v1/calibration/retry-point`。
  - 会话 `pause/resume` 已联动自动采集的暂停与恢复。
- 已修复 `controlMode/pressureMode` 上传未生效问题：
  - `CalibrationConfig` 新增 `ControlMode`、`PressureMode` 字段。
  - `calibrationSetConfigHandler` 已将该两字段写入配置。
- 测试验证：`go test ./...`（除预存在问题的 `cal1604/test` 目录外）全部通过；`go vet` 无异常。
- 剩余待做：P0-2（报警服务深度接入多通道配置化阈值）、P1-2（单位一致性运行期校验）、P1-3（报告导出后端链路）、P2（批量连接 + SSE 消费优化）。

---

## 2. 参考业务基线（1605）

以下能力作为“业务对齐”基线：

- 设备管理：新增/编辑/连接/批量连接（`src/main/device/manager/DeviceManager.ts:318`）。
- 单位一致性：按设备当前单位检查并给出冲突结果（`src/main/device/manager/DeviceManager.ts:240`）。
- 自动采集闭环：`startCollection -> runAutoCollection -> collectPoint`（`src/main/services/CollectionService.ts:343`, `src/main/services/CollectionService.ts:379`, `src/main/services/CollectionService.ts:418`）。
- 手动采集闭环：`manualPressurize/manualCollect`（`src/main/services/CollectionService.ts:795`, `src/main/services/CollectionService.ts:817`）。
- 报警确认闭环：超限判定 + 用户确认继续/重试（`src/main/services/CollectionService.ts:618`, `src/main/services/CollectionService.ts:693`）。
- 报告导出闭环：模板 + 导出路径 + Excel 生成（`src/main/ipc/handlers/reportHandler.ts:50`, `src/main/services/ReportService.ts:170`）。

---

## 3. 业务流程覆盖矩阵（1605 -> Cal1604）

### 3.1 设备管理（新增/编辑/单设备连接）

- 旧系统证据：设备工厂 + 管理器 + 连接流程（`src/main/device/manager/DeviceManager.ts:120`, `src/main/device/manager/DeviceManager.ts:318`）。
- 新系统证据：`/api/v1/devices`、`/api/v1/devices/connect`、`/api/v1/devices/disconnect`（`internal/api/http/router.go:118`, `internal/api/http/router.go:120`, `internal/api/http/router.go:121`）；前端面板支持新增/编辑/连接切换（`web/src/components/DeviceManagementPanel.vue:631`, `web/src/components/DeviceManagementPanel.vue:657`）。
- 结论：✅ 已落地（单设备维度）。

### 3.2 设备批量连接

- 旧系统证据：`connectAll()` 并行连接（`src/main/device/manager/DeviceManager.ts:318`）。
- 新系统证据：当前路由与前端均未发现批量连接入口（`internal/api/http/router.go:118-123` 仅单设备；`web/src/components/DeviceManagementPanel.vue:657` 仅单卡切换）。
- 结论：❌ 缺失。

### 3.3 单位一致性检查（展示 + 业务门禁）

- 旧系统证据：单位一致性作为采集前条件（`src/renderer/views/DataCollection.vue:41`）+ 管理器检查逻辑（`src/main/device/manager/DeviceManager.ts:240`）。
- 新系统证据：
  - 已有检查接口：`/api/v1/checks/unit-consistency`（`internal/api/http/router.go:122`）。
  - 已有 UI 展示：计量页面单位检查块（`web/src/views/MeasurementView.vue:96`）。
  - 检查算法基于配置中的 `Device.Unit` 字段，而非运行期逐设备读取（`internal/device/manager/device_manager.go:97`, `internal/device/manager/persistent_device_manager.go:151`）。
  - 启动采集时未硬性阻断单位不一致（`web/src/stores/calibration/index.ts:132`, `web/src/stores/calibration/index.ts:340`）。
- 结论：⚠️ 部分落地（“看得到”，但“门禁 + 实时读取”未完全对齐）。

### 3.4 自动采集主循环（打压->稳压->采集->记录）

- 旧系统证据：后台服务自动循环驱动全流程（`src/main/services/CollectionService.ts:379`, `src/main/services/CollectionService.ts:418`）。
- 新系统证据：
  - ~~会话启动仅进入 `ready` + 阀门/设备初始化，不会自动跑完整测点循环（`internal/application/calibration/service.go:460`）。~~
  - ~~测点执行依赖前端逐点调用 `pressurize/collect`（`web/src/stores/calibration/index.ts:408`, `web/src/stores/calibration/index.ts:442`；对应接口 `internal/api/http/calibration_handler.go:197`, `internal/api/http/calibration_handler.go:216`）。~~
  - 已新增 `RunAutoCollection` 后台 goroutine 自动逐点执行 `pressurize -> stabilize -> collect -> alarm`（`internal/application/calibration/service.go:519`）。
  - `StartCalibration` 在 `ControlMode == "auto"` 时自动触发自动采集（`internal/application/calibration/service.go:492`）。
  - 新增 `resolve-alarm`、`retry-point` API（`internal/api/http/router.go:145`, `internal/api/http/router.go:146`）。
- 结论：✅ 已落地（基础闭环完成，报警阈值配置化可后续增强）。

### 3.5 手动模式（手动打压/手动采集）

- 旧系统证据：独立 IPC 能力 `MANUAL_PRESSURIZE`、`MANUAL_COLLECT`（`src/main/ipc/handlers/collectionHandler.ts:168`, `src/main/ipc/handlers/collectionHandler.ts:180`）。
- 新系统证据：
  - 前端存在“手动模式”UI 与 `handleManualCollect`（`web/src/views/MeasurementView.vue:783`, `web/src/views/MeasurementView.vue:911`）。
  - 但后端无独立 manual API；手动本质复用通用 `collect`，流程控制主要在前端（`internal/api/http/router.go:132-141` 无 manual 路由）。
- 结论：⚠️ 部分落地（UI 有，后端手动编排缺失）。

### 3.6 报警检测 + 确认/重试闭环

- 旧系统证据：报警判定、声响、确认、重试闭环（`src/main/services/CollectionService.ts:618`, `src/main/services/CollectionService.ts:683`, `src/main/services/CollectionService.ts:693`）。
- 新系统证据：
  - 前端报警开关标注为“本地 UI 状态”（`web/src/views/MeasurementView.vue:788`）。
  - 后端 `AlarmService` 已接入自动采集流程中的 `checkAlarm`（`internal/application/calibration/service.go:612`），支持阻塞等待用户决策。
  - 已新增 `POST /api/v1/calibration/resolve-alarm` 和 `POST /api/v1/calibration/retry-point`（`internal/api/http/router.go:145-146`）。
- 结论：✅ 已落地（基础闭环完成，多通道配置化阈值可后续增强）。

### 3.7 会话状态机一致性

- 旧系统证据：存在采集状态、暂停恢复、报警处理动作（`src/main/services/CollectionService.ts:733`, `src/main/services/CollectionService.ts:750`）。
- 新系统证据：
  - 状态机定义完整，包含 `await_manual_collect`、`await_alarm_resolution`（`internal/workflow/session_machine.go:35`, `internal/domain/session_state.go:17`）。
  - 但当前业务流中基本未触发这两个状态（仅定义，无业务调用证据）。
  - `session/start` 测试期望状态为 `ready`（`internal/api/http/session_handler_test.go:63`），体现启动阶段较轻。
- 结论：⚠️ 状态模型已备齐，编排落地不足。

### 3.8 报告导出

- 旧系统证据：`report/export` IPC + Excel 模板填充（`src/main/ipc/handlers/reportHandler.ts:50`, `src/main/services/ReportService.ts:170`）；前端有独立报告页（`src/renderer/router/index.ts:35`, `src/renderer/views/ReportExport.vue:64`）。
- 新系统证据：
  - 后端仅提供模板文件名选择（`internal/api/http/report_handler.go:16`, `internal/report/template_selector.go:5`）。
  - 前端导出为本地 CSV（`web/src/views/MeasurementView.vue:1005`, `web/src/views/CalibrationView.vue:638`）。
- 结论：⚠️ 部分落地（模板规则在，完整 Excel 导出链路缺失）。

### 3.9 控制模式 / 打压模式的后端生效

- 旧系统证据：`controlMode` 进入采集服务并影响流程（`src/main/services/CollectionService.ts:124`, `src/main/services/CollectionService.ts:369`）；`pressureMode=roundTrip` 影响测点生成（`src/main/services/CollectionService.ts:324`）。
- 新系统证据：
  - 前端会传 `controlMode/pressureMode`（`web/src/stores/calibration/index.ts:287`）。
  - ~~但后端 `CalibrationConfig` 未承载该字段（`internal/application/calibration/service.go:27`）；`calibrationSetConfig` 也未写入（`internal/api/http/calibration_handler.go:136`）。~~
  - 已扩展 `CalibrationConfig` 承载 `ControlMode` 与 `PressureMode`（`internal/application/calibration/service.go:29-30`）。
  - `calibrationSetConfigHandler` 已将该两字段持久化到服务配置（`internal/api/http/calibration_handler.go:141-142`）。
  - `StartCalibration` 已根据 `ControlMode` 决定是否自动启动采集循环（`internal/application/calibration/service.go:492`）。
  - `pressureMode=roundTrip` 的测点生成逻辑待补充（当前 `GeneratePressurePoints` 仅生成单程点）。
- 结论：✅ `controlMode` 已生效；⚠️ `pressureMode=roundTrip` 的测点生成待补充。

### 3.10 连接可靠性与错误可观测性

- 旧系统证据：SCPI 服务有命令互斥、模型策略/回退机制（`src/main/device/scpi/ScpiPressureDeviceService.ts:134`, `src/main/device/scpi/commands/ScpiCommandStrategy.ts:141`）。
- 新系统证据：
  - 连接层提供 timeout + retry + backoff（`internal/application/deviceconnect/service.go:24`, `internal/application/deviceconnect/service.go:150`, `internal/application/deviceconnect/service.go:246`）。
  - 失败快照写入 `lastErrorReason/lastErrorAt` 并通过 SSE 推送（`internal/application/deviceconnect/service.go:206`, `internal/api/http/device_handler.go:239`）。
- 结论：✅ 连接可靠性基础能力已落地。

---

## 4. 命令级映射（旧 -> 新）

### 4.1 WTN1604（计量侧）

| 语义 | 旧系统证据 | 新系统证据 | 结论 |
|---|---|---|---|
| 读阀门状态 | `@01  0`（`src/main/device/adapters/WTN1604ProtocolAdapter.ts:271`） | `ReadValveStatus` 发送 `@01  0`（`internal/infrastructure/driver/tcp_connection_driver.go:193`） | ✅ |
| 写阀门状态 | `w0C01/w0C00`（`src/main/device/adapters/WTN1604ProtocolAdapter.ts:301`） | `SetValveStatus` 发送 `w0C01/w0C00`（`internal/infrastructure/driver/tcp_connection_driver.go:224`） | ✅ |
| 读单位 | `u01101`（`src/main/device/adapters/WTN1604ProtocolAdapter.ts:329`） | `ReadUnit` 发送 `u01101`（`internal/infrastructure/driver/tcp_connection_driver.go:244`） | ✅ |
| 写单位 | `v01101 <coef>`（`src/main/device/adapters/WTN1604ProtocolAdapter.ts:355`） | `SetUnit` 发送 `v01101 <coef>`（`internal/infrastructure/driver/tcp_connection_driver.go:261`） | ✅ |
| 读通道数据 | `r<bitmap>0`（`src/main/device/adapters/WTN1604ProtocolAdapter.ts:221`） | `CollectData` 发送 `r<bitmap>0`（`internal/infrastructure/driver/tcp_connection_driver.go:274`） | ✅ |
| 模块信息 | `q00/q01`（`src/main/device/services/MeasureDeviceService.ts:410`） | `ReadDeviceInfo` 发送 `q00/q01`（`internal/infrastructure/driver/tcp_connection_driver.go:304`） | ✅ |

### 4.2 SCPI 家族（打压侧）

| 设备族 | 旧系统命令证据 | 新系统命令证据 | 结论 |
|---|---|---|---|
| ConST811A / 通用 SCPI | `PRESsure:TARGet`、`PRESsure0?`、`PRESsure:MODule1:STABle?`、`PRESsure:MODE`（`src/main/device/scpi/ScpiCommands.ts:106`, `src/main/device/scpi/ScpiCommands.ts:107`, `src/main/device/scpi/ScpiCommands.ts:125`） | 对应命令均已实现（`internal/infrastructure/driver/tcp_connection_driver.go:418`, `internal/infrastructure/driver/tcp_connection_driver.go:429`, `internal/infrastructure/driver/tcp_connection_driver.go:454`, `internal/infrastructure/driver/tcp_connection_driver.go:500`） | ✅ |
| ConST820 | `MEASure:SCALar:PRESsure1?`、`SOURce:PRESsure`、`OUTPut:PRESsure:MODE`、`UNIT:PRESsure`（`src/main/device/scpi/commands/strategies/ConST820Strategy.ts:166`, `src/main/device/scpi/commands/strategies/ConST820Strategy.ts:203`） | 对应命令均已实现（`internal/infrastructure/driver/tcp_connection_driver.go:542`, `internal/infrastructure/driver/tcp_connection_driver.go:568`, `internal/infrastructure/driver/tcp_connection_driver.go:552`, `internal/infrastructure/driver/tcp_connection_driver.go:597`） | ✅ |
| ConST860 | `PRESsure?`、`PRESsure:STABle?`、`PRESsure:MODule:CONTrol`、`PRESsure:MODule:UNIT`（`src/main/device/scpi/commands/strategies/ConST860Strategy.ts:225`, `src/main/device/scpi/commands/strategies/ConST860Strategy.ts:245`, `src/main/device/scpi/commands/strategies/ConST860Strategy.ts:305`） | 对应命令均已实现（`internal/infrastructure/driver/tcp_connection_driver.go:682`, `internal/infrastructure/driver/tcp_connection_driver.go:720`, `internal/infrastructure/driver/tcp_connection_driver.go:728`, `internal/infrastructure/driver/tcp_connection_driver.go:711`） | ✅ |
| SPC4000 | `GP/GN`、`RP`、`Measure/Vent`、`Units/Units?`（`src/main/device/scpi/commands/strategies/SPC4000Strategy.ts:118`, `src/main/device/scpi/commands/strategies/SPC4000Strategy.ts:110`, `src/main/device/scpi/commands/strategies/SPC4000Strategy.ts:132`, `src/main/device/scpi/commands/strategies/SPC4000Strategy.ts:88`） | 对应命令均已实现（`internal/infrastructure/driver/tcp_connection_driver.go:761`, `internal/infrastructure/driver/tcp_connection_driver.go:789`, `internal/infrastructure/driver/tcp_connection_driver.go:773`, `internal/infrastructure/driver/tcp_connection_driver.go:809`） | ✅ |

补充观察：旧系统的 `*IDN?` 识别命令有明确定义（`src/main/device/scpi/ScpiCommands.ts:54`），新系统当前连接阶段主要通过业务命令探测可用性，未见显式 `*IDN?` 握手路径。

---

## 5. 结论汇总

- ✅ 已对齐：设备单机管理、连接可靠性重试、核心协议命令集（WTN1604 + SCPI 多型号）、模板命名规则。
- ⚠️ 部分对齐：单位检查（展示有，门禁弱）、手动模式（UI 有，后端编排弱）、状态机（定义全，运行态触发少）、报告（仅模板+CSV）。
- ❌ 关键缺口：~~自动采集后端闭环~~（已修复）、~~报警确认重试闭环~~（已修复基础版）、~~`controlMode` 端到端生效~~（已修复）、`pressureMode=roundTrip` 测点生成、批量连接能力、完整 Excel 导出链路。

---

## 6. 修复优先级建议（可转任务）

### P0（先做）

1. ~~建立后端自动采集编排：`start` 后自动执行逐点 `pressurize -> stabilize -> collect -> alarm`，前端只做展示/控制。~~ ✅ 已完成（`2026-04-17`）。
2. ~~报警闭环接入：将 `internal/workflow/alarm_service.go` 接入采集流程，新增确认/重试 API（等价旧 `RESOLVE_ALARM`）。~~ ✅ 已完成基础版（`2026-04-17`）。

### P1（随后）

1. ~~`controlMode/pressureMode` 端到端生效：扩展 `CalibrationConfig`，让 `return` 模式真实生成回程点，`manual` 模式真实驱动状态机。~~ `controlMode` ✅ 已完成（`2026-04-17`）；`pressureMode=roundTrip` 测点生成待补充。
2. 单位一致性改为“运行期真实值校验”：优先读取已连接驱动单位，不仅依赖配置快照。
3. 补齐报告导出后端链路：会话数据 -> 模板填充 -> 文件输出（支持进度事件）。

### P2（收尾）

1. 增加批量连接/断开 API + 前端批量操作入口。
2. 梳理 SSE 事件消费：把 `pressure.applied/data.collected` 等事件接入实际页面组件，减少轮询依赖。
