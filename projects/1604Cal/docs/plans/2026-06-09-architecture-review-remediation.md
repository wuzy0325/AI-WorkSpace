# 架构 Review 修复计划

> **版本**: 1.0 | **日期**: 2026-06-09 | **基于**: 2026-06-09 架构 Review

---

## 一、背景与目标

上一轮架构深化（`docs/plans/architecture-deepening.md`）已完成双状态机统一、驱动引用消除、事件注册表、算法收敛等 6 项整改，项目后端的 `domain → device → application → infrastructure → api` 分层方向明确。

本次 Review 聚焦**当前仍然存在的架构摩擦点**，目标是在不改动业务流程的前提下，为后续迭代建立更可靠的 seam 和更高的模块深度。

---

## 二、问题清单（按风险排序）

### 🔴 高风险

| # | 问题 | 现状 | 影响 |
|---|------|------|------|
| **R1** | 设备会话全局单例，标定/计量/多打压隐式共享状态 | `session.Service` 同一时间只能绑定一组设备；`calibration` 和 `measurement` 共享同一 `sessionService` 和驱动 | 模块间状态互相踩踏，`boundBy` 是裸字符串无法保证租约安全 |
| **R2** | `apiServer` 是装配中心 + 服务定位器，模块依赖被隐藏在构造函数中 | `internal/api/http/device_handler.go:35` 的 `apiServer` 持有所有业务服务的直接引用，`router.go:117` 负责创建全部依赖 | 测试时只能集成测试，无法独立 mock 单 handler；新增模块必须修改 `newRouterWithServer` |

### 🟡 中风险

| # | 问题 | 现状 | 影响 |
|---|------|------|------|
| **R3** | 共享状态机过宽，标定和计量的状态语义强制共用一张迁移表 | `SessionMachine` 的迁移表同时包含 `point_done`、`fitting`、`await_manual_collect` 等跨流程状态 | 调用方暴露过多无关状态；新增状态时必须评估对两个模块的影响 |
| **R4** | 前端 Store 混入 UI 副作用，测试面和业务面未分离 | `measurement/index.ts`（530 行）和 `calibration/index.ts`（377 行）直接调用 `ElMessage` | Store 无法在非 Element Plus 环境测试；UI 文案分散难以统一 |
| **R5** | SSE 与轮询有三套入口，事件生命周期分散 | `main.ts` 全局 EventSource、`useCalibrationSync` SSE + 3 timer、`useMeasurementSync` SSE + 2 timer | 同时存在 3+ EventSource 连接和 5+ timer；重连/错误处理策略不一致 |

### 🟢 中低风险

| # | 问题 | 现状 | 影响 |
|---|------|------|------|
| **R6** | 配置契约偏字符串和弱类型 | `WorkflowConfig.ControlMode`/`PressureMode` 是 `string`；前端各处重复定义 `'auto' | 'manual'` | 新增模式时前端/后端容易漂移；编译期无法检查 |

---

## 三、修复方案

### 方案 R1：设备会话绑定租约化

**目标**：将 `session.Service` 从全局单例升级为"绑定时发行 token，操作时出示 token"的租约模型。

**整改后行为**：
- `BindDevices` / `BindMeasureDevice` 返回 `bindingToken`（带过期时间的结构体或 context）
- `ReadPressure` / `ReadMeasureData` 等数据读取方法接受 `bindingToken`
- 不同模块持有独立 token，超出 token 生命周期的操作直接返回 `ErrBindingExpired`

**涉及文件**：
```
internal/application/session/service.go
internal/application/calibration/service.go
internal/application/measurement/service.go
```

**预估工作量**：3 人日（含测试更新）

---

### 方案 R2：抽取依赖装配模块，解耦 apiServer

**目标**：将路由构造中的"创建所有服务"逻辑移到独立装配模块，`apiServer` 变成纯 handler holder，router 只绑定路径。

**整改后结构**：
```
internal/api/http/
├── router.go              # 路由绑定（仅 mux.HandleFunc）
├── handler_server.go      # apiServer struct（仅持有服务引用 + handler 方法）
├── dependencies/
│   └── assemble.go        # NewDependencies() 创建并连接所有服务
```

**装配模块职责**：
1. 创建 `SessionMachine`、`driver.Factory`
2. 创建 `deviceconnect.Service`、`multipress.Service`
3. 创建 `session.Service`（带驱动提供者链）
4. 创建 `calibration.Service`、`measurement.Service`
5. 注入配置和门禁
6. 返回一个 `*Dependencies` 结构体，包含所有可供 handler 使用的服务

**安全约束**：
- `Dependencies` 只暴露 handler 需要的方法，不暴露内部创建细节
- 每个 handler 通过 `func(apiServer)` 方法访问服务，不直接引用 `Dependencies`

**涉及文件**：
```
internal/api/http/router.go
internal/api/http/device_handler.go
新增: internal/api/http/dependencies/assemble.go
```

**预估工作量**：2 人日（含所有 handler 测试适配）

---

### 方案 R3：状态机封装窄接口

**目标**：底层仍共享 `SessionMachine`，但为不同业务模块提供窄接口，避免暴露无关状态。

**整改后结构**：

```
internal/workflow/
├── session_machine.go       # 底层引擎（保持现有迁移表不变）
├── calibration_state.go     # CalibrationStateMachine — 暴露标定语义接口
│   - Start() → ready
│   - Pressurize() → pressurizing
│   - Stabilize() → stabilizing
│   - Collect() → collecting
│   - Fit() → fitting
│   - Complete() → completed
├── measurement_state.go     # MeasurementStateMachine — 暴露计量语义接口
│   - Start() → collecting
│   - Pause() → paused
│   - Resume() → collecting
│   - Complete() → completed
```

**窄接口不暴露的状态**：
- 标定侧不需要 `await_manual_collect`
- 计量侧不需要 `point_done`、`fitting`

**实现方式**：
- `CalibrationStateMachine` / `MeasurementStateMachine` 内部持有 `*SessionMachine`
- 每个业务方法在调用 `Transition` 前做前置校验
- 对外只返回对应业务域的错误

**涉及文件**：
```
新增: internal/workflow/calibration_state.go
新增: internal/workflow/measurement_state.go
修改: internal/application/calibration/service.go
修改: internal/application/measurement/service.go
```

**预估工作量**：2 人日

---

### 方案 R4：Store 剥离 UI 副作用

**目标**：Pinia Store 变成纯状态 + 业务动作，UI 提示上移到 composable 或 view。

**整改原则**：
- Store actions 返回值对象 `{ ok: boolean; error?: string }` 替代 `ElMessage` 直接调用
- Composable / View 层根据返回值决定展示什么 UI 文案
- 同一业务模块的所有中文文案收敛到常量文件

**整改后结构**：

```
web/src/stores/calibration/
├── index.ts                 # 纯状态 + 业务动作（无 ElMessage 引用）
├── types.ts
├── pressurePoints.ts
├── deviceControl.ts
└── messages.ts              # 新增：本模块所有中文文案常量

web/src/composables/
├── useCalibrationUI.ts      # 新增：消费 store 返回值，展示 ElMessage / ElMessageBox
└── ...
```

**典型改动示例**（calibration store）：

```typescript
// 改前
async startCalibration() {
  if (!canStart.value) {
    ElMessage.warning('请先连接设备并选择通道')
    return
  }
  try {
    await triggerSessionAction('start')
    ElMessage.success('标定已开始')
  } catch (e) {
    ElMessage.error(`开始标定失败: ${e}`)
  }
}

// 改后
async startCalibration(): Promise<ActionResult> {
  if (!canStart.value) return { ok: false, error: 'MISSING_DEVICE_OR_CHANNEL' }
  try {
    await triggerSessionAction('start')
    return { ok: true }
  } catch (e) {
    return { ok: false, error: 'START_FAILED', detail: String(e) }
  }
}
```

**涉及文件**：
```
web/src/stores/calibration/index.ts
web/src/stores/calibration/deviceControl.ts
web/src/stores/calibration/pressurePoints.ts
web/src/stores/measurement/index.ts
web/src/stores/measurement/deviceStore.ts
web/src/stores/device/inventoryStore.ts
新增: web/src/stores/calibration/messages.ts
新增: web/src/stores/measurement/messages.ts
新增: web/src/composables/useCalibrationUI.ts
新增: web/src/composables/useMeasurementUI.ts
```

**预估工作量**：3 人日

---

### 方案 R5：SSE/轮询生命周期收敛为事件 Hub

**目标**：全局只创建一条 EventSource 连接，一个轮询调度器，各模块订阅关心的事件类型。

**整改后结构**：

```
web/src/composables/
├── useEventHub.ts           # 新增：全局 SSE + 轮询管理中心
│   - 单 EventSource 连接，onMessage 分发事件
│   - 统一重连策略（指数退避，max 30s）
│   - 统一轮询调度（按模块注册 poll 任务，按 interval 调度）
│   - 提供 subscribe(type, handler) → unsubscribe
│
├── useCalibrationSync.ts    # 简化：仅 subscribe + 本地状态管理
├── useMeasurementSync.ts    # 简化：同上
```

**内部设计**：

```typescript
export function useEventHub() {
  const subscribers = new Map<string, Set<(data: unknown) => void>>()

  function subscribe(type: string, handler: (data: unknown) => void): () => void { /* ... */ }
  function registerPoll(task: () => Promise<void>, intervalMs: number): () => void { /* ... */ }
}
```

**main.ts 改动**：
- 移除独立的 `createEventStream` 调用
- `useEventHub` 在 `useCalibrationSync` / `useMeasurementSync` 首次调用时惰性初始化
- 全局硬件日志通过 `subscribe(EVENT_HARDWARE_COMMAND, ...)` 替代单独的 EventSource

**安全约束**：
- `useEventHub` 内部使用 `ref` 计数，最后一个 `useCalibrationSync` 或 `useMeasurementSync` 卸载时自动断开 SSE 和停止所有 poll
- 重连时自动触发所有 `subscribe` 重置

**涉及文件**：
```
新增: web/src/composables/useEventHub.ts
修改: web/src/composables/useCalibrationSync.ts
修改: web/src/composables/useMeasurementSync.ts
修改: web/src/main.ts
```

**预估工作量**：2 人日

---

### 方案 R6：配置契约类型强化

**目标**：Go domain 和 TS types 同步定义枚举/常量，消除字符串漂移。

**整改后结构**：

```go
// internal/domain/workflow_config.go
type ControlMode string

const (
  ControlModeAuto   ControlMode = "auto"
  ControlModeManual ControlMode = "manual"
)

type PressureMode string

const (
  PressureModeSingle    PressureMode = "single"
  PressureModeRoundTrip PressureMode = "roundTrip"
)
```

```typescript
// web/src/types/calibration.ts
export const ControlMode = { Auto: 'auto', Manual: 'manual' } as const
export type ControlMode = (typeof ControlMode)[keyof typeof ControlMode]

export const PressureMode = { Single: 'single', RoundTrip: 'roundTrip' } as const
export type PressureMode = (typeof PressureMode)[keyof typeof PressureMode]
```

**涉及文件**：
```
internal/domain/workflow_config.go
internal/application/calibration/service.go
internal/application/measurement/service.go
internal/api/http/*handler.go
web/src/types/calibration.ts
web/src/stores/calibration/
web/src/stores/measurement/
```

**预估工作量**：1 人日

---

## 四、执行顺序（按依赖和风险排列）

### 第一批：低风险、可独立交付的收敛（R6 → R4）

| 顺序 | 方案 | 理由 |
|------|------|------|
| 1 | **R6 — 配置契约类型强化** | 纯类型变更，编译期验证，前后端可并行，无运行时风险 |
| 2 | **R4 — Store 剥离 UI 副作用** | 只改前端，不影响后端；测试可同步更新；改动虽多但按模块拆分 |

### 第二批：前端架构加固（R5）

| 顺序 | 方案 | 理由 |
|------|------|------|
| 3 | **R5 — SSE/轮询收敛为事件 Hub** | 依赖 R4 完成后的 Store 状态稳定；纯前端改动 |

### 第三批：后端架构加固（R2 → R3 → R1）

| 顺序 | 方案 | 理由 |
|------|------|------|
| 4 | **R2 — 抽取依赖装配模块** | 先建立清晰的依赖注入点，为 R3 和 R1 铺路 |
| 5 | **R3 — 状态机封装窄接口** | 在 R2 保证测试隔离后，安全拆分状态机语义 |
| 6 | **R1 — 设备会话绑定租约化** | 依赖 R2（装配模块）和 R3（状态机窄接口）完成，避免租约 token 直接耦合到过宽的接口上 |

---

## 五、质量门禁（每批完成后执行）

### 后端

- `go test ./...` 全部通过
- `go vet ./...` 无告警
- 新增/修改的函数有测试覆盖

### 前端

- `npm run typecheck` 无错误
- `npm run lint` 无告警
- 关键 Store 和 composable 的单元测试通过

### 集成

- 标定流程（自动模式 + 手动模式）端到端可用
- 计量流程（自动模式 + 手动模式）端到端可用
- 设备管理（添加/连接/断开/删除）功能正常
- SSE 事件推送和轮询数据刷新正常

---

## 六、决策记录

### 本次 Review 确认的事项

1. **不做 `application` 重命名为 `usecase`** — 收益不足以抵消导入路径变更代价
2. **不做 `SessionMachine` 完全拆分** — 底层引擎保留一个，通过窄接口（R3）解耦语义
3. **不做 event bus 替换为外部 MQ** — 进程内总线对桌面单机场景已经足够
4. **不做前端路由级代码分割** — Wails webview 是本地加载，网络延迟不是瓶颈

### 暂缓事项

| 事项 | 暂缓原因 |
|------|----------|
| `test/` 目录整理 | 测试文件数量少，等批量增加测试时一并整理 |
| 根目录构建产物清理 | 已有 `.gitignore` 覆盖，但需确认是否完全生效（`nul`、`*.exe` 等） |
| `DeviceManagementPanel.vue`（1233 行）拆分 | 功能稳定，当前无修改需求；需拆分时再启动 |
| `RealtimeDataPanel.vue`（874 行）拆分 | 同上 |

---

## 七、附录：当前文件架构总览

```
.
├── main.go                          # Wails 入口
├── app.go                           # App struct（启动/关闭/端口/导出）
├── cmd/server/main.go               # 独立 HTTP 模式入口（E2E 用）
├── internal/
│   ├── domain/                      # 领域对象（无行为，纯数据 + 常量）
│   │   ├── device.go
│   │   ├── session_state.go
│   │   ├── workflow_config.go
│   │   ├── workflow_session.go
│   │   ├── pressure_point.go
│   │   └── alarm.go
│   ├── device/                      # 设备接口 + 管理器
│   │   ├── interfaces.go            # PressureDriver / MeasureDriver / ConnectionDriver 等核心接口
│   │   ├── interfaces_test.go
│   │   └── manager/
│   │       ├── device_manager.go
│   │       ├── device_manager_test.go
│   │       └── persistent_device_manager.go
│   ├── infrastructure/
│   │   └── driver/                  # 设备驱动适配器
│   │       ├── factory.go           # Factory（按型号创建驱动）
│   │       ├── tcp_base.go          # TCP 连接基类
│   │       ├── wtn1604_driver.go    # WTN1604 计量设备
│   │       ├── const_base_driver.go # ConST 系列基类
│   │       ├── const811a_driver.go  # ConST 811A
│   │       ├── const820_driver.go   # ConST 820
│   │       ├── const860_driver.go   # ConST 860
│   │       ├── spc4000_driver.go    # SPC4000
│   │       ├── simulated_*.go       # 模拟设备
│   │       ├── circuit_breaker.go   # 熔断器
│   │       └── helpers.go
│   ├── application/                 # 业务编排
│   │   ├── calibration/
│   │   │   ├── service.go           # 标定流程编排（757 行）
│   │   │   ├── collector.go         # 数据采集
│   │   │   ├── pressure.go          # 压力点管理
│   │   │   └── service_test.go
│   │   ├── measurement/
│   │   │   ├── service.go           # 计量流程编排（507 行）
│   │   │   ├── collector.go         # 数据采集
│   │   │   ├── points.go
│   │   │   ├── alarm.go
│   │   │   ├── workflow.go
│   │   │   ├── session_store.go
│   │   │   └── service_test.go
│   │   ├── session/
│   │   │   ├── service.go           # 设备会话服务（328 行）
│   │   │   ├── driver_resolver.go
│   │   │   └── service_test.go
│   │   ├── multipress/
│   │   │   ├── service.go           # 多设备打压控制（546 行）
│   │   │   └── service_test.go
│   │   └── deviceconnect/
│   │       ├── service.go           # 设备连接服务
│   │       └── service_test.go
│   ├── api/
│   │   ├── http/
│   │   │   ├── router.go            # 依赖装配 + 路由绑定（350 行）
│   │   │   ├── device_handler.go    # apiServer 定义 + 设备 handler
│   │   │   ├── *handler.go          # 各业务 handler
│   │   │   ├── *handler_test.go
│   │   │   ├── response_writer.go
│   │   │   └── error_mapper.go
│   │   └── dto/
│   │       └── response.go
│   ├── workflow/                    # 状态机 + 稳定性 + 报警
│   │   ├── session_machine.go       # 共享状态机（160 行）
│   │   ├── stability_service.go
│   │   └── alarm_service.go
│   ├── events/                      # 事件总线 + 事件类型常量
│   │   ├── bus.go
│   │   └── event_types.go
│   ├── report/                      # 报告生成
│   │   ├── report_service.go
│   │   ├── template_selector.go
│   │   └── excel_generator.go
│   ├── config/                      # 应用配置
│   │   └── app_config.go
│   └── errors/                      # 错误码
│       └── codes.go
├── web/
│   └── src/
│       ├── api/                     # API 层（按业务模块拆分）
│       │   ├── client.ts
│       │   ├── calibration.ts
│       │   ├── measurement.ts
│       │   ├── multipress.ts
│       │   ├── session.ts
│       │   └── device.ts
│       ├── stores/                  # Pinia Store
│       │   ├── calibration/         # 标定 Store（已拆为 3 文件 + composable）
│       │   ├── measurement/         # 计量 Store（530 行，待拆）
│       │   ├── multipress/
│       │   └── device/
│       ├── composables/             # 可组合逻辑
│       │   ├── useCalibrationSync.ts
│       │   ├── useMeasurementSync.ts
│       │   ├── useCalibrationFlow.ts
│       │   ├── useCalibrationConfig.ts
│       │   ├── useMultiPressSync.ts
│       │   ├── useFileSaveDialog.ts
│       │   └── useConfigPersistence.ts
│       ├── components/              # Vue 组件
│       │   ├── calibration/
│       │   ├── measurement/
│       │   ├── device/
│       │   └── common/
│       ├── views/                   # 页面视图
│       │   ├── CalibrationView.vue
│       │   ├── MeasurementView.vue
│       │   ├── ModuleHubView.vue
│       │   └── ...
│       ├── types/                   # TypeScript 类型
│       └── shared/                  # 共享常量（事件类型）
└── docs/
    └── plans/                       # 设计/计划文档
```
