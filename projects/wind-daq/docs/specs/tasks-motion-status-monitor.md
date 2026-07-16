# Tasks: 位移机构统一状态监视与订阅

> 关联：[spec-motion-status-monitor.md](./spec-motion-status-monitor.md) | [plan-motion-status-monitor.md](./plan-motion-status-monitor.md)
> 状态：Phase 2 任务清单，待人工批准后进入 BUILD
> 日期：2026-07-16

## Phase 1: Driver Hardening

### Task 1: B140 Status() single-flight

**Description:** 在 B140 adapter 增加整轮 `Status()` single-flight，使多个并发调用者共享同一轮 `TD/TS/MG/TP` 命令结果，避免命令数放大。

**Acceptance criteria:**
- [ ] 3 个并发 `Status()` 调用只发送一套 `TD/TS/MG/TP` 命令（fake TCP server 计数验证）
- [ ] single-flight 在 in-flight 完成后立即返回结果给所有等待者
- [ ] in-flight 失败时所有等待者收到同一错误
- [ ] single-flight 不影响 `MoveTo/Jog/Home/Stop` 等命令的并发性
- [ ] 既有 B140 单线程测试无回归

**Verification:**
- [ ] 测试通过：`cd shared\device-sdk\go; $env:GOWORK="off"; go test -race -count=10 ./motion/adapters/hardware -run 'TestB140.*ConcurrentStatus'`
- [ ] 命令计数测试：`go test -v -run 'TestB140StatusCommandBudget'`
- [ ] Build 成功：`go build ./...`

**Dependencies:** None

**Files likely touched:**
- `shared/device-sdk/go/motion/adapters/hardware/b140_motion.go`
- `shared/device-sdk/go/motion/adapters/hardware/b140_motion_test.go`
- `shared/device-sdk/go/motion/adapters/hardware/b140_singleflight_test.go`（新增）

**Estimated scope:** S（1-2 文件）

---

### Task 2: B140 Connection Priority Coordinator

**Description:** 在 B140 adapter 的 `connMu` 之上实现 Connection Priority Coordinator，满足 Decision 16 六条不变量（抢占语义、防插队、可取消、防饥饿、可观测、不替换驱动串行锁）。

**Acceptance criteria:**
- [ ] Critical（Stop/EStop）等待时，当前 Normal（Status）/High（补偿 TP/命令）单次 I/O 完成后立即让出给 Critical
- [ ] Critical 等待期间到达的 Normal/High 不得在 Critical 完成前抢占
- [ ] Critical 之间 FIFO；High 之间 FIFO，不抢占 Normal
- [ ] Critical 等待者响应 context 取消；取消后驱动不继续发送该 Stop/EStop 命令帧
- [ ] Critical 等待超过 600ms 时，放弃当前 Normal I/O（通过 SetDeadline 强制返回）让 Critical 推进
- [ ] 协调器暴露指标：等待者数量（Critical/High/Normal 分别统计）、各优先级等待时长 p50/p95、抢占次数、放弃 Normal I/O 次数
- [ ] 协调器在 `connMu` 之上，不替换 `connMu`；协议一致性仍由 `connMu` 保证

**Verification:**
- [ ] 测试通过：`go test -race -count=10 ./motion/adapters/hardware -run 'TestB140PriorityCoordinator'`
- [ ] 不变量测试矩阵：Critical/High/Normal × 多种到达顺序 × context 取消 × 饥饿恢复
- [ ] Build 成功：`go build ./...`

**Dependencies:** Task 1

**Files likely touched:**
- `shared/device-sdk/go/motion/adapters/hardware/b140_motion.go`
- `shared/device-sdk/go/motion/adapters/hardware/priority_coordinator.go`（新增，可能 B140/WTNMC4A 共用）
- `shared/device-sdk/go/motion/adapters/hardware/priority_coordinator_test.go`（新增）

**Estimated scope:** M（3-5 文件）

---

### Task 3: WTNMC4A Connection Priority Coordinator

**Description:** 在 WTNMC4A adapter 的 `ioMu` 之上实现 Connection Priority Coordinator，复用 Task 2 的协调器类型（若可共用）或独立实现，满足 Decision 16 六条不变量。

**Acceptance criteria:**
- [ ] 保留既有 `ioMu`、`statusQueryMu`、statusQueryFlight single-flight（无回归）
- [ ] Critical（Stop/EStop）等待时，当前 Normal/High 单次 I/O 完成后立即让出给 Critical
- [ ] Critical 等待超过 600ms 时，放弃当前 Normal I/O（通过 SDK 200ms timeout 让出）让 Critical 推进
- [ ] 协调器在 `ioMu` 之上，不替换 `ioMu`
- [ ] 既有 WTNMC4A single-flight 测试无回归

**Verification:**
- [ ] 测试通过：`go test -race -count=10 ./motion/adapters/hardware -run 'TestWTNMC4A.*PriorityCoordinator|TestWTNMC4A.*Concurrent'`
- [ ] 既有 single-flight 测试：`go test -v -run 'TestWTNMC4AStatusSingleFlight'`
- [ ] Build 成功：`go build ./...`

**Dependencies:** Task 2（若共用协调器类型）

**Files likely touched:**
- `shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion.go`
- `shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion_test.go`
- `shared/device-sdk/go/motion/adapters/hardware/priority_coordinator.go`（若共用）

**Estimated scope:** S-M（2-4 文件）

---

## Phase 2: Shared Monitor

### Task 4: Monitor 数据模型 + FreshnessPolicy 接口

**Description:** 在 `shared/motion-control/go/monitor/` 新建包，定义 `ControllerStatusSnapshot`、`StatusSnapshot`、`Freshness`、`FreshnessPolicy` 接口、`ErrGenerationChanged`、`CommandKind` 类型，按 spec Data Model 章节。

**Acceptance criteria:**
- [ ] `ControllerStatusSnapshot` 含 `ControllerID/Generation/Sequence/AttemptedAt/SucceededAt/ValidUntil/Status/Err` 字段
- [ ] `StatusSnapshot` 含 `Sequence/PublishedAt/Controllers` 字段
- [ ] `Freshness` 结构体含 `Age time.Duration` 与 `IsStale bool`，不固化在快照中
- [ ] `FreshnessPolicy` 接口含 `Freshness(now time.Time, snap ControllerStatusSnapshot) Freshness` 方法
- [ ] `ErrGenerationChanged` 结构体含 `ControllerID/OldGen/NewGen` 字段，实现 `error` 接口
- [ ] `CommandKind` 枚举含 `CmdKindMove/CmdKindStop/CmdKindConfig`
- [ ] 所有导出类型有中文注释解释时序/安全原因
- [ ] 不引入第三方依赖

**Verification:**
- [ ] 测试通过：`go test ./monitor/...`
- [ ] Vet 通过：`go vet ./monitor/...`
- [ ] Build 成功：`go build ./...`

**Dependencies:** None

**Files likely touched:**
- `shared/motion-control/go/monitor/snapshot.go`（新增）
- `shared/motion-control/go/monitor/errors.go`（新增）
- `shared/motion-control/go/monitor/snapshot_test.go`（新增）

**Estimated scope:** S（1-3 文件）

---

### Task 5: MotionStatusMonitor 核心

**Description:** 实现 `MotionStatusMonitor` 与 `StatusMonitor` 接口，包含 `Latest/LatestController/Subscribe/RequestRefresh/NotifyCommandExecuted` 五个方法；每台已连接控制器唯一轮询 goroutine、不可变快照、latest-only 订阅、自适应频率（运动/空闲/命令快速窗口）。

**Acceptance criteria:**
- [ ] 多个并发消费者只触发一轮 controller `Status()`
- [ ] `Latest()` 不触发硬件读取，返回深拷贝
- [ ] `Subscribe()` 首次订阅立即投递当前快照；channel 容量 1，latest-only
- [ ] 慢订阅者不阻塞采集循环（新快照覆盖未消费旧快照）
- [ ] 发布与订阅取消并发不会 send-on-closed 或数据竞争
- [ ] `RequestRefresh(controllerID)` 非阻塞、幂等、可合并；不触发 2s 快速窗口
- [ ] `NotifyCommandExecuted(controllerID, CmdKindMove)` 触发 2s 快速窗口 + 一轮额外采集
- [ ] `NotifyCommandExecuted(controllerID, CmdKindStop)` 仅触发单轮 refresh
- [ ] `NotifyCommandExecuted(controllerID, CmdKindConfig)` 触发 generation 重置 + 单轮 refresh
- [ ] 运动/空闲频率切换正确（moving=100ms, idle=500ms, fast-grace=100ms 持续 2s）
- [ ] Disconnect/Shutdown 无 goroutine 泄漏（测试验证）
- [ ] 不使用 `time.Sleep` 断言轮询时序；注入 clock/ticker

**Verification:**
- [ ] 测试通过：`go test -race -count=10 ./monitor/... -run 'TestMonitor'`
- [ ] Goroutine 泄漏测试：`go test -run 'TestMonitorNoLeak'`
- [ ] Vet 通过：`go vet ./monitor/...`

**Dependencies:** Task 4

**Files likely touched:**
- `shared/motion-control/go/monitor/monitor.go`（新增）
- `shared/motion-control/go/monitor/monitor_test.go`（新增）
- `shared/motion-control/go/monitor/fake_clock.go`（新增，测试用）

**Estimated scope:** L（5-7 文件，但同包内聚）

---

### Task 6: WaitNext + FreshnessPolicy 默认实现 + Generation 重连语义

**Description:** 实现 `WaitNext` 含 TOCTOU 语义（调用瞬间已有更大 sequence 立即返回）与 `ErrGenerationChanged` 语义；实现默认 `FreshnessPolicy`（moving/idle 模式切换、ValidUntil 计算）；实现 Generation/Sequence 重连行为（Disconnect 递增 generation、Sequence 重置为 0、清空旧缓存、在途 WaitNext 返回 ErrGenerationChanged）。

**Acceptance criteria:**
- [ ] `WaitNext(ctx, id, afterSeq)` 在调用瞬间已有更大 sequence（同 generation）时立即返回，不等待下一轮
- [ ] `WaitNext` 检查 sequence 与注册 waiter 在同一临界区，禁止丢失唤醒
- [ ] context 取消后立即返回 `ctx.Err()`
- [ ] generation 切换时 `WaitNext` 返回 `ErrGenerationChanged{OldGen, NewGen}`，不返回新 generation 快照
- [ ] 首帧语义：重连后尚无首帧时 WaitNext 阻塞，不立即返回零值快照
- [ ] Disconnect 后 Generation 单调递增；Sequence 重置为 0；首帧 Sequence=1
- [ ] 重连完成时旧 generation 的快照缓存被清空
- [ ] `ApplyConfig` 触发 generation 重置，行为等同 Disconnect+Connect
- [ ] 旧 generation 的 Sequence 不被新 generation 复用
- [ ] `FreshnessPolicy.Freshness(now, snap)` 计算 `Age = now - SucceededAt`；`IsStale = now > ValidUntil`
- [ ] 模式切换（moving/idle）时 staleThreshold 正确切换
- [ ] Err != nil 但 SucceededAt 仍新鲜时 IsStale=false
- [ ] 快照结构体中不存在静态 `IsStale` 字段

**Verification:**
- [ ] 测试通过：`go test -race -count=10 ./monitor/... -run 'TestWaitNext|TestFreshnessPolicy|TestGeneration'`
- [ ] TOCTOU 测试：`go test -v -run 'TestWaitNextTOCTOU'`
- [ ] Generation 重连测试：`go test -v -run 'TestGenerationChanged'`
- [ ] Vet 通过：`go vet ./monitor/...`

**Dependencies:** Task 5

**Files likely touched:**
- `shared/motion-control/go/monitor/monitor.go`
- `shared/motion-control/go/monitor/freshness.go`（新增）
- `shared/motion-control/go/monitor/freshness_test.go`（新增）
- `shared/motion-control/go/monitor/monitor_test.go`

**Estimated scope:** M（3-5 文件）

---

## Phase 3: Manager + Safety

### Task 7: MotionManager 持有 monitor

**Description:** 改造 `shared/motion-control/go/manager/motion_manager.go`，使 MotionManager 创建并持有 monitor；构造时注入 clock、monitor config 和 controller factory；Connect 成功后注册/启动该控制器的采集循环；Disconnect/ApplyConfig 递增 generation；Disconnect 顺序固定为"标记 closing + 递增 generation → cancel 采集循环 → 有界等待在途采集退出 → 调用驱动 Disconnect → 发布断开快照"；Move/Jog/Home/Stop/EStop/ApplyConfig 成功或失败后调用 `NotifyCommandExecuted`；原子 ownership 切换；`Status/StatusAll` 改为投影 monitor 最新快照。

**Acceptance criteria:**
- [ ] MotionManager 构造时创建 monitor，注入 clock 与 config
- [ ] Connect 成功后注册 controller 并启动采集循环
- [ ] Disconnect 顺序固定：closing 标记 + generation 递增 → cancel 采集 → 有界等待 → 驱动 Disconnect → 发布断开快照
- [ ] ApplyConfig 触发 generation 重置 + 单轮 refresh
- [ ] Move/Jog/Home 后调用 `NotifyCommandExecuted(id, CmdKindMove)`
- [ ] Stop/EStop 后调用 `NotifyCommandExecuted(id, CmdKindStop)`
- [ ] ApplyConfig 后调用 `NotifyCommandExecuted(id, CmdKindConfig)`
- [ ] `Status(id)` / `StatusAll()` 改为读取 monitor `Latest()` / `LatestController()`，不触发硬件 I/O
- [ ] 原子 ownership 切换：`unregister` + `register` 在同一锁内，无中间窗口
- [ ] 启动期发现重复注册 fail-fast 并拒绝启动 monitor
- [ ] manager 不持有自身 map/profile 锁等待驱动 I/O、monitor goroutine 或订阅者
- [ ] 应用关闭时 cancel 根 context，等待所有 monitor goroutine 退出
- [ ] 独立 motion 子进程不连接真实控制器，不启动 monitor

**Verification:**
- [ ] 测试通过：`go test -race -count=10 ./manager/...`
- [ ] 集成测试：`go test -v -run 'TestMotionManagerMonitorIntegration'`
- [ ] 既有 manager 测试无回归
- [ ] Vet 通过：`go vet ./manager/...`

**Dependencies:** Task 5, Task 6

**Files likely touched:**
- `shared/motion-control/go/manager/motion_manager.go`
- `shared/motion-control/go/manager/motion_manager_test.go`
- `shared/motion-control/go/manager/ownership.go`（新增，原子 ownership 切换）

**Estimated scope:** M-L（3-6 文件）

---

### Task 8: JudgeArrival 四层门禁

**Description:** 在 shared motion safety 包（建议 `shared/motion-control/go/safety/`）实现 `JudgeArrival` 函数，按 spec Motion Safety Integration 章节的伪代码实现四层门禁：Generation → Freshness → Sequence → 业务字段。

**Acceptance criteria:**
- [ ] 第 1 层 Generation：`ErrGenerationChanged` 返回 `JudgeGenerationChanged`，业务必须 abort
- [ ] 第 2 层 Freshness：`snap.Freshness.IsStale` 返回 `JudgeStatusUnavailable`，不使用旧 `Moving=false`
- [ ] 第 3 层 Sequence：`snap.Sequence <= awaitedSeq` 返回 `JudgeWaitMore`
- [ ] 第 4 层业务字段：`!axis.Moving && math.Abs(axis.Position-target) <= arrivalTolerance` 返回 `JudgeArrived`
- [ ] `awaitedSeq=0` 时业务必须先调 `LatestController` 获取基准，再调 `WaitNext`
- [ ] `ErrGenerationChanged` 后禁止 fallthrough 继续用旧 sequence 等待
- [ ] 安全判定基于目标控制器单个 `ControllerStatusSnapshot.Sequence + Generation`
- [ ] Moving=true 且已进入到位容差：继续等待，不触发 NoProgress
- [ ] Moving=true 且容差外长时间无进展：触发 NoProgress
- [ ] 校准到位后仍按配置执行 `DwellTimeMs`；遍历仍执行 fixed/adaptive stabilization

**Verification:**
- [ ] 测试通过：`go test -race ./safety/... -run 'TestJudgeArrival'`
- [ ] 四层门禁单测：`go test -v -run 'TestJudgeArrival(Layer|Generation|Freshness|Sequence|Business)'`
- [ ] Vet 通过：`go vet ./safety/...`

**Dependencies:** Task 4, Task 6

**Files likely touched:**
- `shared/motion-control/go/safety/judge_arrival.go`（新增）
- `shared/motion-control/go/safety/judge_arrival_test.go`（新增）

**Estimated scope:** S（2 文件）

---

### Task 9: 急停不可信协议（后端）

**Description:** 在 MotionManager 实现 Decision 15 急停不可信协议：Stop/EStop 排队或下发耗时超过驱动声明阈值时进入急停不可信模式；命令冻结（禁用 Move/Jog/Home/ApplyConfig，允许 Stop/EStop/Reset/Disconnect）；告警锁存（不因状态恢复自动清除）；SOP 来源从 `motion_profiles` 读取；恢复条件三重门禁（Disconnect+重连 + 首帧新鲜快照 + 操作员显式复位）；CRITICAL 日志含完整上下文；UI 不显示"急停已下发成功"。

**Acceptance criteria:**
- [ ] Stop/EStop 排队或下发耗时超过阈值（B140 600ms / WTNMC4A 600ms 目标）时进入急停不可信模式
- [ ] 命令冻结：Move/Jog/Home/ApplyConfig 被拒绝并返回错误；Stop/EStop/Reset/Disconnect 仍可用
- [ ] 告警锁存：状态恢复后不自动清除，必须操作员显式复位
- [ ] SOP 来源：从 `motion_profiles` 的 `emergency_sop.physical_stop_location` 与 `emergency_sop.power_off_procedure` 读取
- [ ] 未配置 `emergency_sop` 的 profile 启动运动任务时返回错误
- [ ] 恢复条件三重门禁：(a) Disconnect+重连成功 (b) monitor 首帧 SucceededAt 新鲜 (c) 操作员显式"复位"动作完成
- [ ] 三重门禁任一缺失时运动命令保持禁用
- [ ] CRITICAL 日志包含 controller ID、排队耗时、是否已确认、是否已复位等字段
- [ ] 暴露急停不可信状态给前端：新增 `GET /api/motion/emergency-status` + Wails 事件 `motion:emergency-untrusted`
- [ ] 独立窗口通过 HTTP 轮询 `/api/motion/emergency-status`（与 status 同频率）

**Verification:**
- [ ] 测试通过：`go test -race ./manager/... -run 'TestEmergencyStopUntrusted'`
- [ ] 命令冻结测试：`go test -v -run 'TestEmergencyStopCommandFreeze'`
- [ ] 恢复条件测试：`go test -v -run 'TestEmergencyStopRecovery'`
- [ ] Vet 通过：`go vet ./manager/...`

**Dependencies:** Task 7, Task 8

**Files likely touched:**
- `shared/motion-control/go/manager/emergency_stop.go`（新增）
- `shared/motion-control/go/manager/emergency_stop_test.go`（新增）
- `shared/motion-control/go/manager/motion_manager.go`（集成调用点）
- `shared/motion-control/go/profile/profile.go`（新增 `emergency_sop` 字段）
- `shared/motion-control/go/httpapi/routes.go`（新增 `/api/motion/emergency-status` 端点）

**Estimated scope:** M-L（5-7 文件）

---

## Phase 4: Project Port + API

### Task 10: wind-daq MotionStatusReader 端口 + adapter

**Description:** 在 wind-daq `services/api-go/internal/ports/motion.go` 新增 `MotionStatusReader` 接口与 `MotionControllerSnapshot` 类型（含 `Generation/Sequence/AttemptedAt/SucceededAt/ValidUntil/Status/Err/Freshness`）；在 `adapters/motion/wrapper.go` 实现 adapter，将 shared monitor 快照转换为项目类型，保留 Generation 与 ErrGenerationChanged，调用 FreshnessPolicy 注入 Freshness 字段。

**Acceptance criteria:**
- [ ] `MotionStatusReader` 接口含 `LatestController(controllerID) (MotionControllerSnapshot, bool)`、`WaitNext(ctx, controllerID, afterSeq) (MotionControllerSnapshot, error)`、`FreshnessPolicy() FreshnessPolicy`
- [ ] `MotionControllerSnapshot` 含 Generation/Sequence/AttemptedAt/SucceededAt/ValidUntil/Status/Err/Freshness 字段
- [ ] adapter 在 `LatestController` / `WaitNext` 返回前调用 `FreshnessPolicy.Freshness(now, snap)` 填充 `Freshness`
- [ ] Generation 从 shared `ControllerStatusSnapshot.Generation` 透传，禁止丢失
- [ ] `WaitNext` 透传 monitor 的 `ErrGenerationChanged`，包装为项目侧 `ErrGenerationChanged`，保留 OldGen/NewGen
- [ ] HTTP/Wails 兼容路径把快照投影为现有 `[]ControllerStatus`（仅业务字段）
- [ ] 校准和遍历必须使用 `MotionStatusReader` 而非兼容路径

**Verification:**
- [ ] 测试通过：`cd projects\wind-daq\services\api-go; go test ./internal/adapters/motion/... ./internal/ports/...`
- [ ] Generation 透传测试：`go test -v -run 'TestMotionStatusReaderGeneration'`
- [ ] Freshness 注入测试：`go test -v -run 'TestMotionStatusReaderFreshness'`
- [ ] Build 成功：`go build -buildvcs=false ./...`

**Dependencies:** Task 6, Task 7

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/ports/motion.go`
- `projects/wind-daq/services/api-go/internal/adapters/motion/wrapper.go`
- `projects/wind-daq/services/api-go/internal/adapters/motion/wrapper_test.go`

**Estimated scope:** S-M（2-4 文件）

---

### Task 11: /api/motion/status + Wails MotionGetStatus 读取 monitor 缓存

**Description:** 改造 `shared/motion-control/go/httpapi/routes.go` 的 `GET /api/motion/status`，改为读取 monitor `Latest()`，不触发硬件查询；保持 `[]ControllerStatus` 响应兼容；monitor 故障时返回 HTTP 503；Wails `MotionGetStatus` 同样读取缓存。

**Acceptance criteria:**
- [ ] `/api/motion/status` 改为读取 `monitor.Latest()`，不触发硬件查询
- [ ] 响应结构保持 `[]ControllerStatus` 兼容（不破坏现有消费者）
- [ ] monitor 故障或未启动时返回 HTTP 503 + 错误体
- [ ] 高频并发请求不增加 fake controller `Status()` 调用次数
- [ ] Wails `MotionGetStatus` 读取 monitor 缓存
- [ ] 诊断元数据（sequence/generation/freshness）走独立端点或 header（首期不强制）

**Verification:**
- [ ] 测试通过：`go test ./httpapi/... -run 'TestStatusEndpointCache'`
- [ ] 并发不触发硬件测试：`go test -v -run 'TestStatusEndpointNoHardwareTrigger'`
- [ ] HTTP 503 测试：`go test -v -run 'TestStatusEndpoint503'`
- [ ] Build 成功：`go build ./...`

**Dependencies:** Task 7, Task 10

**Files likely touched:**
- `shared/motion-control/go/httpapi/routes.go`
- `shared/motion-control/go/httpapi/routes_test.go`
- `projects/wind-daq/apps/desktop-wails/backend/motion_bindings.go`（Wails MotionGetStatus）

**Estimated scope:** S（2-3 文件）

---

## Phase 5: Use Case Migration

### Task 12: 校准 usecase 迁移到 WaitNext + JudgeArrival

**Description:** 改造 `projects/wind-daq/services/api-go/internal/usecase/calibration.go` 的 `fallbackRuntime.WaitForMotionComplete`（L1082-1165），从 100ms `motionCompletePoll` ticker + `StatusAll` 迁移为 `MotionStatusReader.WaitNext` + `JudgeArrival`；处理 `ErrGenerationChanged` 时 abort 当前点；保持既有到位/NoProgress/StatusUnavailable 语义不变。

**Acceptance criteria:**
- [ ] `fallbackRuntime.WaitForMotionComplete` 使用 `MotionStatusReader.WaitNext` 替代 100ms ticker + `StatusAll`
- [ ] 到位判定使用 `JudgeArrival` 四层门禁
- [ ] `ErrGenerationChanged` 时 abort 当前点并通知上层
- [ ] `awaitedSeq=0` 时先调 `LatestController` 获取基准
- [ ] 既有 NoProgress 到位容差修复测试无回归
- [ ] 既有 RR0 Moving 修复测试无回归
- [ ] DwellTimeMs 仍按配置执行
- [ ] 校准配置 JSON 不发生破坏性变化

**Verification:**
- [ ] 测试通过：`cd projects\wind-daq\services\api-go; go test ./internal/usecase/... -run 'TestCalibration.*Motion'`
- [ ] 回归测试：`go test -v -run 'TestCalibration(NoProgress|ArrivalTolerance|RR0Moving)'`
- [ ] Build 成功：`go build -buildvcs=false ./...`

**Dependencies:** Task 8, Task 10

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/usecase/calibration.go`
- `projects/wind-daq/services/api-go/internal/usecase/calibration_test.go`

**Estimated scope:** M（2-3 文件）

---

### Task 13: 遍历 usecase 迁移到 WaitNext + JudgeArrival

**Description:** 改造 `projects/wind-daq/services/api-go/internal/usecase/traversal_acquisition.go` 的 `waitForMotionComplete`（L1056-1155）与 `recheckMotionSafety`（L854-904），从 100ms ticker + `StatusAll` 迁移为 `WaitNext` + `JudgeArrival`；处理 `ErrGenerationChanged` 时 abort 当前点；保持 fixed/adaptive stabilization 语义不变。

**Acceptance criteria:**
- [ ] `waitForMotionComplete` 使用 `MotionStatusReader.WaitNext` 替代 100ms ticker + `StatusAll`
- [ ] `recheckMotionSafety` 使用 `MotionStatusReader.WaitNext` 替代周期 `StatusAll`
- [ ] 到位判定使用 `JudgeArrival` 四层门禁
- [ ] `ErrGenerationChanged` 时 abort 当前点并通知上层
- [ ] fixed/adaptive stabilization 语义不变
- [ ] 既有遍历回归测试无回归
- [ ] 遍历配置 JSON 不发生破坏性变化

**Verification:**
- [ ] 测试通过：`go test ./internal/usecase/... -run 'TestTraversal.*Motion'`
- [ ] 回归测试：`go test -v -run 'TestTraversal(Stabilization|MotionComplete|RecheckSafety)'`
- [ ] Build 成功：`go build -buildvcs=false ./...`

**Dependencies:** Task 8, Task 10, Task 12（校准先迁移，复用模式）

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/usecase/traversal_acquisition.go`
- `projects/wind-daq/services/api-go/internal/usecase/traversal_acquisition_test.go`

**Estimated scope:** M（2-3 文件）

---

## Phase 6: Frontend

### Task 14: motionApi/motionStore 单一状态源

**Description:** 改造 `projects/wind-daq/apps/desktop-wails/frontend/src/api/motionApi.ts` 与 `stores/motionStore.ts`，确保每个进程内最多保持一条状态轮询/订阅链；`attachStatusListener()` 只注册观察者，不创建新的后端轮询链；`refreshStatus()` 仅用于首次加载/显式刷新。

**Acceptance criteria:**
- [ ] `motionApi` 每个进程内最多保持一条状态轮询/订阅链
- [ ] `motionStore.attachStatusListener()` 只注册观察者，不创建新的后端轮询链
- [ ] `motionStore.refreshStatus()` 读取后端缓存，仅用于首次加载/显式刷新
- [ ] 多个 `attachStatusListener` 只建立一个底层状态源
- [ ] 最后一个 listener 取消后正确停止前端轮询链
- [ ] 不使用 `any`、`@ts-ignore`

**Verification:**
- [ ] 测试通过：`cd projects\wind-daq\apps\desktop-wails\frontend; npm run test -- --grep 'motionApi|motionStore'`
- [ ] Typecheck 通过：`npm run typecheck`
- [ ] Build 成功：`npm run build`

**Dependencies:** Task 11

**Files likely touched:**
- `projects/wind-daq/apps/desktop-wails/frontend/src/api/motionApi.ts`
- `projects/wind-daq/apps/desktop-wails/frontend/src/stores/motionStore.ts`
- `projects/wind-daq/apps/desktop-wails/frontend/src/api/motionApi.test.ts`（新增）
- `projects/wind-daq/apps/desktop-wails/frontend/src/stores/motionStore.test.ts`（新增）

**Estimated scope:** S-M（2-4 文件）

---

### Task 15: 删除 3 类校准 Main 的 300ms motionStatusPollTimer

**Description:** 删除 `FiveHoleMain.vue`（L78/L113-115）、`ThreeHoleMain.vue`（L59/L344-346）、`TotalPressureMain.vue`（L55/L378-380）的 300ms `motionStatusPollTimer`；组件只观察 `motionStore.statusList`。

**Acceptance criteria:**
- [ ] 五孔、三孔、总压 Main 不再创建 `motionStatusPollTimer`
- [ ] 组件仍能从 motionStore 显示位置、Moving、限位
- [ ] 总温 Main 本无 timer，保持不变
- [ ] 组件 unmount 时 cleanup 行为不变（仅 cleanupSubscriptions，不调 reset）

**Verification:**
- [ ] 测试通过：`npm run test -- --grep 'CalibrationMain.*Motion'`
- [ ] Typecheck 通过：`npm run typecheck`
- [ ] Build 成功：`npm run build`
- [ ] 手动检查：四类 Main 切换/卸载后无 timer 残留

**Dependencies:** Task 14

**Files likely touched:**
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/calibration/five-hole/FiveHoleMain.vue`
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/calibration/three-hole/ThreeHoleMain.vue`
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/calibration/total-pressure/TotalPressureMain.vue`

**Estimated scope:** S（3 文件，但每个仅删除几行）

---

### Task 16: 独立运动窗口失联降级协议

**Description:** 在 `motionApi.ts` 与新建的 `StandaloneDisconnectBanner.vue` 组件实现失联降级：HTTP 请求超时（1s）显示"主进程连接丢失，状态不可信"横幅并禁用运动命令输入；连续失败 3 次锁定 UI 为只读模式；失联期间不使用最后成功响应的快照作为实时状态，显式标注"数据时间：YYYY-MM-DD HH:MM:SS（已过期）"；"重连"按钮触发 HTTP 请求；主进程返回 HTTP 503 时等同失联处理。

**Acceptance criteria:**
- [ ] HTTP 请求超时（1s）后 UI 显示"主进程连接丢失，状态不可信"横幅
- [ ] 失联期间禁用所有运动命令输入
- [ ] 连续失败 3 次锁定 UI 为只读模式，仅允许"重连"按钮
- [ ] 失联期间前端不使用最后成功响应的快照作为实时状态
- [ ] 显式标注"数据时间：YYYY-MM-DD HH:MM:SS（已过期）"
- [ ] 失联期间发送运动命令被立即拒绝并提示"主进程失联，命令不可发送"
- [ ] "重连"按钮触发 HTTP 请求；成功后清空失败计数并恢复，失败则继续锁定
- [ ] 重连成功后横幅变绿"已重连"2s 后消失，刷新当前快照
- [ ] 主进程返回 HTTP 503 时独立窗口识别并等同失联处理
- [ ] 失联状态通过 IPC 通知主进程（若 Wails 模式支持）

**Verification:**
- [ ] 测试通过：`npm run test -- --grep 'StandaloneDisconnect'`
- [ ] Typecheck 通过：`npm run typecheck`
- [ ] Build 成功：`npm run build`

**Dependencies:** Task 14

**Files likely touched:**
- `projects/wind-daq/apps/desktop-wails/frontend/src/api/motionApi.ts`
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/motion/StandaloneDisconnectBanner.vue`（新增）
- `projects/wind-daq/apps/desktop-wails/frontend/src/composables/useStandaloneDisconnect.ts`（新增）
- `projects/wind-daq/apps/desktop-wails/frontend/src/api/motionApi.test.ts`

**Estimated scope:** M（3-5 文件）

---

### Task 17: 急停不可信 UI

**Description:** 新建 `EmergencyStopUntrustedBanner.vue` 组件，订阅后端 `motion:emergency-untrusted` 事件（Wails 模式）或轮询 `/api/motion/emergency-status`（独立窗口）；显示红色横幅，含物理急停按钮位置（从 `emergency_sop.physical_stop_location` 读取）、断电程序、联系维护人员提示；"已物理急停"确认按钮只记录操作员已知悉事件并打日志，不关闭告警或恢复运动命令；"复位"按钮触发后端三重门禁恢复流程；运动命令在急停不可信模式下被禁用（前端禁用 + 后端拒绝）。

**Acceptance criteria:**
- [ ] 红色横幅显示，含 SOP 来源（physical_stop_location / power_off_procedure）
- [ ] 告警锁存，不因状态恢复自动清除
- [ ] "已物理急停"按钮点击后只记录操作员已知悉事件并打日志
- [ ] 按钮不命名为"恢复"或"重置"
- [ ] 按钮不关闭告警或恢复运动命令
- [ ] "复位"按钮触发后端恢复流程（三重门禁）
- [ ] 运动命令在急停不可信模式下被前端禁用（Move/Jog/Home/ApplyConfig 按钮 disabled）
- [ ] 后端拒绝命令时前端显示错误提示
- [ ] UI 不显示"急停已下发成功"
- [ ] Wails 事件订阅与 HTTP 轮询两模式都支持

**Verification:**
- [ ] 测试通过：`npm run test -- --grep 'EmergencyStopUntrusted'`
- [ ] Typecheck 通过：`npm run typecheck`
- [ ] Build 成功：`npm run build`
- [ ] 手动检查：确认按钮不关闭告警；复位三重门禁

**Dependencies:** Task 9（后端协议）, Task 14（前端基础）

**Files likely touched:**
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/motion/EmergencyStopUntrustedBanner.vue`（新增）
- `projects/wind-daq/apps/desktop-wails/frontend/src/composables/useEmergencyStopUntrusted.ts`（新增）
- `projects/wind-daq/apps/desktop-wails/frontend/src/stores/motionStore.ts`（订阅事件）
- `projects/wind-daq/apps/desktop-wails/frontend/src/api/motionApi.ts`（HTTP 轮询 /api/motion/emergency-status）

**Estimated scope:** M（3-5 文件）

---

## Phase 7: Real Hardware + Cross-Project

### Task 18: motion-controller 项目验证

**Description:** motion-controller 项目完全复用 `shared/motion-control/go/{manager,httpapi,profile}`，仅类型别名（[usecase/motion.go](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/motion-controller/services/api-go/internal/usecase/motion.go)）；验证 build/test/vet 全绿；如有独立 UI 需求（急停不可信横幅、独立窗口失联），增加最小迁移任务。

**Acceptance criteria:**
- [ ] motion-controller 后端 `go build` + `go test` + `go vet` 全绿
- [ ] motion-controller desktop shell `go build` + `go test` + `go vet` 全绿
- [ ] motion-controller 前端 `npm run typecheck` + `npm run build` 全绿
- [ ] 确认 motion-controller 是否需要急停不可信 UI（若有独立运动窗口则需）
- [ ] 若需要 UI，新增最小迁移任务（沿用 wind-daq Task 17 的组件）

**Verification:**
- [ ] `cd projects\motion-controller\services\api-go; go test ./...; go vet ./...; go build -buildvcs=false ./...`
- [ ] `cd projects\motion-controller\apps\desktop-wails; go test ./...; go vet ./...; go build -buildvcs=false ./...`
- [ ] `cd projects\motion-controller\apps\desktop-wails\frontend; npm run typecheck; npm run build`

**Dependencies:** Task 9, Task 11, Task 17

**Files likely touched:**
- 极少（验证为主）；如需独立 UI，新增 1-2 个 Vue 组件

**Estimated scope:** XS-S（0-2 文件）

---

### Task 19: WTNMC4A 192.168.3.141 只读测试

**Description:** 执行 spec Commands 章节的 WTNMC4A 实机只读测试，预热 10 轮、测量 300 轮；3 并发消费者测量 100 批；同时报告单调用 p50/p95、3 并发批次 p50/p95、排队增量比；区分底层单轮真实耗时与排队耗时；禁止发送任何运动命令。

**Acceptance criteria:**
- [ ] 单线程稳定性测试通过（300 轮无错误）
- [ ] 3 并发状态共享测试通过
- [ ] 单调用 p95 < 500ms（若超过，报告原始 query duration 而非误判为排队）
- [ ] 3 并发批次 p95 排队增量 ≤ 单调用 p95 × 25%
- [ ] 不得出现因重复串行轮询造成的 3 倍调用量
- [ ] 同时报告单调用 p50/p95 与排队增量比
- [ ] 测试输出明确标注"未发送运动命令"

**Verification:**
- [ ] 测试通过：`cd shared\device-sdk\go; $env:GOWORK="off"; $env:WTNMC4A_READONLY_IP="192.168.3.141"; $env:WTNMC4A_READONLY_ITERATIONS="300"; go test ./motion/adapters/hardware -run '^TestWTNMC4AReadOnly(Stability|ConcurrentStatus)$' -count=1 -v -timeout 120s`
- [ ] 测试报告归档（含 p50/p95/排队增量比）

**Dependencies:** Task 3, Task 7

**Files likely touched:**
- `shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_readonly_test.go`（可能新增或扩展）

**Estimated scope:** S（1-2 文件）

---

### Task 20: WTNMC4A DLL 200ms timeout 故障注入测试

**Description:** 通过 mock DLL 或实机故障注入，模拟 DLL 调用超过 200ms timeout deadline，验证是否触发 CRITICAL 日志和 Decision 15 急停不可信协议（命令冻结、告警锁存、恢复条件）；决定 600ms 是否可从目标提升为硬保证。

**Acceptance criteria:**
- [ ] mock DLL 调用超过 200ms timeout 时触发 CRITICAL 日志
- [ ] 触发 Decision 15 急停不可信协议（命令冻结、告警锁存）
- [ ] 恢复条件三重门禁生效
- [ ] UI 不显示"急停已下发成功"
- [ ] 验证报告：DLL 是否严格可中断
- [ ] 若验证通过，600ms 可从目标提升为硬保证；否则保持目标/告警阈值
- [ ] 若实机故障注入不可行，用 mock DLL 覆盖（明确标注 mock 环境）

**Verification:**
- [ ] mock DLL 测试通过：`go test -race ./motion/adapters/hardware -run 'TestWTNMC4AFaultInjection'`
- [ ] 实机故障注入测试报告（若执行）
- [ ] ADR 记录 600ms 是否为硬保证

**Dependencies:** Task 9, Task 19

**Files likely touched:**
- `shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_fault_injection_test.go`（新增）
- `shared/device-sdk/go/motion/adapters/hardware/mock_dll.go`（若需 mock）

**Estimated scope:** S-M（2-3 文件）

---

## Phase 8: Cleanup

### Task 21: 删除/弃用 events.StartStatusPoller

**Description:** `events.StartStatusPoller` 已在 wind-daq 关闭（[app.go L141-144](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/apps/desktop-wails/backend/app.go)）；monitor 完成后该函数无消费者；删除或改造成 monitor 的薄启动包装，禁止并存两个后台采集循环。

**Acceptance criteria:**
- [ ] `events.StartStatusPoller` 被删除或改造为 monitor 薄包装
- [ ] 无消费者引用（全工作区 grep 验证）
- [ ] 不存在两个后台采集循环并存
- [ ] 既有引用（如 ADR-003）标注"已由 monitor 替代"

**Verification:**
- [ ] `go build ./...` 全绿
- [ ] `go test ./...` 全绿
- [ ] Grep 验证无消费者：`StartStatusPoller` 仅在历史 ADR / 注释中提及

**Dependencies:** Task 7, Task 11, Task 14

**Files likely touched:**
- `shared/motion-control/go/events/status_poller.go`（删除或改造）
- `shared/motion-control/go/events/status_poller_test.go`（删除或改造）

**Estimated scope:** XS（1-2 文件）

---

### Task 22: ADR（急停不可信协议 + Generation 重连语义 + FreshnessPolicy 设计）

**Description:** 编写 ADR 记录三项关键设计决策：(a) Decision 15 急停不可信协议的设计理由与恢复条件；(b) Generation/Sequence 重连语义（重置为 0、ErrGenerationChanged、不自动恢复）；(c) FreshnessPolicy 调用瞬间计算而非固化静态 IsStale 的理由；附 Phase 7 实机测试证据。

**Acceptance criteria:**
- [ ] ADR 文件位于 `docs/decisions/ADR-XXX-motion-status-monitor-design.md`
- [ ] 含三项设计决策的理由、备选方案、风险
- [ ] 附 Phase 7 WTNMC4A 实机测试报告引用
- [ ] 附 Phase 7 DLL 200ms timeout 验证结论
- [ ] 经人工 review

**Verification:**
- [ ] ADR 文件已提交
- [ ] 人工 review 通过

**Dependencies:** Task 19, Task 20

**Files likely touched:**
- `docs/decisions/ADR-XXX-motion-status-monitor-design.md`（新增，编号待定）

**Estimated scope:** XS（1 文件）

---

## Final Verification Checklist

### Build & Test
- [ ] `cd shared\motion-control\go; $env:GOWORK="off"; go test -race -count=10 ./monitor/... ./manager/...`
- [ ] `cd shared\device-sdk\go; $env:GOWORK="off"; go test -race -count=10 ./motion/adapters/hardware -run 'Test(B140|WTNMC4A).*(Concurrent|Status|Disconnect|PriorityCoordinator)'`
- [ ] `cd projects\wind-daq\services\api-go; go test ./internal/... ./api/...; go vet ./internal/... ./api/...; go build -buildvcs=false ./...`
- [ ] `cd projects\wind-daq\apps\desktop-wails\frontend; npm run typecheck; npm run test; npm run build`
- [ ] `cd projects\wind-daq\apps\desktop-wails; task check-bindings`
- [ ] `cd projects\motion-controller\services\api-go; go test ./...; go vet ./...; go build -buildvcs=false ./...`
- [ ] `cd projects\motion-controller\apps\desktop-wails; go test ./...; go vet ./...; go build -buildvcs=false ./...`
- [ ] `cd projects\motion-controller\apps\desktop-wails\frontend; npm run typecheck; npm run build`
- [ ] `powershell -ExecutionPolicy Bypass -File scripts\validate-structure.ps1`

### Functional Success Criteria (来自 spec)
- [ ] B140、WTNMC4A、Simulated 均通过同一 monitor 契约发布状态
- [ ] 每台已连接控制器同一时刻最多一轮 `Status()` 在途
- [ ] `StatusAll()`、HTTP/Wails status 请求不再直接触发额外硬件读取
- [ ] 校准与遍历不再包含周期性 `StatusAll()` 硬件读取循环
- [ ] 四类校准 Main 与运动/遍历组件只观察 motionStore
- [ ] Move/Stop/EStop 后能主动触发状态刷新
- [ ] stale 快照触发 StatusUnavailable，不会提前采集
- [ ] Freshness 通过 `FreshnessPolicy` 在调用瞬间计算
- [ ] `MotionControllerSnapshot` 包含 `Generation` 字段
- [ ] `WaitNext` 在 generation 切换时返回 `ErrGenerationChanged`
- [ ] `NotifyCommandExecuted` 区分 Move/Stop/Config 触发不同 refresh 策略
- [ ] 急停超阈值触发 Decision 15 急停不可信协议
- [ ] 独立窗口失联时 UI 显示横幅并禁用运动命令

### Reliability Success Criteria
- [ ] monitor 关闭、控制器断开、context 取消均无 goroutine 泄漏
- [ ] 慢消费者不阻塞采集器
- [ ] 每控制器快照序号单调，Position/Moving/Limit 来自该控制器同一发布轮次
- [ ] 连续通信失败后快照变 stale，校准/遍历按安全策略中断
- [ ] B140 Stop/EStop 在可控阻塞 fake 下从进入驱动到开始发送硬件命令不超过 600ms
- [ ] WTNMC4A 在正常网络和可恢复超时场景记录 Stop/EStop 排队耗时
- [ ] 急停不可信模式下命令冻结生效
- [ ] 急停不可信模式恢复必须同时满足三重门禁
- [ ] 连接优先级协调器满足 Decision 16 六条不变量
- [ ] 现有 NoProgress 到位容差修复和 RR0 Moving 修复全部回归测试通过

### Compatibility Success Criteria
- [ ] `/api/motion/status` 现有消费者无需修改即可继续读取 `[]ControllerStatus`
- [ ] 现有 motion profiles、Wails bindings、校准/遍历配置 JSON 不发生破坏性变化
- [ ] wind-daq 与 motion-controller 两个项目均通过构建和测试
