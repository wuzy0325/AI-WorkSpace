# Cal1604 标定系统 — 具体优化实施计划

> **版本**: 1.0 | **日期**: 2026-04-17 | **基于**: 业务逻辑现状总结与改善建议.md + calibration-module-logic.md + 项目代码结构分析

---

## 目录

1. [优化总览与优先级矩阵](#1-优化总览与优先级矩阵)
2. [Phase 1: 架构安全与开发基础 (P0)](#2-phase-1-架构安全与开发基础-p0)
3. [Phase 2: 模块解耦与代码质量 (P1)](#3-phase-2-模块解耦与代码质量-p1)
4. [Phase 3: 功能完善与工程化 (P2)](#4-phase-3-功能完善与工程化-p2)
5. [跨Phase依赖关系](#5-跨phase依赖关系)
6. [验收标准总表](#6-验收标准总表)

---

## 1. 优化总览与优先级矩阵

### 1.1 问题来源交叉分析

两份文档共识别出 **15 类问题**，按影响范围和紧迫程度分为三个 Phase：

| # | 问题 | 来源文档 | 影响等级 | Phase |
|---|------|----------|----------|-------|
| 1 | 缺少断路器 (CircuitBreaker) | 现状总结 §4.1(1) | **严重** — 故障设备拖垮全系统 | Phase 1 |
| 2 | 缺少设备模拟层 (Mock) | 现状总结 §4.1(2) | **严重** — 无法脱离硬件开发/测试 | Phase 1 |
| 3 | 标定/计量模块高度耦合 | calibration-logic §10 | **严重** — 维护/扩展/测试均受阻 | Phase 1 |
| 4 | 缺少前端 Composables 层 | 现状总结 §4.1(3) | **严重** — 逻辑分散、复用性差 | Phase 1 |
| 5 | 设备驱动单文件实现 | 现状总结 §4.2(4) | **中等** — 可维护性差 | Phase 2 |
| 6 | 缺少独立 RetryStrategy | 现状总结 §4.2(5) | **中等** — 重试不可配置 | Phase 2 |
| 7 | 报警机制不完整 | 现状总结 §4.2(6) | **中等** — 缺声音/确认/通道级控制 | Phase 2 |
| 8 | 报告导出功能不完整 | 现状总结 §4.2(7) | **中等** — 缺Excel生成/进度/路径选择 | Phase 2 |
| 9 | controlMode/pressureMode 参数断层 | calibration-logic §7.2 | **中等** — 前端有UI但未传参 | Phase 2 |
| 10 | PressurePointList.vue 状态不一致 | calibration-logic §7.1 | **轻微** — 废弃组件待清理 | Phase 2 |
| 11 | 报告模板选择未接入生成流程 | calibration-logic §7.3 | **轻微** — 选择后无后续逻辑 | Phase 2 |
| 12 | 前后端类型不同步 | 现状总结 §4.2(8) | **轻微** — 手动对齐有遗漏风险 | Phase 3 |
| 13 | 缺少共享类型/常量目录 | 现状总结 §4.3(9) | **轻微** | Phase 3 |
| 14 | 配置服务不独立 | 现状总结 §4.3(10) | **轻微** | Phase 3 |
| 15 | 缺少输入验证器/日志工具 | 现状总结 §4.3(11)(12) | **轻微** | Phase 3 |

### 1.2 Phase 定义

| Phase | 定位 | 目标 |
|-------|------|------|
| **Phase 1** | 架构安全与开发基础 | 消除严重风险，建立开发/测试基础设施，解耦核心模块 |
| **Phase 2** | 模块解耦与代码质量 | 拆分驱动、完善报警/报告、修复参数断层、清理废弃代码 |
| **Phase 3** | 功能完善与工程化 | 类型同步、配置服务、验证器、日志等工程化提升 |

---

## 2. Phase 1: 架构安全与开发基础 (P0)

### 2.1 [P0-1] 引入断路器模式

**问题**：设备持续故障时，每次请求都执行完整重试，导致系统资源浪费、长时间等待、级联故障。

**目标**：防止设备故障时系统资源耗尽和级联失败。

#### 2.1.1 新增文件

```
internal/resilience/
├── circuit_breaker.go         # 断路器核心实现
├── circuit_breaker_test.go    # 断路器单元测试
├── retry_strategy.go          # 独立重试策略（合并 P1-5 到此处）
├── retry_strategy_test.go     # 重试策略单元测试
└── config.go                  # 弹性配置定义
```

#### 2.1.2 断路器实现规格

```go
// internal/resilience/circuit_breaker.go

type State int
const (
    Closed   State = iota  // 正常：请求放行
    Open                    // 熔断：请求直接拒绝
    HalfOpen                // 试探：放行有限请求探测恢复
)

type CircuitBreaker struct {
    name              string
    state             State
    failureCount      int
    successCount      int
    failureThreshold  int           // 触发熔断的连续失败次数（默认 5）
    successThreshold  int           // 半开→关闭的连续成功次数（默认 3）
    resetTimeout      time.Duration // 熔断超时时间（默认 30s）
    lastFailureTime   time.Time
    mu                sync.RWMutex
    onStateChange     func(from, to State)  // 状态变更回调（用于事件推送）
}

// Execute 包装操作，根据断路器状态决定放行/拒绝/试探
func (cb *CircuitBreaker) Execute(ctx context.Context, op func() error) error
```

**状态机**：
```
CLOSED ──failureCount >= threshold──> OPEN
OPEN ──resetTimeout elapsed──> HALF_OPEN
HALF_OPEN ──op success──> successCount++ ──successCount >= successThreshold──> CLOSED
HALF_OPEN ──op failure──> OPEN (重置计时器)
```

#### 2.1.3 重试策略实现规格

```go
// internal/resilience/retry_strategy.go

type RetryConfig struct {
    MaxRetries        int           // 最大重试次数
    InitialBackoff    time.Duration // 初始退避时间
    MaxBackoff        time.Duration // 最大退避时间
    BackoffMultiplier float64      // 退避倍数（默认 2.0）
    RetryableErrors   []string     // 可重试的错误码列表
}

// RetryWithConfig 执行带配置的重试
func RetryWithConfig(ctx context.Context, cfg RetryConfig, op func() error) error
```

**预置配置**：

| 场景 | MaxRetries | InitialBackoff | MaxBackoff | BackoffMultiplier |
|------|-----------|---------------|------------|-------------------|
| 设备连接 | 3 | 80ms | 300ms | 2.0 |
| 设备断开 | 2 | 40ms | 120ms | 2.0 |
| 数据采集 | 2 | 500ms | 2000ms | 2.0 |
| SCPI命令 | 3 | 200ms | 1000ms | 2.0 |

#### 2.1.4 集成点

**修改文件**：

| 文件 | 修改内容 |
|------|----------|
| `internal/application/deviceconnect/service.go` | 用 `CircuitBreaker.Execute()` 包装设备连接/断开操作；用 `RetryWithConfig()` 替换内联 `retryWithBackoff` |
| `internal/application/calibration/service.go` | 在 `Pressurize`、`Collect`、`ReadCurrentPressure` 等设备IO操作中包装断路器 |
| `internal/infrastructure/driver/tcp_connection_driver.go` | 各驱动的 `Connect`/`Disconnect` 方法外层包装断路器 |
| `internal/device/manager/device_manager.go` | `DeviceManager` 为每个设备实例维护独立的 `CircuitBreaker`，通过 map 管理 |

**集成方式**：
```go
// device_manager.go 中
type DeviceManager struct {
    // ...
    circuitBreakers map[string]*resilience.CircuitBreaker  // key: deviceID
}

func (dm *DeviceManager) getOrCreateBreaker(deviceID string) *resilience.CircuitBreaker {
    // 懒创建，每个设备独立断路器
}
```

#### 2.1.5 事件推送

断路器状态变更时通过 `events.Bus` 推送事件，前端可展示设备熔断状态：

```go
cb, _ := resilience.NewCircuitBreaker("device-811a", resilience.Config{
    OnStateChange: func(from, to resilience.State) {
        events.Bus.Publish(events.Event{
            Type: "circuit_breaker.state_changed",
            Data: map[string]interface{}{"device": "device-811a", "from": from, "to": to},
        })
    },
})
```

#### 2.1.6 验收标准

- [ ] `CircuitBreaker` 单元测试覆盖 CLOSED→OPEN→HALF_OPEN→CLOSED 全状态迁移
- [ ] `RetryWithConfig` 单元测试覆盖重试成功、重试耗尽、可重试错误过滤
- [ ] `DeviceManager` 中每个设备有独立断路器实例
- [ ] 设备连续故障 5 次后断路器熔断，后续请求立即返回错误（不执行TCP连接）
- [ ] 断路器状态变更事件通过 SSE 推送到前端
- [ ] 原有 `retryWithBackoff` 函数被 `RetryWithConfig` 替换，无残留调用

---

### 2.2 [P0-2] 建立设备模拟层

**问题**：无独立设备模拟器，开发/测试依赖真实硬件，前端无法独立运行。

**目标**：支持无硬件开发调试、前端独立运行、自动化测试。

#### 2.2.1 新增文件

```
internal/infrastructure/driver/mock/
├── mock_pressure_driver.go    # 打压设备模拟驱动
├── mock_measure_driver.go     # 计量设备模拟驱动
├── mock_wtn1604_driver.go     # WTN1604 模拟驱动
├── config.go                  # 模拟配置
└── config_test.go             # 配置测试
```

#### 2.2.2 模拟配置规格

```go
// internal/infrastructure/driver/mock/config.go

type MockConfig struct {
    Enabled           bool          // 是否启用模拟
    ConnectDelay      time.Duration // 连接延迟（默认 500ms）
    CommandDelay      time.Duration // 命令响应延迟（默认 100ms）
    PressureJitter    float64       // 压力波动幅度（默认 ±0.05，即目标值 ±5%）
    ChannelNoise      float64       // 通道噪声幅度（默认 ±0.01）
    StabilityDelay    time.Duration // 稳定判定延迟（默认 2s）
    ErrorRate         float64       // 错误注入概率（默认 0.0）
    Seed              int64         // 随机种子（0 = 使用时间）
}
```

#### 2.2.3 模拟驱动实现规格

**MockPressureDriver**（实现 `PressureDriver` 接口）：
- `Connect()`: 模拟延迟后返回成功
- `Disconnect()`: 模拟延迟后返回成功
- `SetTargetPressure(value)`: 记录目标值，模拟延迟
- `ReadCurrentPressure()`: 返回 `targetValue ± random(PressureJitter)`
- `IsStable()`: 距上次 SetTargetPressure 超过 `StabilityDelay` 后返回 true

**MockMeasureDriver**（实现 `MeasureDriver` 接口）：
- `Connect()`/`Disconnect()`: 同上
- `ReadMeasureData()`: 返回 16 通道模拟数据，每通道 = `基准值 + 通道偏移 + random(ChannelNoise)`
- `StartCalibration()`/`EndCalibration()`/`PerformFitting()`/`SaveCoefficients()`: 状态记录，模拟延迟

**MockWTN1604Driver**（实现 WTN1604 专用接口）：
- 继承 MockMeasureDriver 能力
- `SetValveStatus()`: 记录阀门状态
- `ReadValveStatus()`: 返回记录的阀门状态
- `SetMeasureUnit()`: 记录单位
- `ReadMeasureUnit()`: 返回记录的单位

#### 2.2.4 驱动工厂集成

**修改文件**：`internal/infrastructure/driver/factory.go`

```go
func (f *Factory) CreatePressureDriver(model string, cfg interface{}) (PressureDriver, error) {
    if f.mockConfig.Enabled {
        return mock.NewMockPressureDriver(f.mockConfig), nil
    }
    // 原有真实驱动创建逻辑...
}
```

#### 2.2.5 配置入口

**修改文件**：`internal/config/app_config.go`

```go
type AppConfig struct {
    // ...existing fields...
    UseMock   bool   `json:"useMock"`   // 环境变量 CAL1604_MOCK=true 时为 true
    MockSeed  int64  `json:"mockSeed"`  // 可选：固定随机种子
}
```

**环境变量**：`CAL1604_MOCK=true` 启用模拟模式。

#### 2.2.6 前端独立模式（可选增强）

在 `web/src/services/apiClient.ts` 中增加后端不可用检测：

```ts
// 当后端不可用时，自动切换到前端模拟数据
async function fetchWithMockFallback(url: string, options?: RequestInit) {
    try {
        const response = await fetch(url, options)
        if (!response.ok) throw new Error(response.statusText)
        return response
    } catch (e) {
        if (MOCK_MODE) return generateMockResponse(url, options)
        throw e
    }
}
```

#### 2.2.7 验收标准

- [ ] `MockPressureDriver` 实现完整 `PressureDriver` 接口
- [ ] `MockMeasureDriver` 实现完整 `MeasureDriver` 接口
- [ ] `MockWTN1604Driver` 实现 WTN1604 专用接口
- [ ] 设置 `CAL1604_MOCK=true` 后，应用启动使用模拟驱动，无需真实硬件
- [ ] 模拟驱动的压力值、通道数据在合理范围内波动
- [ ] `ErrorRate > 0` 时能按概率注入错误
- [ ] 驱动工厂在 mock 模式下返回模拟驱动，非 mock 模式下返回真实驱动
- [ ] 前端在 mock 模式下可独立运行完整校准流程

---

### 2.3 [P0-3] 标定/计量模块解耦

**问题**：标定模块与计量模块共用 `useCalibrationStore` 和后端 `calibration.Service`，语义污染、维护困难、无法独立测试。

**目标**：标定与计量模块在 Store 层和 Service 层清晰分离，共享逻辑下沉到公共层。

#### 2.3.1 前端 Store 拆分

**新增文件**：

```
web/src/stores/
├── session/                          # ★ 新增：公共会话 Store
│   └── index.ts                      # 通用会话操作（start/pause/resume/stop）
├── calibration/
│   └── index.ts                      # 标定专属 Store（精简后）
├── measurement/                      # ★ 改造：计量独立 Store
│   ├── index.ts                      # 计量专属 Store（从 calibration 剥离）
│   └── deviceStore.ts                # 保留
└── ...
```

**`useSessionStore`**（公共层）— 从 `useCalibrationStore` 中提取：

```ts
// web/src/stores/session/index.ts
export const useSessionStore = defineStore('session', () => {
    // 通用会话状态
    const sessionState = ref<SessionState>('idle')
    const isRunning = computed(() => ['pressurizing','stabilizing','collecting',...].includes(sessionState.value))

    // 通用会话操作（重命名，去除 Calibration 语义污染）
    async function startSession() { ... }
    async function pauseSession() { ... }
    async function resumeSession() { ... }
    async function stopSession() { ... }

    // 通用数据轮询
    async function refreshPressure() { ... }
    async function refreshStability() { ... }
    async function refreshMeasureData() { ... }

    return { sessionState, isRunning, startSession, pauseSession, resumeSession, stopSession, ... }
})
```

**`useCalibrationStore`**（标定精简后）：

```ts
// web/src/stores/calibration/index.ts
export const useCalibrationStore = defineStore('calibration', () => {
    const sessionStore = useSessionStore()

    // 标定专属状态
    const currentStep = ref<CalibrationStep>(CalibrationStep.DEVICE_CONNECT)
    const selectedChannels = ref<number[]>([])
    const pressurePoints = ref<PressurePoint[]>([])

    // 标定专属操作
    async function fitData() { ... }           // 仅标定有
    async function endCalibration() { ... }     // 仅标定有
    async function setCalibrationChannels() { ... }

    return { ...sessionStoreRefs, currentStep, selectedChannels, pressurePoints, fitData, endCalibration, ... }
})
```

**`useMeasurementStore`**（计量独立）：

```ts
// web/src/stores/measurement/index.ts
export const useMeasurementStore = defineStore('measurement', () => {
    const sessionStore = useSessionStore()

    // 计量专属状态
    const controlMode = ref<'auto' | 'manual'>('auto')
    const pressureMode = ref<'single' | 'roundTrip'>('single')

    // 计量专属操作
    async function generatePressurePoints() { ... }  // 仅计量有
    async function resetCollection() { ... }          // 仅计量有

    // 禁止访问 fitData / endCalibration
    return { ...sessionStoreRefs, controlMode, pressureMode, generatePressurePoints, resetCollection, ... }
})
```

#### 2.3.2 后端 Service 拆分

**新增文件**：

```
internal/application/
├── calibration/
│   └── service.go              # 标定专属（精简后，移除计量逻辑）
├── measurement/                # ★ 新增
│   └── service.go              # 计量专属 Service
├── session/                    # ★ 新增：公共会话编排
│   └── service.go              # 通用会话生命周期管理
└── ...
```

**`session.Service`**（公共层）：

```go
// internal/application/session/service.go
type Service struct {
    stateMachine  *workflow.SessionMachine
    eventBus      *events.Bus
    // 通用设备引用
}

// 通用操作
func (s *Service) Start(ctx context.Context) error
func (s *Service) Pause(ctx context.Context) error
func (s *Service) Resume(ctx context.Context) error
func (s *Service) Stop(ctx context.Context) error
func (s *Service) GetState() domain.SessionState
```

**`calibration.Service`**（精简后）：

```go
// 保留标定专属方法
func (s *Service) Fit(ctx context.Context) error
func (s *Service) EndCalibration(ctx context.Context) error
// 移除计量专属逻辑（如 resetCollection）
```

**`measurement.Service`**（新增）：

```go
// internal/application/measurement/service.go
type Service struct {
    sessionService *session.Service
    // 计量专属依赖
}

func (s *Service) GeneratePressurePoints(ctx context.Context, cfg CalibrationConfig) error
func (s *Service) ResetCollection(ctx context.Context) error
// 无 Fit / EndCalibration
```

#### 2.3.3 状态机实例化

**修改文件**：`internal/workflow/session_machine.go`

当前 `SessionMachine` 是全局单例，改为可实例化：

```go
// 支持多实例
func NewSessionMachine(name string) *SessionMachine {
    return &SessionMachine{
        name:    name,  // "calibration" 或 "measurement"
        current: SessionStateIdle,
        // ...
    }
}
```

标定和计量模块各自持有独立的 `SessionMachine` 实例。

#### 2.3.4 API Handler 调整

**新增文件**：`internal/api/http/measurement_handler.go`

**修改文件**：`internal/api/http/router.go`

```go
// router.go 中注册计量模块独立路由
measurementHandler := NewMeasurementHandler(measurementService)
r.Route("/api/v1/measurement", func(r chi.Router) {
    r.Post("/start", measurementHandler.Start)
    r.Post("/pause", measurementHandler.Pause)
    r.Post("/resume", measurementHandler.Resume)
    r.Post("/stop", measurementHandler.Stop)
    r.Post("/points/generate", measurementHandler.GeneratePoints)
    r.Post("/reset", measurementHandler.ResetCollection)
    // ...
})
```

#### 2.3.5 视图层改造

**修改文件**：

| 文件 | 修改内容 |
|------|----------|
| `web/src/views/CalibrationView.vue` | `useCalibrationStore` → 精简后的标定 Store；移除计量专属逻辑 |
| `web/src/views/MeasurementView.vue` | `useCalibrationStore` → `useMeasurementStore`；移除标定专属逻辑 |

#### 2.3.6 验收标准

- [ ] `useSessionStore` 包含通用会话操作，无 `Calibration` 语义
- [ ] `useCalibrationStore` 仅包含标定专属状态和操作（fitData、endCalibration）
- [ ] `useMeasurementStore` 仅包含计量专属状态和操作（generatePressurePoints、resetCollection）
- [ ] `MeasurementView.vue` 不再 import `useCalibrationStore`
- [ ] 后端 `measurement.Service` 独立存在，不依赖 `calibration.Service`
- [ ] 标定和计量各有独立的 `SessionMachine` 实例，状态互不覆盖
- [ ] `/api/v1/measurement/*` 路由独立注册
- [ ] 原有标定和计量功能不退化（回归测试通过）

---

### 2.4 [P0-4] 建立前端 Composables 层

**问题**：业务逻辑直接写在 Store 或组件中，采集命令、事件监听、报警配置等逻辑分散，无法复用。

**目标**：解耦业务逻辑与状态管理，提高代码复用性。

#### 2.4.1 新增文件

```
web/src/composables/
├── useCollectionCommands.ts    # 采集命令封装
├── useCollectionEvents.ts      # 采集事件监听
├── useAlarmConfig.ts           # 报警配置管理
├── useDeviceConnection.ts      # 设备连接通用逻辑
├── usePressureMonitor.ts       # 压力实时监控
└── usePolling.ts               # 通用轮询控制
```

#### 2.4.2 各 Composable 规格

**useCollectionCommands**：

```ts
// web/src/composables/useCollectionCommands.ts
export function useCollectionCommands(sessionStore: ReturnType<typeof useSessionStore>) {
    // 封装校准/计量流程的 API 调用序列
    async function startAndRun(): Promise<void> {
        await sessionStore.startSession()
        // 自动模式：等待 SSE 事件驱动；手动模式：返回由用户控制
    }

    async function pressurizeAndCollect(pointIndex: number): Promise<void> {
        await apiClient.post('/calibration/pressurize', { index: pointIndex })
        // 等待稳定事件
        await waitForStability()
        await apiClient.post('/calibration/collect', { index: pointIndex })
    }

    // 统一错误处理
    function handleCollectionError(error: unknown): void { ... }

    return { startAndRun, pressurizeAndCollect, handleCollectionError }
}
```

**useCollectionEvents**：

```ts
// web/src/composables/useCollectionEvents.ts
export function useCollectionEvents(
    sessionStore: ReturnType<typeof useSessionStore>,
    deviceStore: ReturnType<typeof useDeviceStore>
) {
    let eventSource: EventSource | null = null

    function connect(): void {
        eventSource = new EventSource('/api/v1/events/stream')
        eventSource.addEventListener('session.state.changed', (e) => {
            sessionStore.syncSessionState(JSON.parse(e.data).state)
        })
        eventSource.addEventListener('device.status.changed', (e) => {
            deviceStore.loadDevices(true)
        })
        eventSource.addEventListener('circuit_breaker.state_changed', (e) => {
            // 断路器状态变更处理
        })
    }

    function disconnect(): void {
        eventSource?.close()
        eventSource = null
    }

    onMounted(() => connect())
    onUnmounted(() => disconnect())

    return { connect, disconnect }
}
```

**useAlarmConfig**：

```ts
// web/src/composables/useAlarmConfig.ts
export function useAlarmConfig() {
    const config = ref<AlarmConfig>({ threshold: 0.05, enabledChannels: [], soundEnabled: true })
    const isSaving = ref(false)

    // 防抖保存（250ms）
    const debouncedSave = useDebounceFn(async () => {
        isSaving.value = true
        await apiClient.post('/calibration/alarm/config', config.value)
        isSaving.value = false
    }, 250)

    // 监听变更自动同步
    watch(config, () => debouncedSave(), { deep: true })

    return { config, isSaving }
}
```

**usePolling**：

```ts
// web/src/composables/usePolling.ts
export function usePolling(
    fetchFn: () => Promise<void>,
    intervalMs: number = 2000,
    options?: { enabled?: Ref<boolean> }
) {
    const isPolling = ref(false)
    let timer: ReturnType<typeof setInterval> | null = null

    function start(): void {
        if (timer) return
        isPolling.value = true
        timer = setInterval(async () => {
            if (options?.enabled && !options.enabled.value) return
            await fetchFn()
        }, intervalMs)
    }

    function stop(): void {
        if (timer) { clearInterval(timer); timer = null }
        isPolling.value = false
    }

    onMounted(() => start())
    onUnmounted(() => stop())

    return { isPolling, start, stop }
}
```

#### 2.4.3 视图层集成

**修改文件**：`web/src/views/CalibrationView.vue`、`web/src/views/MeasurementView.vue`

```ts
// CalibrationView.vue 改造示例
const sessionStore = useSessionStore()
const calibStore = useCalibrationStore()
const { pressurizeAndCollect } = useCollectionCommands(sessionStore)
const { connect: connectEvents, disconnect: disconnectEvents } = useCollectionEvents(sessionStore, deviceStore)
const { config: alarmConfig } = useAlarmConfig()
const { isPolling: isDataPolling } = usePolling(
    async () => {
        await sessionStore.refreshPressure()
        await sessionStore.refreshStability()
        await sessionStore.refreshMeasureData()
    },
    2000,
    { enabled: sessionStore.isRunning }
)
```

#### 2.4.4 验收标准

- [ ] `useCollectionCommands` 封装完整的采集命令序列
- [ ] `useCollectionEvents` 统一管理 SSE 连接生命周期（mount 自动连接，unmount 自动断开）
- [ ] `useAlarmConfig` 支持防抖保存和自动同步
- [ ] `usePolling` 支持条件启停和自动清理
- [ ] `CalibrationView.vue` 和 `MeasurementView.vue` 中无内联 SSE 监听代码
- [ ] `CalibrationView.vue` 和 `MeasurementView.vue` 中无内联 setInterval 轮询代码
- [ ] 各 Composable 有对应的单元测试

---

## 3. Phase 2: 模块解耦与代码质量 (P1)

### 3.1 [P1-1] 拆分设备驱动为独立文件 + SCPI 策略模式

**问题**：5 个设备驱动全部在 `tcp_connection_driver.go` 单文件中，文件过大，新增设备需修改同一文件。

**目标**：每个驱动独立文件，引入 SCPI 策略模式，支持独立测试。

#### 3.1.1 目标结构

```
internal/infrastructure/driver/
├── factory.go                        # 驱动工厂（保留，增加策略注册）
├── factory_test.go
├── tcp_base.go                       # ★ 新增：TCP 连接基础能力（从 tcp_connection_driver.go 提取）
├── tcp_base_test.go
├── scpi/
│   ├── scpi_strategy.go              # ★ 新增：SCPI 策略接口
│   ├── scpi_base.go                  # ★ 新增：SCPI 基础实现（通用命令）
│   ├── const811a.go                  # ★ 新增：ConST 811A 驱动 + 策略
│   ├── const811a_test.go
│   ├── const820.go                   # ★ 新增：ConST 820 驱动 + 策略
│   ├── const820_test.go
│   ├── const860.go                   # ★ 新增：ConST 860 驱动 + 策略
│   ├── const860_test.go
│   └── spc4000.go                    # ★ 新增：SPC4000 驱动 + 策略
│       └── spc4000_test.go
├── wtn1604/
│   ├── wtn1604_driver.go             # ★ 新增：WTN1604 驱动
│   └── wtn1604_driver_test.go
└── mock/                             # Phase 1 已创建
    ├── mock_pressure_driver.go
    ├── mock_measure_driver.go
    └── config.go
```

#### 3.1.2 SCPI 策略接口

```go
// internal/infrastructure/driver/scpi/scpi_strategy.go

type ScpiStrategy interface {
    // 连接后初始化命令序列
    InitSequence() []string
    // 打压命令（返回命令序列）
    ApplyPressure(value float64) []string
    // 解析压力读数
    ParsePressure(resp string) (float64, error)
    // 解析稳定状态
    ParseStability(resp string) (bool, error)
    // 设备标识查询命令
    IdentifyCommand() string
    // 单位设置命令
    SetUnitCommand(unit string) string
    // 单位查询命令
    QueryUnitCommand() string
}
```

#### 3.1.3 各驱动策略实现示例

```go
// internal/infrastructure/driver/scpi/const811a.go

type ConST811AStrategy struct{}

func (s *ConST811AStrategy) InitSequence() []string {
    return []string{"*RST", "*CLS"}
}

func (s *ConST811AStrategy) ApplyPressure(value float64) []string {
    return []string{fmt.Sprintf("SOUR:PRES %f", value), "SOUR:PRES:CONT:STAT ON"}
}

func (s *ConST811AStrategy) ParsePressure(resp string) (float64, error) {
    return strconv.ParseFloat(strings.TrimSpace(resp), 64)
}
// ...
```

#### 3.1.4 TCP 基础提取

```go
// internal/infrastructure/driver/tcp_base.go

type TCPConnection struct {
    conn    net.Conn
    address string
    mu      sync.Mutex
}

func NewTCPConnection(address string) *TCPConnection
func (c *TCPConnection) Connect() error
func (c *TCPConnection) Disconnect() error
func (c *TCPConnection) SendCommand(cmd string) (string, error)
func (c *TCPConnection) IsConnected() bool
```

#### 3.1.5 验收标准

- [ ] `tcp_connection_driver.go` 被拆分，原文件删除或仅保留兼容引用
- [ ] 每个设备驱动有独立文件和独立测试
- [ ] `ScpiStrategy` 接口定义完整，4 种 SCPI 设备各有策略实现
- [ ] `TCPConnection` 提取为公共基础，所有驱动复用
- [ ] 驱动工厂通过策略名创建驱动，新增设备只需添加策略实现
- [ ] 所有原有驱动功能不退化

---

### 3.2 [P1-2] 完善报警机制

**问题**：存在 `alarm_service.go` 但缺少声音报警、用户确认交互、报警事件推送、通道级控制。

#### 3.2.1 后端增强

**修改文件**：`internal/workflow/alarm_service.go`

```go
type AlarmConfig struct {
    Threshold       float64  // 报警阈值（偏差百分比，默认 0.05）
    EnabledChannels []int    // 启用报警的通道列表（空 = 全部启用）
    SoundEnabled    bool     // 是否启用声音报警
}

type AlarmEvent struct {
    Type      string    // "threshold_exceeded"
    PointIndex int
    Channel   int
    Value     float64
    Threshold float64
    Timestamp time.Time
}
```

报警事件通过 `events.Bus` 推送：

```go
func (s *AlarmService) CheckAndNotify(data map[int]float64, target float64) (*AlarmEvent, error) {
    for ch, val := range data {
        if !s.isChannelEnabled(ch) { continue }
        if deviation(val, target) > s.config.Threshold {
            event := &AlarmEvent{...}
            s.eventBus.Publish(events.Event{
                Type: "calibration.alarm",
                Data: event,
            })
            return event, nil
        }
    }
    return nil, nil
}
```

#### 3.2.2 前端增强

**修改文件**：`web/src/composables/useAlarmConfig.ts`（Phase 1 已创建）

增加 SSE 监听报警事件 + 弹窗确认：

```ts
// 监听报警事件
eventSource.addEventListener('calibration.alarm', async (e) => {
    const alarm = JSON.parse(e.data)
    if (alarmConfig.value.soundEnabled) {
        playAlarmSound()  // Web Audio API 提示音
    }
    // 弹出确认对话框
    const decision = await showAlarmConfirmDialog(alarm)
    // 回传用户决策
    await apiClient.post('/calibration/alarm', { decision, pointIndex: alarm.pointIndex })
})
```

#### 3.2.3 验收标准

- [ ] 报警事件通过 SSE 推送到前端
- [ ] 前端收到报警事件后弹出确认对话框（继续/重试）
- [ ] 用户选择后通过 REST API 回传，后端根据决策继续或重试
- [ ] 支持通道级报警启用/禁用
- [ ] 声音报警可通过配置开关

---

### 3.3 [P1-3] 完善报告导出

**问题**：仅有模板路径选择，缺少 Excel 生成、导出进度、路径选择。

#### 3.3.1 后端增强

**修改文件**：`internal/report/report_service.go`

```go
import "github.com/xuri/excelize/v2"

type ReportService struct {
    templateSelector *TemplateSelector
    eventBus         *events.Bus
}

// GenerateReport 根据模板和采集数据生成 Excel 报告
func (s *ReportService) GenerateReport(ctx context.Context, templatePath string, data *CalibrationResult, outputPath string) error {
    // 1. 打开模板
    f, err := excelize.OpenFile(templatePath)
    // 2. 填充数据到对应 sheet/cell
    // 3. 通过 SSE 推送进度
    s.eventBus.Publish(events.Event{Type: "report.progress", Data: map[string]interface{}{"percent": 30}})
    // 4. 保存到 outputPath
    return f.SaveAs(outputPath)
}
```

#### 3.3.2 前端增强

**修改文件**：`web/src/views/CalibrationView.vue`

```ts
async function exportReport() {
    // 1. Wails Runtime 打开文件选择对话框
    const outputPath = await window.WailsRuntime.OpenFileDialog({
        Title: '选择报告保存路径',
        DefaultFilename: `校准报告_${new Date().toISOString().slice(0,10)}.xlsx`,
    })
    if (!outputPath) return

    // 2. 调用后端生成报告
    await apiClient.post('/report/generate', { templatePath: selectedTemplate, outputPath })
    // 3. 进度通过 SSE 事件 report.progress 监听
}
```

#### 3.3.3 验收标准

- [ ] 使用 excelize 库根据模板生成 Excel 文件
- [ ] 导出进度通过 SSE 推送（percent: 0~100）
- [ ] 前端通过 Wails Runtime 选择输出路径
- [ ] 导出前验证采集会话数据完整性

---

### 3.4 [P1-4] 修复 controlMode/pressureMode 参数断层

**问题**：前端有 UI 但未传参，后端未接收，双向断层。

#### 3.4.1 修改清单

| 文件 | 修改内容 |
|------|----------|
| `web/src/views/CalibrationView.vue` | 将局部 `controlMode`/`pressureMode` ref 传入 `startCalibration()` 或 `setCalibrationConfig()` |
| `web/src/stores/calibration/index.ts` | `startCalibration()` 接收并传递 `controlMode`/`pressureMode` 参数 |
| `web/src/services/apiClient.ts` | `POST /sessions/start` 请求体增加 `controlMode`/`pressureMode` 字段 |
| `internal/api/http/session_handler.go` | `sessionStartHandler` 从请求体解析 `controlMode`/`pressureMode` |
| `internal/application/calibration/service.go` | `StartCalibration` 接收并应用 `controlMode`/`pressureMode` |

#### 3.4.2 验收标准

- [ ] 前端 UI 选择的 controlMode/pressureMode 成功传递到后端
- [ ] 后端根据 controlMode 决定自动/手动采集模式
- [ ] 后端根据 pressureMode 决定单程/回程压力点生成

---

### 3.5 [P1-5] 清理废弃组件

**问题**：`PressurePointList.vue` 状态枚举与当前 Store 不一致，处于未使用状态。

#### 3.5.1 修改清单

| 操作 | 文件 | 说明 |
|------|------|------|
| 删除 | `web/src/components/calibration/PressurePointList.vue` | 状态模型已过时，页面未使用 |
| 清理 | `web/src/views/CalibrationView.vue` | 移除对 PressurePointList 的 import（如有残留） |
| 清理 | `web/src/views/MeasurementView.vue` | 同上 |

#### 3.5.2 验收标准

- [ ] `PressurePointList.vue` 已删除
- [ ] 无残留 import 引用
- [ ] 标定/计量页面功能不受影响

---

## 4. Phase 3: 功能完善与工程化 (P2)

### 4.1 [P2-1] 前后端类型同步

**问题**：前端 TypeScript 接口手动维护，与后端 Go 结构体手动对齐，易遗漏。

#### 4.1.1 短期方案：手动对齐 + API 契约测试

**新增文件**：`web/src/__tests__/api-contract.test.ts`

```ts
// API 契约测试：验证前端类型定义与后端实际响应一致
describe('API Contract', () => {
    test('SessionState 枚举与后端一致', async () => {
        const resp = await apiClient.get('/sessions/state')
        expect(Object.values(SessionState)).toContain(resp.data.state)
    })

    test('PressurePoint 结构与后端一致', async () => {
        const resp = await apiClient.get('/calibration/points')
        const point = resp.data[0]
        expect(point).toHaveProperty('index')
        expect(point).toHaveProperty('targetPressure')
        expect(point).toHaveProperty('status')
        // ...
    })
})
```

#### 4.1.2 中期方案（可选）：OpenAPI 规范

- 后端 Handler 注释中添加 OpenAPI 注解
- 构建时生成 OpenAPI spec
- 前端通过 openapi-typescript 生成 TS 类型

#### 4.1.3 验收标准

- [ ] API 契约测试覆盖所有核心接口的请求/响应结构
- [ ] 契约测试在 CI 中运行
- [ ] 类型不匹配时测试失败并给出明确提示

---

### 4.2 [P2-2] 独立配置服务

**问题**：配置通过 `AppConfig` 结构体 + JSON 文件加载，无独立配置服务。

#### 4.2.1 新增文件

```
internal/config/
├── app_config.go          # 保留
├── config_service.go      # ★ 新增：配置服务
└── config_service_test.go
```

```go
type ConfigService struct {
    mu       sync.RWMutex
    filePath string
    config   *AppConfig
}

// Load 加载配置文件
func (s *ConfigService) Load(ctx context.Context) error

// Get 获取当前配置（只读副本）
func (s *ConfigService) Get() AppConfig

// Update 部分更新配置并持久化
func (s *ConfigService) Update(ctx context.Context, patch map[string]interface{}) error

// ResetToDefaults 重置为默认配置
func (s *ConfigService) ResetToDefaults(ctx context.Context) error

// Watch 监听配置文件变更（fsnotify）
func (s *ConfigService) Watch(ctx context.Context, onChange func(AppConfig))
```

#### 4.2.2 验收标准

- [ ] `ConfigService` 支持加载、部分更新、重置默认
- [ ] 配置变更自动持久化到 JSON 文件
- [ ] 支持文件变更监听（可选）

---

### 4.3 [P2-3] 输入验证器

**问题**：验证逻辑分散在各 Handler 和 Service 中。

#### 4.3.1 新增文件

```
internal/validation/
├── validators.go          # 通用验证函数
└── validators_test.go
```

```go
func ValidateIP(ip string) error
func ValidatePort(port int) error
func ValidatePressure(value, min, max float64) error
func ValidateChannels(channels []int) error       // 1~16，不重复
func ValidatePointCount(count int) error          // 2~11
func ValidateAverageCount(count int) error        // 1~100
```

#### 4.3.2 验收标准

- [ ] 验证器覆盖 IP、端口、压力值、通道、点数、采样次数
- [ ] Handler 层调用验证器，验证失败返回 400 + 明确错误信息
- [ ] 验证器有完整单元测试

---

### 4.4 [P2-4] 结构化日志

**问题**：Go 端使用标准 `log` 包，无结构化日志。

#### 4.4.1 方案

引入 `slog`（Go 1.21+ 标准库）替换 `log`：

```go
import "log/slog"

// 替换所有 log.Printf 为 slog.Info/slog.Error/slog.Warn
slog.Info("device connected",
    "device_id", deviceID,
    "model", model,
    "duration_ms", elapsed.Milliseconds(),
)
```

#### 4.4.2 验收标准

- [ ] 所有 `log.Printf`/`log.Println` 替换为 `slog` 调用
- [ ] 日志输出包含结构化字段（device_id、operation、error_code 等）
- [ ] 支持日志级别配置（DEBUG/INFO/WARN/ERROR）

---

## 5. 跨Phase依赖关系

```
Phase 1 (P0)                          Phase 2 (P1)                     Phase 3 (P2)
─────────────                         ─────────────                    ─────────────
P0-1 断路器 ──────────────────────> P1-1 驱动拆分（断路器包装驱动）
     │
     └──> P0-2 模拟层 ──────────> P1-1 驱动拆分（模拟驱动纳入新结构）

P0-3 标定/计量解耦 ─────────────> P1-4 参数断层修复（解耦后各模块独立修复）
     │                          > P1-2 报警完善（解耦后各模块独立报警配置）
     │
     └──> P0-4 Composables ────> P1-2 报警完善（useAlarmConfig 增强）
                              > P1-3 报告导出（Composable 封装导出逻辑）

P1-1 驱动拆分 ─────────────────> P2-1 类型同步（驱动接口变更触发契约测试更新）
```

**关键路径**：P0-3（模块解耦）是后续大部分工作的前提，应优先完成。

---

## 6. 验收标准总表

### Phase 1 验收

| # | 优化项 | 核心验收条件 |
|---|--------|-------------|
| P0-1 | 断路器 | 设备连续故障5次后熔断；状态变更SSE推送；原有重试逻辑替换 |
| P0-2 | 模拟层 | `CAL1604_MOCK=true` 启动后无需硬件可运行完整流程 |
| P0-3 | 模块解耦 | 计量视图不 import 标定 Store；后端有独立 measurement.Service |
| P0-4 | Composables | 视图中无内联 SSE/轮询代码；各 Composable 有单元测试 |

### Phase 2 验收

| # | 优化项 | 核心验收条件 |
|---|--------|-------------|
| P1-1 | 驱动拆分 | 每驱动独立文件+测试；SCPI策略模式可扩展 |
| P1-2 | 报警完善 | 报警事件SSE推送+前端弹窗确认+声音提示+通道级控制 |
| P1-3 | 报告导出 | Excel生成+进度推送+路径选择 |
| P1-4 | 参数断层 | controlMode/pressureMode 前端→后端全链路贯通 |
| P1-5 | 废弃清理 | PressurePointList.vue 删除，无残留引用 |

### Phase 3 验收

| # | 优化项 | 核心验收条件 |
|---|--------|-------------|
| P2-1 | 类型同步 | API契约测试覆盖核心接口，CI中运行 |
| P2-2 | 配置服务 | 支持部分更新+持久化+重置默认 |
| P2-3 | 验证器 | Handler层统一调用，验证失败返回400 |
| P2-4 | 结构化日志 | 所有log替换为slog，输出含结构化字段 |

---

## 附录 A: 文件变更汇总

### 新增文件（共 ~20 个）

| Phase | 文件路径 | 说明 |
|-------|----------|------|
| P0 | `internal/resilience/circuit_breaker.go` | 断路器实现 |
| P0 | `internal/resilience/circuit_breaker_test.go` | 断路器测试 |
| P0 | `internal/resilience/retry_strategy.go` | 重试策略 |
| P0 | `internal/resilience/retry_strategy_test.go` | 重试策略测试 |
| P0 | `internal/resilience/config.go` | 弹性配置 |
| P0 | `internal/infrastructure/driver/mock/mock_pressure_driver.go` | 打压模拟 |
| P0 | `internal/infrastructure/driver/mock/mock_measure_driver.go` | 计量模拟 |
| P0 | `internal/infrastructure/driver/mock/mock_wtn1604_driver.go` | WTN1604模拟 |
| P0 | `internal/infrastructure/driver/mock/config.go` | 模拟配置 |
| P0 | `internal/application/session/service.go` | 公共会话服务 |
| P0 | `internal/application/measurement/service.go` | 计量服务 |
| P0 | `internal/api/http/measurement_handler.go` | 计量API |
| P0 | `web/src/stores/session/index.ts` | 公共会话Store |
| P0 | `web/src/stores/measurement/index.ts` | 计量Store |
| P0 | `web/src/composables/useCollectionCommands.ts` | 采集命令 |
| P0 | `web/src/composables/useCollectionEvents.ts` | 事件监听 |
| P0 | `web/src/composables/useAlarmConfig.ts` | 报警配置 |
| P0 | `web/src/composables/useDeviceConnection.ts` | 设备连接 |
| P0 | `web/src/composables/usePressureMonitor.ts` | 压力监控 |
| P0 | `web/src/composables/usePolling.ts` | 通用轮询 |
| P1 | `internal/infrastructure/driver/tcp_base.go` | TCP基础 |
| P1 | `internal/infrastructure/driver/scpi/scpi_strategy.go` | SCPI策略接口 |
| P1 | `internal/infrastructure/driver/scpi/scpi_base.go` | SCPI基础 |
| P1 | `internal/infrastructure/driver/scpi/const811a.go` | 811A驱动 |
| P1 | `internal/infrastructure/driver/scpi/const820.go` | 820驱动 |
| P1 | `internal/infrastructure/driver/scpi/const860.go` | 860驱动 |
| P1 | `internal/infrastructure/driver/scpi/spc4000.go` | SPC4000驱动 |
| P1 | `internal/infrastructure/driver/wtn1604/wtn1604_driver.go` | WTN1604驱动 |
| P2 | `internal/config/config_service.go` | 配置服务 |
| P2 | `internal/validation/validators.go` | 验证器 |

### 主要修改文件（共 ~15 个）

| Phase | 文件路径 | 修改内容 |
|-------|----------|----------|
| P0 | `internal/application/deviceconnect/service.go` | 断路器+重试策略集成 |
| P0 | `internal/application/calibration/service.go` | 断路器包装；精简为标定专属 |
| P0 | `internal/device/manager/device_manager.go` | 每设备独立断路器 |
| P0 | `internal/infrastructure/driver/factory.go` | mock 模式返回模拟驱动 |
| P0 | `internal/config/app_config.go` | 增加 UseMock 字段 |
| P0 | `internal/workflow/session_machine.go` | 支持多实例 |
| P0 | `internal/api/http/router.go` | 注册计量模块路由 |
| P0 | `web/src/stores/calibration/index.ts` | 精简为标定专属 |
| P0 | `web/src/views/CalibrationView.vue` | 使用 Composable + 精简 Store |
| P0 | `web/src/views/MeasurementView.vue` | 使用 measurementStore + Composable |
| P1 | `internal/workflow/alarm_service.go` | 通道级控制+事件推送 |
| P1 | `internal/report/report_service.go` | Excel生成+进度推送 |
| P1 | `internal/api/http/session_handler.go` | 解析 controlMode/pressureMode |
| P2 | 所有 `log.Printf` 调用点 | 替换为 slog |

### 删除文件

| Phase | 文件路径 | 说明 |
|-------|----------|------|
| P1 | `web/src/components/calibration/PressurePointList.vue` | 废弃组件 |
| P1 | `internal/infrastructure/driver/tcp_connection_driver.go` | 拆分后删除（或保留兼容引用） |

---

## 附录 B: 建议实施顺序

```
Step 1: P0-1 断路器 + 重试策略（独立模块，无依赖）
Step 2: P0-2 模拟层（独立模块，仅依赖驱动接口）
Step 3: P0-3 标定/计量解耦（关键路径，后续工作前提）
Step 4: P0-4 Composables 层（依赖 P0-3 的 Store 拆分结果）
  ── Phase 1 完成，回归测试 ──
Step 5: P1-1 驱动拆分 + SCPI 策略（依赖 P0-1 断路器、P0-2 模拟层）
Step 6: P1-2 报警完善（依赖 P0-3 解耦、P0-4 Composables）
Step 7: P1-3 报告导出（依赖 P0-4 Composables）
Step 8: P1-4 参数断层修复（依赖 P0-3 解耦）
Step 9: P1-5 废弃组件清理（独立）
  ── Phase 2 完成，回归测试 ──
Step 10: P2-1 ~ P2-4 工程化提升（可并行）
  ── Phase 3 完成 ──
```
