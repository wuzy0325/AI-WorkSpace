# Spec: WindLabX4 双探针并行遍历测试

> 日期：2026-07-27
> 状态：待审批
> 修订：v3（根据实施计划审查与现有 checkpoint v2 基线修订）
> 关联：
> - [遍历可靠性与断点恢复](./spec-traversal-reliability-and-recovery.md)
> - [遍历运动安全](./spec-traversal-motion-safety.md)

## Objective

当前 WindLabX4 仅支持一个遍历任务。客户有两套物理独立的运动控制器与探针支架，希望在同一次吹风周期内并行测量两支探针。两支探针可以同型号或异型号，分别使用自己的探针文件、通道绑定、点位表、运动控制器和输出文件。

本功能增加单探针/双探针模式，但不改变既有单探针页面、HTTP 契约和运行语义。双探针模式中的两个任务必须做到状态、配置、插值器、运动控制、输出会话和恢复状态相互隔离；允许共享只读采集数据总线，但不得共享有状态输出端口或运动资源。

### 用户故事

- 操作员在没有活动任务时切换“单探针模式”或“双探针模式”。
- 双探针模式同时展示两支探针的进度、当前点位、关键数值和 Warning。
- 每支探针独立配置型号、PRB/校准 CSV、通道、点位表、大气压、温度、运动控制器和输出路径。
- 每支探针独立启动、暂停、恢复和停止；一支完成或发生普通任务错误时，另一支继续运行。
- 每支探针在运行期间独立写入自己的 CSV、结果日志和 checkpoint。

### 范围外

- 不新增遍历 CSV 的“导出/复制”API。这里的“CSV 独立”指运行期间独立配置和持续落盘。
- 不支持两支探针共享同一运动控制器，即使绑定不同物理轴。
- 不支持运行或暂停期间切换单/双探针模式。
- 不改变五孔、七孔插值算法和现有 CSV 列定义。
- 不改变采集设备的所有权；遍历读取共享采集数据，不因单个任务停止而停止设备采集。
- 不新增第三方后端依赖，不引入 chi router。

## Core Invariants

### I1. 运动资源独占

- 双探针的每一路必须显式绑定一个非空运动控制器 ID。
- `probe1` 与 `probe2` 必须绑定不同控制器；相同控制器配置在启动前拒绝。
- 双模式禁止使用“空绑定时操作所有已连接控制器”的兼容回退。
- 普通 Stop 和 EmergencyStop 只作用于该任务启动快照中绑定的控制器和轴。
- 两路资源检查与预占必须是原子操作，不能出现两边各自检查通过后同时占用同一资源的 TOCTOU 竞态。

### I2. 工作流互斥

- registry 负责双探针工作流的统一准入；不能让两个子 manager 分别争抢当前全局 `workflow:traversal` 锁。
- 双探针工作流整体仍遵循现有遍历与其它不兼容工作流的互斥语义。
- registry 使用固定 holder 身份持有一份全局 traversal workflow lease：第一个 probe session 启动前获取，活动 session 计数从 0 变为 1；后续 probe 复用同一 lease；最后一个 session 完成清理、计数从 1 变为 0 后释放。
- 获取全局 lease、预占控制器和登记 session 必须作为一个带回滚的准入事务；任一步失败都撤销本次已取得的资源，不影响既有活动 session。
- probe-scoped Start/RunPoint/Pause/Resume/Stop/ResumeFromCheckpoint/ClearCheckpoint/Close 必须通过 registry façade；HTTP handler 不得先 `GetOrCreate` manager 后直接调用生命周期方法绕过 session ownership。
- 准入事务提交时由 registry 生成不可复用的 session token（包含 probe ID 与 generation）。manager 的完成通知必须携带该 token；registry 仅在 token 仍是该 probe 当前 session 且尚未完成时原子减计数一次。
- normal completion、`CloseProbe`、Stop 和 `Shutdown` 的重复/并发完成通知必须幂等；旧 generation 通知只记录诊断，不得影响当前 session、控制器 lease 或全局计数。
- 准入事务提交前的回滚直接撤销临时资源，不走 session 完成计数；防止“未计入却减一”。
- registry-managed dual session 的 manager 禁止直接释放全局 traversal workflow lease；只能通知 registry 完成 session，由 registry 在最后一路清理后释放。legacy single manager 保留既有直接 lease 路径。
- 每个运行会话额外持有其控制器资源锁，直到 goroutine 退出、输出端口关闭后释放。
- 锁必须由会话生命周期持有，不依赖固定 24 小时 TTL 后被其它任务接管；若底层锁保留 TTL，运行期间必须续约。
- 控制器资源锁使用独立 lease 端口，至少提供 token-checked Acquire/Renew/Release；旧 generation 不得续约或释放新 session 的 lease。

### I3. 运行状态隔离

每个 probe session 独立拥有以下全部可变状态，而不只是一份 config/status：

- manager、run session、取消上下文和完成信号；
- config、status、启动防重入状态、错误和完成事件；
- 五孔/七孔插值器、实时插值缓存与加载状态；
- CSV writer、结果日志 writer、checkpoint port；
- checkpoint、活动索引记录和结果记录；
- 前端轮询订阅、请求 generation、实时计算 timer 和 stale-response 防护。

### I4. 共享服务边界

- `AcquisitionHub`、`MotionAccess`、`DeviceManager`、配置文件存储和活动索引服务可以共享，但必须明确支持并发调用。
- CSV writer、结果日志 writer 和 manager 内部 session 状态禁止跨 probe 共享实例。
- 两路可以读取同一 DAQ 设备的最新数据；每路只消费自己配置的 channel refs。
- 停止、删除或卸载一个 probe session 不得停止共享采集，也不得取消另一 probe 的订阅。

### I5. 全局任务身份

- 每次运行必须有服务端权威、进程内及持久化范围全局唯一的 `taskId`。
- 双模式 task ID 使用包含 probe 身份的命名空间或服务端 UUID，不允许 `probe1` 和 `probe2` 在结果 store、活动索引或 checkpoint 中发生键冲突。
- dual Start 的 task ID 由服务端 `TaskIDGenerator` 生成；客户端提交值不得成为权威 dual task ID。legacy single 请求契约保持不变。
- `probeId` 作为独立元数据写入日志和 checkpoint；不得仅依赖解析文件名恢复 probe 身份。

### I6. 终态与清理

- 正常完成：关闭并 Sync 输出端口，保存最终结果，注销可恢复 checkpoint 和活动索引，再释放资源锁。
- 用户停止：遵循既有可靠性规格，等待运行协程退出并保留该 probe 唯一的可恢复 checkpoint；恢复索引必须保留，使重启后可发现。
- 错误：包括位置超差、限位、控制器掉线和该 controller 的 EmergencyStop 在内，均只停止并锁存该 probe 启动快照中的控制器，不得向另一 probe 的控制器发送 Stop/EmergencyStop。
- registry 删除条目前必须完成 `cancel/stop -> wait Done -> finalize -> release`。只从 map `delete` 指针是禁止的。
- 应用退出时 registry 拒绝新启动，并行 shutdown 所有 manager，在明确超时内等待并聚合错误。

## Functional Requirements

### FR1. 模式切换

- 模式组合边界位于 `TraversalView.vue`：
  - single 渲染现有 `TraversalMain`，保持既有 DOM 结构、布局和交互；
  - dual 渲染新的 `DualTraversalMain`。
- 任一 single/dual session 为 running、moving、stabilizing、acquiring、saving 或 paused 时，模式开关禁用并显示原因。
- 只有所有 session 均为 idle/completed/error/stopped 且清理完成后才能切换模式。
- UI 模式切换不直接删除活动 manager；终态 manager 由 registry 的显式关闭操作安全清理。

### FR2. Probe ID 与配置

- 本功能固定支持 `probe1`、`probe2` 两个 `ProbeId`，其它值返回 `400 invalid_probe_id`。
- 两路各自保存完整 `TraversalTestConfig`，包括探针类型、文件身份、通道、点位、运动绑定、环境参数和输出设置。
- 配置持久化使用 probe-scoped key（例如 `traversal.probe1`、`traversal.probe2`）；既有 single 模式继续使用 `traversal` key。
- 双模式不自动迁移 single 配置，也不覆盖既有 single 配置。
- 双路启动前必须原子校验两个控制器 ID 非空且不同；运行期间另一 probe 不得改用已被占用的控制器。

### FR3. Manager Registry 与工厂

`ManagerRegistry` 位于 usecase，只依赖接口和 manager factory，不直接创建 storage/hardware adapter：

```go
type ProbeID string

const (
    Probe1 ProbeID = "probe1"
    Probe2 ProbeID = "probe2"
)

type TraversalManagerFactory interface {
    NewManager(probeID ProbeID) (ManagedTraversalManager, error)
}

type ManagerRegistry struct {
    mu       sync.RWMutex
    closing  bool
    factory  TraversalManagerFactory
    managers map[ProbeID]ManagedTraversalManager
}
```

- `GetOrCreate` 校验 probe ID，并通过 factory 创建完整装配的 manager。
- factory 在 composition root 实现，明确区分共享依赖和每 manager 新建的有状态端口。
- `CloseProbe(ctx, probeID)` 在 map 中保留 closing 状态，完成 manager shutdown 后才删除。
- cleanup 超时或失败时保留条目，禁止立即创建替代 manager。
- `Shutdown(ctx)` 原子设置 `closing=true`、拒绝新任务、并行关闭所有 manager并返回聚合错误。
- registry 维护当前 session token、活动 session 计数和全局 workflow lease；manager 只携带 token 回报清理完成，不自行释放该全局 lease。
- registry 暴露 Start/RunPoint/Pause/Resume/Stop/ResumeFromCheckpoint/ClearCheckpoint/Close façade；managed manager 生命周期入口仅由 registry 调用。legacy single manager 可保留既有直接 lease 路径，两种所有权不得在同一 session 混用。
- 以下三个生产装配流必须使用同一 factory/registry 语义：
  - `pkg/appcontext.NewAppContext`；
  - `internal/bootstrap.BuildAPIServer`；
  - `pkg/apiserver.Start`。

每个 manager 必须新建：

- `TraversalCsvWriter`；
- `TraversalResultLog`；
- manager 自身和实时插值 cache；
- 当前任务 checkpoint port（仍可由共享 factory 按输出路径创建）。

可以共享但必须线程安全：

- `AcquisitionHub`/latest-data reader；
- `MotionAccess`；
- `DeviceManager` 的 unit/acquisition 查询端口；
- checkpoint store/factory、dual recovery index、app config store。dual manager 不注入或使用 legacy `traversal-active-index.json`。

### FR4. HTTP API

项目继续使用标准库 `net/http.ServeMux`。`api.Deps` 增加 registry 接口；handler 严格解析两段路径 `{probeId}/{action}`，选择 manager 后委托共享 action handler。

双模式 manager-scoped 路由：

```text
GET  /api/traversal/{probeId}/config
POST /api/traversal/{probeId}/config
POST /api/traversal/{probeId}/importPrb
POST /api/traversal/{probeId}/importCalibrationCsv
POST /api/traversal/{probeId}/importMultiPrb
POST /api/traversal/{probeId}/importSevenHolePrb
POST /api/traversal/{probeId}/importSevenHoleCalibrationCsv
POST /api/traversal/{probeId}/clearInterpolator
POST /api/traversal/{probeId}/calculateRealtime
POST /api/traversal/{probeId}/checkPreconditions
POST /api/traversal/{probeId}/start
GET  /api/traversal/{probeId}/status
POST /api/traversal/{probeId}/runPoint
POST /api/traversal/{probeId}/pause
POST /api/traversal/{probeId}/resume
POST /api/traversal/{probeId}/stop
POST /api/traversal/{probeId}/close
GET  /api/traversal/{probeId}/result?taskId=...
GET  /api/traversal/{probeId}/loadCheckpoint
POST /api/traversal/{probeId}/resumeFromCheckpoint
POST /api/traversal/{probeId}/clearCheckpoint
```

`generateGrid` 是纯操作，可以保留既有无 probe 路由。现有 `/api/traversal/{action}` 单探针 API 必须保持请求、响应和错误语义不变，不隐式转发到 `probe1`。

API 必须区分：未知 probe、manager 创建失败、资源冲突、同 probe 已运行、registry closing、任务 ID 不匹配和普通业务错误。服务端不得接受客户端提交的 checkpoint 文件路径作为恢复权威来源。

每个 probe 同时最多保留一个可恢复任务：

- 双模式使用独立的 `dual-traversal-recovery-index.json`，采用带 `version: 1` 的 envelope，维护 `probeId -> taskId -> checkpointPath` 权威映射；task ID 仍保持全局唯一。
- 既有 single `traversal-active-index.json` 的格式和读写逻辑保持不变。dual index 不读取、迁移或覆盖 legacy index；两个文件均通过各自的原子替换流程更新。
- stopped/error checkpoint 存在时，该 probe 的新 Start 返回 `recoverable_task_exists`，操作员必须恢复原任务或显式放弃后才能新建任务。
- `loadCheckpoint` 只返回该 probe 映射的唯一候选，不扫描目录猜测。
- `resumeFromCheckpoint` 和 `clearCheckpoint` 请求体只携带 `taskId` 作为用户确认；服务端必须验证它等于恢复索引中的 task ID，再加载权威路径。
- 正常完成或显式放弃后，原子注销 dual recovery mapping；dual 路径不读写 legacy `traversal-active-index.json`。失败时不得创建新任务覆盖旧映射。

### FR5. 前端状态模型

前端使用 keyed session，而不是平行增加少量 `dualConfig`/`dualStatus` 字段：

```typescript
type ProbeId = 'probe1' | 'probe2'

interface TraversalSessionState {
  config: TraversalTestConfig | null
  status: TraversalTestStatus | null
  isStarting: boolean
  error: string | null
  completeEvent: TraversalCompleteEvent | null
  checkpoint: TraversalCheckpoint | null
  realtimePressures: TraversalRawPressure | null
  realtimeResult: InterpolationResult | null
  hasLoadedInterpolator: boolean
  interpolatorRestoreMessage: string | null
}
```

- 双模式 store/composable 以 `Record<ProbeId, TraversalSessionState>` 管理状态。
- 请求 ID、启动窗口、订阅句柄、实时计算 timer 和 pending input 同样按 probe ID 存储。
- 每个 action 显式接收 `ProbeId`；一路 reset、失败或卸载不得修改另一条 session。
- 可复用展示组件通过 props/emits 或 scoped controller 消费状态，不得在双路 row 内再次解析全局 singleton store。
- 既有 single store API 保留，避免修改原单探针组件行为。

### FR6. 轮询与实时数据

- 每个 probe ID 有一个共享轮询 channel，供该 probe 的 progress/complete/error 订阅复用。
- 每个 channel 同时最多一个 in-flight status 请求；上一请求未结束时不再叠加请求。
- 首个 subscriber 注册后立即请求一次，此后按 500ms 调度。
- 使用 generation 或 AbortController 丢弃取消订阅、切换模式或新任务启动后的旧响应。
- 最后一个 subscriber 注销时停止该 probe 的 timer；停止一路不得停止另一路 timer。
- 实时 DAQ 订阅覆盖该 probe 配置中所有唯一 `deviceId`，使用 device-level 引用计数；两路共享设备时，一路卸载不能取消另一条订阅。
- 500ms 双路轮询（最多 4 req/s）作为本功能已确认基线，不新增批量 status API；性能测试不通过时另立优化规格。

### FR7. UI 布局

- 双模式使用上下两个 `DualProbeRow`，顶部使用双列摘要状态栏。
- 不把现有 `TraversalLiveMonitor` 简单缩到 240px；新增 compact probe monitor，仅保留：
  - 状态、进度和当前点位；
  - Alpha/Beta、总压、静压、速度等与探针类型对应的关键值；
  - Warning/Error 摘要；
  - 独立启动、暂停、恢复、停止和设置入口。
- 完整通道值、运动状态、点位预览和诊断信息放在该 row 的 Tab/详情区域。
- 每个 row 的详情区可独立滚动；控制栏和 Warning 不得被滚动隐藏。
- 遵循 `DESIGN.md` 的固定桌面布局，不增加移动端 breakpoint。
- 在 1440x900 和 1600x900、light/dark、五孔/七孔组合、长错误文本和双 Warning 下均不得遮挡关键控制。

### FR8. CSV、结果日志与恢复

- 每路运行使用独立 CSV、结果日志和 checkpoint port。
- 当前可靠性 checkpoint v2（`Checkpoint` + `TraversalRunSnapshot`、commitSeq、真实 CSV/结果日志路径和 hash）是兼容基线。双模式新建 checkpoint 使用格式版本 3，在完整 v2 字段基础上增加 `ProbeID` 与 `BoundControllerIDs`；既有 single checkpoint v1/v2 继续只由 legacy single 路由按原行为读取，不自动迁移为双模式 checkpoint。
- 输出文件名以 probe ID 为前缀或包含等价可辨识片段，同时继续使用现有唯一文件 `-2/-3` 防覆盖机制。
- 实际最终路径以后端 status/checkpoint 返回值为准，不能由前端按期望文件名推断。
- 每路继续遵守可靠性规格的结果日志 Sync -> CSV Sync -> checkpoint 原子替换协议。
- 一路 writer 的 Open/Append/Sync/Close 故障只能使该路进入 error，不得关闭另一 writer。
- 恢复通过 `probeId -> taskId -> checkpointPath` 权威映射查找唯一 checkpoint；checkpoint 中的 probe ID、task ID 和控制器绑定必须匹配。
- 恢复必须在打开 append 输出端口或执行任何运动前，走与新 Start 完全相同的全局 workflow lease + 控制器预占 + session token 准入事务；checkpoint 控制器已被其它 session 占用时返回 `resource_conflict`，且不得修改 checkpoint 或输出文件。
- 相同 task ID 在不同 probe 的索引注册必须被拒绝或通过服务端命名空间保证不可能发生。

### FR9. 应用退出

- Wails `ServiceShutdown` 和独立 API server shutdown 都必须在关闭共享服务前调用 registry `Shutdown(ctx)`。
- `cmd/server` 必须使用 `signal.NotifyContext`（或等价机制）拥有 `http.Server` shutdown；不得继续使用无法执行有序清理的裸 `http.ListenAndServe` 阻塞路径。
- shutdown 同时请求两路停止，但分别等待和聚合错误；一路卡住不能跳过另一条的清理。
- 默认 graceful deadline 为 5 秒，整个 shutdown hard deadline 为自 shutdown 开始起 10 秒；两个值可在 composition root 通过现有配置方式覆盖，但必须为有限正值且 hard > graceful。
- graceful deadline 到期后，对仍未退出任务的所属控制器逐台并发执行 EmergencyStop 并再次取消 session。每个 EmergencyStop 使用从“剩余 hard deadline”派生的 context，不允许 `context.Background()` 或无界等待。
- EmergencyStop 尝试和后续 manager 等待共同包含在同一个 10 秒 hard deadline 内；单个 adapter 卡住不能延长总 deadline，也不能阻止尝试其它控制器。
- hard deadline 内 manager 未全部退出时，registry 返回包含 probe/task ID 的 shutdown error；调用方禁止继续关闭仍可能被 goroutine 使用的共享 motion、acquisition 和 storage 服务，也不得报告正常退出。
- standalone server 收到该错误后记录 fatal shutdown 并以非零状态结束进程；Wails host 记录 fatal shutdown 后直接结束进程，不执行后续共享服务 Close。进程结束前必须发起所有有界 EmergencyStop 尝试并收集截至 hard deadline 已返回的错误，不等待越过 deadline 的 adapter 调用。
- 超时任务保留最后一个有效 checkpoint；不得在超时路径删除 registry 条目或释放其控制器资源给同进程内的新任务。

## Tech Stack

| 层 | 技术 | 约束 |
|---|---|---|
| 后端 | Go，使用模块当前锁定版本 | 标准库并发原语，不新增第三方依赖 |
| HTTP | `net/http` `ServeMux` | 严格路径解析，薄 handler |
| 前端 | Vue 3 + TypeScript + Pinia + Naive UI | 使用项目 `package.json` 锁定版本 |
| 桌面 | Wails v3 | 使用 Go module/runtime 各自锁定版本；本功能不升级依赖 |
| 实时状态 | HTTP polling | 每 probe 500ms、single-flight、可取消 |
| 测试 | Go `testing`、Vitest | 包含 race、集成和视觉尺寸验收 |

## Commands

```powershell
# Backend
cd projects\WindLabX4\services\api-go
gofmt -l .
go build -buildvcs=false ./...
go vet ./...
go test ./...
go test -race ./internal/usecase ./internal/adapters/storage

# Frontend
cd projects\WindLabX4\apps\desktop-wails\frontend
npm run typecheck
npm run test
npm run build

# Workspace structure
cd <workspace-root>
powershell -ExecutionPolicy Bypass -File scripts\validate-structure.ps1
powershell -ExecutionPolicy Bypass -File scripts\validate-frontend-structure.ps1 -CheckFileSize
```

Wails binding 方法签名发生变化时，额外运行：

```powershell
cd projects\WindLabX4\apps\desktop-wails
go run github.com/wailsapp/wails/v3/cmd/wails3 generate bindings
```

## Project Structure

实际文件由 PLAN 阶段根据影响分析收敛，预期边界如下：

```text
projects/WindLabX4/services/api-go/
├── internal/usecase/
│   ├── traversal.go                         # 保留单 manager 任务语义
│   ├── traversal_manager_registry.go        # registry、资源准入、shutdown
│   └── traversal_manager_registry_test.go
├── internal/ports/
│   └── traversal.go                         # registry/factory 所需接口（如必要）
├── api/
│   ├── server.go                            # probe 路由解析与共享 action handler
│   └── server_dual_traversal_test.go
├── pkg/appcontext/context.go                # desktop composition root
├── internal/bootstrap/bootstrap.go          # bootstrap composition root
└── pkg/apiserver/apiserver.go               # standalone server composition root

projects/WindLabX4/apps/desktop-wails/frontend/src/
├── views/TraversalView.vue                  # single/dual 组合边界和模式开关
├── components/traversal/dual/
│   ├── DualTraversalMain.vue
│   ├── DualStatusBar.vue
│   ├── DualProbeRow.vue
│   ├── DualProbeCompactMonitor.vue
│   └── DualTraversalSettings.vue
├── stores/
│   └── dualTraversalStore.ts                # keyed session，single store 保持兼容
├── api/traversalApi.ts                      # probe-aware client + keyed polling
└── shared/types/traversal.ts                # ProbeId/session API 类型
```

## Code Style

- 业务准入与生命周期编排位于 usecase；HTTP handler 不实现资源冲突规则。
- adapter 的构造只出现在 composition root，registry 不导入 storage/hardware adapter。
- 不在持有 registry/manager mutex 时执行运动、文件、配置或网络 I/O。
- 锁内只完成状态转换和不可变快照；外部 I/O 完成后再短暂加锁提交结果。
- 多错误清理使用 `errors.Join`，不得因第一路失败跳过第二路清理。
- 新增 Go 非测试文件遵守每文件不超过 500 行、函数不超过 50 行。

## Testing Strategy

### 后端单元与并发

1. 两个不同控制器的 manager 可以同时启动并独立推进状态。
2. 相同控制器、空绑定、未知 probe ID 在任何运动或文件 I/O 前被拒绝。
3. 两路资源原子预占，`go test -race` 下并发启动不存在双重占用。
4. 同一 probe 重复启动被拒绝；不同 probe 的 task ID 不发生 store/index 冲突。
5. 两路分别获得不同 CSV writer 和结果日志实例，可同时 Open/Append/Sync/Close。
6. 一路 Stop、error、complete 和 writer 故障不修改另一路状态或端口。
7. `CloseProbe` 在 running/paused/terminal/error 下满足生命周期契约；超时保留条目。
8. registry shutdown 拒绝新任务、尝试关闭所有 manager并聚合错误。
9. probe-scoped 配置独立持久化，重启后分别恢复插值器和配置。
10. 每路 checkpoint 创建、停止保留、正常完成注销、错误恢复均相互隔离。
11. 第一路启动获取全局 workflow lease，第二路启动不重复争抢；一路结束时 lease 仍被持有，最后一路清理后才释放。
12. 每个 probe 有且仅有一个可恢复候选；存在候选时新 Start 被拒绝，错误 task ID 不能恢复或清除该候选。
13. shutdown graceful/hard deadline、控制器 EmergencyStop 和共享服务禁止提前 Close 的路径均有可控 fake 回归测试。
14. 重复完成、旧 generation 完成以及 Start 回滚不会重复减 session 计数或提前释放 lease。
15. checkpoint resume 在打开文件前重新执行资源准入；控制器冲突时 checkpoint 和输出文件保持不变。
16. dual recovery index 与 legacy single active index 可同时存在、独立原子更新并分别恢复。

并发测试使用 channel/barrier 和可控 fake，不依赖固定 `time.Sleep` 判定时序。

### HTTP 集成

1. 完整 probe-scoped action 集合选择正确 manager。
2. 未知 probe、缺失 action、多余路径段、错误 method 返回稳定错误。
3. legacy single 路由的请求/响应回归测试保持通过。
4. 两路并发 start/status/pause/resume/stop/result/checkpoint 不串状态。
5. `go test ./...` 必须包含 `tests/integration`。

### 前端

1. 两个 session 的 config/status/error/checkpoint/interpolator/timer 完全隔离。
2. 一路 start/reset/unmount/失败不取消另一条订阅或清空另一条状态。
3. keyed polling 每 probe 最多一个 in-flight 请求，并丢弃取消后的旧响应。
4. 多设备订阅和共享设备引用计数正确。
5. 有 active/paused session 时模式开关禁用；全部终态并清理后可切换。
6. single 模式继续渲染原 `TraversalMain`，现有 single 测试不修改期望以迁就 dual。
7. 1440x900、1600x900 和 light/dark 下执行人工/截图验收。

### HIL

使用两套真实控制器验证：

- 同时运动、同时采样与独立完成；
- 停止 probe1 时 probe2 控制器不接收 Stop；
- probe1 普通故障时 probe2 继续；
- probe1 位置超差、限位、掉线或 EmergencyStop 时，仅 probe1 控制器收到对应停机命令，probe2 继续；
- 应用退出 hard deadline 路径会分别尝试 EmergencyStop 两个仍活动的控制器；
- 输出文件、checkpoint 和日志可按 probe/task 追溯。

## Boundaries

### Always

- 修改任何生产 symbol 前执行 GitNexus upstream impact，并对 HIGH/CRITICAL 风险先告警。
- 先写能复现隔离失败的测试，再修改 manager、registry、路由或 store。
- 三个 composition root 使用一致的 manager factory 语义。
- 双模式启动前校验控制器绑定，资源冲突不得降级为 warning。
- 每路有状态端口独立实例，所有外部 I/O 错误可见并传播。
- 保持可靠性规格的停止、提交和恢复不变量。

### Ask First

- 支持同一控制器不同轴并行。
- 改变“运行期故障只停故障 probe 自有控制器、应用退出按活动 controller 分别急停”的安全策略。
- 新增后端依赖、批量 status API 或 WebSocket/SSE。
- 修改既有 single HTTP 契约、CSV 列或 checkpoint 版本兼容策略。
- 修改 Wails binding 或共享 `shared/frontend` 组件。

### Never

- 仅通过 `map[probeId]*TraversalManager` 就宣称资源已隔离。
- 两个 manager 共享 CSV writer、结果日志 writer 或可变插值器实例。
- 双模式使用未绑定控制器时操作全部设备的兼容回退。
- 运行中通过删除 registry 条目清理 manager。
- 因 UI 模式切换、组件卸载或一路 Stop 而中断另一条任务。
- 把两路结果写进同一个 CSV、结果日志或 checkpoint。
- 为双模式改写单探针页面布局或改变现有 single API 行为。

## Success Criteria

1. 两套不同控制器可并行运行两个遍历任务，状态和进度独立。
2. 相同/空控制器绑定在任何运动、采集提交或输出文件创建前被拒绝。
3. probe1 暂停、停止、完成、普通错误或输出错误时，probe2 继续且不收到 probe1 的 Stop/Close/reset。
4. 两路配置、插值器、实时计算、轮询、CSV、结果日志、checkpoint 和恢复记录均独立。
5. 双模式完整有状态 API 均按 probe ID 路由；未知 ID 和资源冲突返回明确错误。
6. single 模式的 UI、配置 key 和 legacy HTTP 路由保持兼容。
7. active/paused 时不能切换模式；终态清理后切换不会遗留 goroutine、文件句柄、锁或轮询 timer。
8. Wails 和 standalone server 退出均调用 registry shutdown，并能定位未按时退出的 probe/task。
9. 双模式在 1440x900、1600x900、light/dark、五孔/七孔组合和双 Warning 下可操作且关键控制不被遮挡。
10. `gofmt`、Go build/vet/test/race、frontend typecheck/test/build 和 workspace 结构校验全部通过。
11. 双控制器 HIL 验证证明一路普通停止不会向另一控制器发送停止命令。
12. 第一路到最后一路的全局 workflow lease 引用计数经并发测试验证，不会提前释放或阻止第二路合法启动。
13. 每个 probe 最多一个权威恢复候选，恢复/清除均校验 task ID，legacy single v1/v2 checkpoint 不会被双模式误加载。
14. shutdown 超时先尝试活动控制器 EmergencyStop；仍未退出时不继续关闭共享服务，并以可诊断的非正常退出结束。
15. session token/generation 保证所有完成路径 exactly-once，旧任务回调不能释放新任务资源。
16. 恢复与新启动使用同一原子准入事务，资源冲突发生在 append 文件或运动 I/O 之前。
17. bounded shutdown 在配置的 hard deadline 内返回；挂起的单个 EmergencyStop 不阻止其它控制器尝试，也不无限延长退出。

## Confirmed Decisions

1. 本功能只支持 `probe1`、`probe2`，且必须使用不同运动控制器。
2. DAQ latest-data bus 可只读共享；通道消费和前端订阅按 probe 隔离。
3. 运行或暂停期间禁止切换模式。
4. CSV 需求是独立持续落盘，不新增导出按钮/API。
5. 状态仍按每 probe 500ms 轮询，不新增批量接口。
6. `TraversalView` 是模式组合边界，现有 `TraversalMain` 保持 single 专用。
7. manager 由 composition-root factory 完整装配，registry 不直接依赖 adapter。
8. 运行期运动安全故障只处置故障 probe 的自有控制器，不跨 probe 联停；应用退出 hard deadline 对所有仍活动控制器分别尝试 EmergencyStop。
9. 双模式 checkpoint 使用含 `ProbeID`/controller binding 的 v3 envelope；现有可靠性 v2 字段完整保留，legacy single v1/v2 checkpoint 保持原路由兼容且不自动迁移。

## Open Questions

无。若上述 Confirmed Decisions 需要调整，必须先更新本规格再进入 PLAN。

## Approval Gate

本 v3、实施计划与任务清单须经用户批准后才能修改生产代码；文档修订本身不授权开始实现。
