# Spec: 位移机构统一状态监视与订阅

> 来源：WTNMC4A 校准 NoProgress 误报调查 → 多轮询源架构审查 → spec-driven-development Phase 1
> 日期：2026-07-16
> 状态：待批准（Phase 1，已按 2026-07-16 评审修订：Freshness 可执行契约、Generation 重连语义、急停不可信协议、优先级协调器行为规则、独立窗口失联降级）
> 关联：
> - [spec-calibration-motion-safety.md](./spec-calibration-motion-safety.md)（校准运动安全消费者）
> - [spec-traversal-motion-safety.md](./spec-traversal-motion-safety.md)（遍历运动安全消费者）
> - [spec-calibration-view-state-recovery.md](./spec-calibration-view-state-recovery.md)（前端校准状态恢复与轮询治理）

## Assumptions

1. 本规格先覆盖 B140、WTNMC4A 与模拟运动控制器，同时服务 WindLabX4 与独立 motion-controller 项目。
2. 硬件无法主动推送完整状态，因此“观察者模式”指后端使用唯一轮询采集器产生快照，再向业务和界面发布；不是要求控制器主动发事件。
3. 共通能力归属 `shared/motion-control/go`；设备协议、寄存器解释和 I/O 继续归属 `shared/device-sdk/go/motion/adapters/hardware`。
4. `GET /api/motion/status` 与现有 Wails/TypeScript 状态字段保持兼容；首期快照元数据只供后端安全逻辑、诊断日志和测试使用，不承诺前端展示。
5. 驱动层保留并发查询合并或串行保护，统一监视器不能成为绕过驱动安全边界的理由。
6. 迁移完成后，校准、遍历、运动界面及独立运动窗口不得各自直接制造硬件状态轮询。

若以上假设被否定，必须先修订本规格，再进入 Plan。

## Objective

### 目标

建立位移机构共通的 `MotionStatusMonitor`：每台已连接控制器只有一个状态采集源，统一生成带时序与新鲜度信息的不可变快照，并向校准、遍历、HTTP/Wails API、主运动界面和独立运动窗口分发。

该能力用于解决：

- 多个业务与前端轮询源重复调用 `StatusAll()`，放大硬件 I/O 和锁等待。
- B140 每轮状态查询包含 10 至 14 条串行 TCP 命令，多个请求可相互穿插并扩大快照采样跨度。
- WTNMC4A 单轮四轴状态实测平均约 135ms，多个请求排队后曾出现 500 至 1160ms HTTP 慢请求。
- `Moving=false`、限位、位置等变化因请求排队而延迟可见，影响校准与遍历到位/安全判定。
- 前端组件既订阅 `motionStore`，又额外启动 300ms `refreshStatus()`，形成重复轮询。
- 当前 `events.StartStatusPoller` 只有 ticker 与 emit，没有缓存、序号、新鲜度和多消费者协调，无法作为可靠状态基础设施。

### 用户

- 风洞校准/遍历操作员：运动状态及时、稳定，不因界面数量或窗口切换影响到位与安全判断。
- 设备维护工程师：能够从诊断日志确认最后尝试、最后成功、通信故障和快照过期，而不是把旧状态误当作实时状态。
- 开发人员：新增运动业务只订阅统一快照，不重复实现轮询、缓存、超时和并发治理。

### 用户可感知结果

1. 打开校准页、遍历页、主运动面板和独立运动窗口时，不会成倍增加真实控制器查询频率。
2. 运动期间 UI 位置与 `Moving` 及时刷新；轴停止后 `Moving=false` 不被其它排队请求延迟数秒。
3. 状态采集异常时，业务得到“状态不可用/过期”，诊断日志给出失败控制器、采样时间和耗时，而不是无限使用旧快照。
4. 校准与遍历继续使用相同到位和安全语义，不因状态基础设施迁移而提前采集或降低保护等级。

## Scope

### In Scope

1. 新增共享状态快照模型与 `MotionStatusMonitor`。
2. 每台控制器唯一轮询、不可变快照缓存、序号、采样时间、新鲜度判断。
3. 多消费者订阅、取消订阅、等待下一快照、主动刷新。
4. 运动/空闲自适应轮询频率。
5. `MotionManager` 的状态读取迁移为读取 monitor 快照。
6. 校准、遍历从“ticker + 直读硬件”迁移为“等待新快照”。
7. HTTP/Wails 状态接口读取同一快照。
8. 前端 `motionApi` 保留唯一状态源，所有组件只观察 `motionStore`。
9. 删除四类校准 Main 的独立 300ms 运动状态轮询。
10. B140 增加驱动层 single-flight；WTNMC4A 保留现有 single-flight。
11. 状态采集诊断日志和可测试的性能门禁。

### Out of Scope

- 不合并 B140 与 WTNMC4A 的协议实现。
- 不改变 B140 TCP 命令、WTNMC4A DLL ABI、轴映射或脉冲换算。
- 不重写 B140 编码器补偿算法；仅定义其与 monitor 的协调边界。
- 不改变校准算法、遍历点位生成、驻留/稳定、CSV schema 或数据采集算法。
- 不新增第三方消息总线、Rx 库或持久化数据库。
- 不要求跨进程共享内存；独立窗口继续通过主进程本地 HTTP 获取状态。
- 不在本规格中把 WindLabX4 的运动安全策略全部迁入 shared；是否提升另立决策。

## Current State

### 已有共通抽象

| 层 | 已有能力 | 位置 |
|---|---|---|
| Core | `AxisStatus`、`ControllerStatus`、轴/控制器配置 | `shared/device-sdk/go/motion/core/types.go` |
| Port | Connect/Status/Move/Jog/Home/Stop/EStop 等原始能力 | `shared/device-sdk/go/motion/ports/motion.go` |
| Manager | 控制器实例、配置、连接与 `StatusAll()` | `shared/motion-control/go/manager/motion_manager.go` |
| Poller | 固定周期 `getStatus → emit` | `shared/motion-control/go/events/status_poller.go` |

### 当前硬件读取源

| 读取者 | 激活条件 | 当前行为 |
|---|---|---|
| 校准 fallbackRuntime | 当前校准点运动阶段 | 每 100ms 调 `StatusAll()` |
| 遍历 waitForMotionComplete | 当前遍历点运动阶段 | 每 100ms 调 `StatusAll()` |
| 遍历稳定复检 | 稳定等待阶段 | 周期调 `StatusAll()` |
| 校准 Main | 当前选中的一种探针 Main 挂载 | 订阅 motionStore，另加 300ms `refreshStatus()` |
| 运动控制面板 | 面板挂载 | 订阅 motionApi 状态源 |
| 独立运动窗口 | 窗口打开 | HTTP 轮询主进程状态接口 |
| 浏览器/Wails 适配层 | 每个监听者 | 可能各建一条轮询链 |

四种校准 Main 通过动态组件一次只挂载一种，不会四种同时读取；但当前可见 Main 的“订阅 + 主动轮询”与后端校准等待循环仍会重复读取。校准、遍历受资源锁约束通常不会同时运行，但 UI/HTTP 读取与任一业务等待循环可以并发。

### 设备差异

| 项目 | B140 | WTNMC4A |
|---|---|---|
| 通信 | TCP ASCII | 官方 DLL 封装 TCP |
| 位置 | `TD` / 编码器 `TPx` | `ReadLP` |
| Moving | `TS` bit 7 | RR0 `XDRV/YDRV/ZDRV/UDRV` |
| 限位 | 每轴 `MG _LFx/_LRx` | RR1 `LMTP/LMTM` |
| 单轮成本 | 四轴约 10 至 14 条串行命令 | 4 次 ReadLP + 完整轮的 RR0/RR1 |
| 驱动并发保护 | 单命令 `connMu`，尚无整轮 single-flight | `ioMu` + Status single-flight |

## Confirmed Decisions

1. **统一采集器位于 Go 后端**：前端关闭后校准/遍历仍需可靠状态，因此前端不能成为唯一采集源。
2. **一个控制器一个采集 flight**：同一时刻每台控制器最多运行一个 `Status()`；多消费者共享结果。
3. **Monitor 由 MotionManager 持有**：manager 负责 monitor 的构造、连接注册、断开注销和关闭；迁移完成后 `MotionManager.Status/StatusAll` 只投影 monitor 最新快照，不直接触发硬件 I/O。
4. **新鲜度是安全契约**：每控制器快照必须带 `Sequence`、`AttemptedAt`、`SucceededAt` 与采集错误；业务不得把过期快照当作实时状态。
5. **命令后主动刷新**：Move/Jog/Home/Stop/EStop/Connect/Disconnect/ApplyConfig 成功或失败后，以 controller ID 请求 monitor 尽快产生新快照。
6. **自适应频率**：运动中高频，全部空闲低频；具体默认值见 Polling Policy，可配置但有安全边界。
7. **慢消费者隔离**：发布者不能因某个订阅者不消费而阻塞硬件采集；每个订阅只保留最新快照。
8. **状态快照不可变**：发布和返回前深拷贝 `Status` 与 `Axes`，消费者不得修改 monitor 内部缓存。
9. **驱动仍负责协议一致性**：B140/WTNMC4A 各自保证单轮读取安全；monitor 不解释寄存器或 TCP 响应。
10. **业务仍负责安全策略**：到位、NoProgress、Overshoot、Deviation、LimitTriggered 等保留在 usecase/shared safety，不下沉到硬件 adapter。
11. **API 兼容优先**：现有 `/api/motion/status` 的主体仍可返回 `[]ControllerStatus`；内部/新增诊断接口可暴露快照元数据。
12. **旧 `StartStatusPoller` 不直接复活**：它应被新 monitor 替换或改造成 monitor 的薄启动包装，禁止并存两个后台采集循环。
13. **Stop/EStop 优先于后续状态命令**：状态查询不能抢占已经进入协议/DLL 的单次调用，但每个调用结束后必须先让出给待处理 Stop/EStop。仅当底层传输能在 500ms 内强制返回时，Stop/EStop 从进入驱动到开始发送硬件命令的保证上限为 600ms；对无法中断的原生调用，该数值只作为目标和告警阈值，不得宣称硬实时保证。超阈值时进入 **急停不可信状态**，必须按 Decision 15 执行系统和操作员安全协议，禁止仅以日志和 UI 文案变更收尾。
14. **B140 补偿不建立第二个通用状态轮询器**：补偿等待停止和限位复用 monitor 快照；补偿 checking/compensating 阶段可直接读取目标轴 `TP` 作为闭环控制测量，并发送补偿命令，但该专用测量不发布为通用状态快照。
15. **急停超阈值的系统与操作员安全协议**：当 Stop/EStop 排队或下发耗时超过该驱动声明的阈值（B140 600ms 硬保证上限；WTNMC4A 600ms 目标/告警阈值，验证通过前不作硬保证）时，系统必须进入"急停不可信"模式并保持到安全状态被显式恢复：
    - **命令冻结**：MotionManager 立即取消所有进行中的运动命令排队，禁用新运动命令输入（Move/Jog/Home/ApplyConfig）；只允许 Stop/EStop/Reset/Disconnect。
    - **告警锁存**：UI 必须显示红色急停失败横幅，包含物理急停按钮位置、断电程序、联系维护人员提示；告警锁存，不得因状态恢复自动清除。
    - **SOP 来源**：物理急停按钮位置与断电程序必须在 `motion_profiles` 配置中显式声明（字段 `emergency_sop.physical_stop_location` / `emergency_sop.power_off_procedure`），未配置的 profile 不允许启动运动任务。
    - **确认按钮语义**："已物理急停"按钮只能记录操作员已知悉事件并打日志，不得关闭告警或恢复运动命令；按钮不得命名为"恢复"或"重置"。
    - **恢复条件**：运动命令解锁必须同时满足：(a) 控制器 Disconnect 成功并重连；(b) monitor 产生首帧 SucceededAt 新鲜快照；(c) 操作员显式触发"复位"动作并完成；任何一项缺失时运动命令保持禁用。
    - **观察性**：超阈值事件必须产生 CRITICAL 日志，包含 controller ID、排队耗时、是否已确认、是否已复位等字段；UI 不得显示"急停已下发成功"。
16. **连接优先级协调器只规定可观察行为不变量**：B140 `connMu`、WTNMC4A `ioMu` 与补偿/Stop/EStop 共存时，必须满足下列不变量，具体类型/实现放 Phase 2 计划：
    - **抢占语义**：当 Critical（Stop/EStop）等待时，当前 Normal（Status）/High（Compensation）单次 I/O 结束后必须立即让出给 Critical；Critical 之间 FIFO。
    - **防插队**：Critical 等待期间到达的 Normal/High 不得在 Critical 完成前抢占；High 之间 FIFO，不抢占 Normal。
    - **可取消**：Critical 等待者必须响应 context 取消；取消后驱动不得继续发送该 Stop/EStop 命令帧，但已发送的不可撤回。
    - **防饥饿**：Critical 长时间等待时，Normal/High 不得持续占用连接；若 Critical 等待超过阈值，必须放弃当前 Normal/High 单次 I/O（B140 通过 SetDeadline，WTNMC4A 通过 SDK timeout），让 Critical 推进。
    - **可观测**：协调器必须暴露当前等待者数量、各优先级等待时长、抢占/放弃次数等指标，供测试断言。
    - **不替换驱动串行锁**：协调器在 `connMu`/`ioMu` 之上，不替换；驱动串行锁仍负责协议一致性。
    Phase 2 计划必须给出具体接口签名、并发测试矩阵和竞态测试用例；本规格不强制采用任何特定 Go 接口形态。

## Proposed Architecture

```text
┌──────────────────────────────────────────────────────────────┐
│ Consumers                                                    │
│ Calibration │ Traversal │ HTTP/Wails │ Motion UI │ Diagnostics│
└──────┬────────────┬────────────┬────────────┬─────────────────┘
       │ WaitNext   │ Subscribe  │ Latest     │
       ▼            ▼            ▼            ▼
┌──────────────────────────────────────────────────────────────┐
│ shared/motion-control/go/monitor                             │
│ MotionStatusMonitor                                         │
│ - one polling loop + snapshot per connected controller      │
│ - immutable aggregate view (explicitly non-atomic)          │
│ - sequence / attemptedAt / succeededAt / error / freshness  │
│ - adaptive interval / RequestRefresh                        │
│ - non-blocking latest-only subscribers                      │
└──────────────────────────────┬───────────────────────────────┘
                               │ controller.Status(ctx)
             ┌─────────────────┼─────────────────┐
             ▼                 ▼                 ▼
       B140 adapter      WTNMC4A adapter    Simulated adapter
       TCP commands      DLL RR0/RR1/LP     in-memory state
```

### Ownership

- `MotionManager` 创建并持有 monitor；构造时注入 clock、monitor config 和 controller factory，测试不依赖真实时间或硬件。
- Connect 成功后注册/启动该控制器的采集循环。
- Disconnect/ApplyConfig 递增该控制器 generation；旧 generation 的在途结果不得发布。
- Disconnect 顺序固定为：标记 closing 并递增 generation → cancel 对应采集循环 → 有界等待在途采集退出 → 调用驱动 Disconnect → 发布断开快照。
- manager 不得持有自身 map/profile 锁等待驱动 I/O、monitor goroutine 或订阅者，避免锁反转。
- 应用关闭时 cancel 根 context，等待所有 monitor goroutine 退出。
- 独立 motion 子进程不连接真实控制器，不启动 monitor；它只消费主进程 API。

## Data Model

```go
// ControllerStatusSnapshot 表示一台控制器的一次完整观察结果。
type ControllerStatusSnapshot struct {
    ControllerID string
    Generation   uint64
    Sequence     uint64
    AttemptedAt  time.Time
    SucceededAt  time.Time
    ValidUntil   time.Time
    Status       core.ControllerStatus
    Err          error
}

// StatusSnapshot 是各控制器最新快照的聚合视图，不保证跨控制器原子同采样。
type StatusSnapshot struct {
    Sequence    uint64
    PublishedAt time.Time
    Controllers []ControllerStatusSnapshot
}

// FreshnessPolicy 是 monitor 注入的新鲜度判定策略，消费者通过它读取 freshness，
// 避免在快照中固化发布瞬间的 Age/IsStale 静态值。
//   - ValidUntil 已写入快照，最简单的策略只需比较 now 与 ValidUntil；
//   - 若需要按业务上下文（运动/空闲）动态切换阈值，可通过 policy 实现；
//   - 消费者禁止自行硬编码 stale 阈值，必须通过注入的 FreshnessPolicy 计算。
type FreshnessPolicy interface {
    // Freshness 在调用瞬间根据当前时钟和快照元数据计算新鲜度。
    // 返回值 Age 是 now-SucceededAt；IsStale 由 policy 按运动/空闲阈值判定。
    Freshness(now time.Time, snap ControllerStatusSnapshot) Freshness
}

type Freshness struct {
    Age     time.Duration
    IsStale bool
}
```

约束：

- 每控制器 `Sequence` 在每次采集尝试完成后单调递增，包括失败尝试；全局 `StatusSnapshot.Sequence` 只表示聚合视图发布版本。
- `AttemptedAt` 每轮更新；`SucceededAt` 仅在该控制器整轮可信采集成功时更新。freshness 只基于 `SucceededAt`。
- 失败快照保留该控制器最后可信 `Status`，同时写本轮 `Err`、新的 `AttemptedAt` 和不变的 `SucceededAt`。
- **Freshness 不固化在快照中**：`Age` 与 `IsStale` 是时间敏感的瞬时值，发布时写入会立即失真（发布瞬间 Age≈0，之后无新发布时该字段永远显示 fresh）。消费者只能通过 `FreshnessPolicy.Freshness(now, snap)` 在调用瞬间计算；`ValidUntil` 字段是 policy 计算的输入，不是消费者直接读取的"是否过期"结论。
- 聚合视图明确是"各控制器最新值"，不承诺不同控制器来自同一采样时刻。校准/遍历按 controller ID 使用对应 `ControllerStatusSnapshot` 的 sequence/freshness。
- `Status`、`Controllers` 和嵌套 `Axes` 对 monitor 外部是不可变值。
- generation 不匹配的在途采集结果直接丢弃，不能更新时间或状态。
- **Generation/Sequence 重连语义**：
    - `Generation` 在 Disconnect/ApplyConfig 时单调递增；同一 controller ID 重连后 Generation 必须大于上一次。
    - 重连后 `Sequence` **重置为 0**；新 generation 的首帧快照 Sequence=1。
    - monitor 必须在重连完成时清空旧 generation 的快照缓存，避免消费者读到 stale generation 的旧数据。
    - 在途 `WaitNext(ctx, id, oldSeq)` 在 generation 切换时不得返回新 generation 的快照（因为 oldSeq 属于旧 generation），必须返回 `ErrGenerationChanged{OldGen, NewGen}` 让消费者显式重新决策。
    - 业务等待循环必须捕获 `ErrGenerationChanged` 并按安全策略 abort 当前点或重新发起等待。
    - 旧 generation 的 Sequence 不会被新 generation 复用，因此 `ErrGenerationChanged` 是单向通知，不能通过"等待更大 sequence"自动恢复。

## Interface Contract

```go
type StatusMonitor interface {
    Latest() StatusSnapshot
    LatestController(controllerID string) (ControllerStatusSnapshot, bool)
    WaitNext(ctx context.Context, controllerID string, afterSequence uint64) (ControllerStatusSnapshot, error)
    Subscribe(ctx context.Context) <-chan StatusSnapshot
    RequestRefresh(controllerID string)
    NotifyCommandExecuted(controllerID string, cmdKind CommandKind)
    FreshnessPolicy() FreshnessPolicy
}

// ErrGenerationChanged 表示 WaitNext 期间目标控制器发生 Disconnect/ApplyConfig，
// afterSequence 所属 generation 已被新 generation 替换；消费者必须显式重新决策。
type ErrGenerationChanged struct {
    ControllerID string
    OldGen       uint64
    NewGen       uint64
}

func (e *ErrGenerationChanged) Error() string { /* ... */ }

// CommandKind 区分运动命令和普通命令，用于 NotifyCommandExecuted 触发不同 refresh 策略。
type CommandKind int
const (
    CmdKindMove CommandKind = iota  // Move/Jog/Home：触发 2s 快速窗口
    CmdKindStop                     // Stop/EStop：仅触发单轮 refresh
    CmdKindConfig                    // ApplyConfig：触发单轮 refresh + generation 重置
)
```

### `Latest`

- 只读内存，不访问硬件。
- 未产生首个快照时返回零序号，并由调用者按 unavailable 处理。
- 返回深拷贝或只读安全副本。

### `WaitNext`

- 等待目标控制器 `Sequence > afterSequence` 的下一快照。
- 若调用瞬间已有更大 Sequence 的快照（与 afterSequence 同 generation），必须立即返回，不得额外等待下一轮——这是避免 TOCTOU 的关键。
- context 取消后立即返回 `ctx.Err()`。
- 不为每个等待者单独触发硬件读取。
- 校准和遍历用它替代自己的 ticker + `StatusAll()`。
- "检查 sequence"和"注册 waiter"必须在同一临界区或同一事件循环中完成，禁止检查后注册造成丢失唤醒。
- **Generation 切换语义**：若目标控制器在 WaitNext 期间发生 Disconnect/ApplyConfig（generation 递增），必须返回 `ErrGenerationChanged{OldGen, NewGen}`，不得返回新 generation 的快照。消费者按安全策略处理（典型为 abort 当前点并通知上层）。
- **首帧语义**：若 controller 重连后尚无首帧快照（SucceededAt 为零值），WaitNext 必须阻塞直到首帧产生或 context 取消；不得用零值快照立即返回。

### `Subscribe`

- 首次订阅立即投递当前快照（若存在）。
- 每个订阅者 channel 容量为 1 或等价 latest-only 语义。
- 新快照到达而旧快照未消费时覆盖/丢弃旧快照，不阻塞采集循环。
- context 取消后关闭订阅 channel 并移除订阅者。
- channel 只允许 monitor 所有者关闭；发布、注销和关闭必须经同一锁/事件循环串行化，禁止 send-on-closed。
- **使用场景区分**：Subscribe 返回聚合视图，适合 UI（关心整体显示、可接受跨控制器非原子）；业务安全判定不得使用 Subscribe，必须用 `LatestController` + `WaitNext` 按单控制器 sequence/freshness 判断。

### `RequestRefresh`

- 按 controller ID 非阻塞、幂等、可合并。
- 每控制器维护一个 pending refresh 位：若无采集在途则尽快启动；若已有采集在途只置位一次，完成后清位并最多补一轮。
- 连续请求不得无限推迟既定周期轮询，也不得刷新无关控制器。
- **不触发 2s 快速窗口**：RequestRefresh 是单轮额外采集，频率模式不变；适合 UI 显式刷新或诊断触发。

### `NotifyCommandExecuted`

- 由 MotionManager 在运动命令（Move/Jog/Home）成功或失败返回后调用，触发该 controller 的 2s 快速观察窗口 + 一轮额外采集。
- Stop/EStop/Reset 命令仅触发单轮 refresh（不进入快速窗口，避免 Stop 后高频采集放大硬件压力）。
- ApplyConfig 触发 generation 重置 + 单轮 refresh。
- 快速窗口期间轮询频率按 Polling Policy 的 moving 间隔执行；窗口结束后回落到 idle 间隔。
- 与 RequestRefresh 共存时合并：若快速窗口已激活，RequestRefresh 仅触发一轮额外采集，不延长窗口。

## Polling Policy

默认值：

| 状态 | 间隔 | 说明 |
|---|---:|---|
| 任一轴 `Moving` 或 `Compensating` | 100ms | 支持到位与安全判定 |
| 命令后快速观察窗口 | 100ms，持续 2s | 避免硬件启动位尚未拉起时误降频 |
| 全部连接控制器空闲 | 500ms | UI 位置与限位心跳 |
| 无已连接控制器 | 不轮询 | 等待 Connect/RequestRefresh |

规则：

- 周期从上一轮完成后开始计算，禁止 `setInterval` 式重入和积压。
- 若单轮耗时超过目标间隔，下一轮立即或最小退让后开始，但同一控制器绝不并发采集。
- 轮询间隔不得小于控制器和网络可承受范围；默认 100ms 是目标节奏，不保证慢设备每秒严格 10 帧。
- 空闲间隔不得大于快照 stale 阈值，否则正常空闲快照会被误判过期。

## Freshness and Failure Semantics

### Freshness 计算契约

Freshness 由注入 monitor 的 `FreshnessPolicy` 在调用瞬间计算，不固化在快照中。默认 policy 行为：

- monitor 在每次成功采集时写入 `ValidUntil = SucceededAt + staleThreshold`，staleThreshold 按"运动/空闲"模式选择。
- `FreshnessPolicy.Freshness(now, snap)` 计算 `Age = now - snap.SucceededAt`；`IsStale = now > snap.ValidUntil`（或等价地 `Age > staleThreshold`）。
- 模式判定由 policy 内部完成，可读取快照中 `Status` 的 `Moving`/`Compensating` 字段；消费者只调用 `Freshness()` 不感知模式。
- **不允许**在快照结构体中预存 `IsStale bool` 字段，也不允许消费者自行硬编码阈值。
- 测试时注入可控时钟（`FakeClock`）和可配置 policy，验证 stale 判定和模式切换。

默认 stale 阈值：

| 模式 | staleThreshold | 说明 |
|---|---:|---|
| 运动/补偿中 | `max(3 × movingInterval, 1s)` | movingInterval=100ms，阈值=1s |
| 空闲 | `max(3 × idleInterval, 2s)` | idleInterval=500ms，阈值=2s |
| 无首帧 / SucceededAt 零值 | 立即 stale | 等同 monitor 未启动 |
| Err != nil 且 SucceededAt 仍在 stale 阈值内 | 仍按 SucceededAt 判定 | 单次失败不立即推翻可信状态 |
| Err != nil 且 SucceededAt 已 stale | stale | 不掩盖长期故障 |

### 设备专属阈值

WTNMC4A 实测单轮状态采集平均约 135ms，历史 500-1160ms 慢请求**主要来自重复轮询排队放大**（多消费者各自调 StatusAll），不是单轮硬件调用本身。统一 monitor 后排队放大消失，预期单轮耗时回归到 135ms 量级。

- **禁止**在未区分"底层单轮真实耗时"和"排队耗时"的情况下直接上调 WTNMC4A 的 stale 阈值。
- 实机只读测试（[Real Hardware Read-Only Tests](#real-hardware-read-only-tests)）必须同时报告：单调用 p50/p95、3 并发批次 p50/p95、排队增量比。
- 若实测证明 WTNMC4A 单轮真实最坏耗时超过 1s（极端情况），才考虑为 WTNMC4A 配置专属 staleThreshold；上调必须 ADR 记录，且不得放宽到掩盖通信故障的水平。
- B140 单轮命令数 10-14 条 × 单条 SetDeadline 500ms 上界远大于 1s stale 阈值；但 B140 实际单轮总耗时通常远小于 100ms（命令串行但快），不触发专属阈值。

### 失败语义

- 连续失败只更新 `AttemptedAt` 与 `Err`，不更新 `SucceededAt`。
- 失败快照仍发布，消费者通过 `Err != nil` 可见本轮错误；但 Freshness 仍按 `SucceededAt` 判定，避免单次抖动触发 StatusUnavailable。
- 连续失败导致 SucceededAt 过期时，IsStale 自动变为 true，触发 StatusUnavailable。
- 不得通过增大 stale 阈值掩盖长期通信故障；连续失败 N 次（建议 N=5，对应 5 × movingInterval 或 idleInterval）必须产生 CRITICAL 日志。

### 业务行为

| 条件 | 校准/遍历行为 |
|---|---|
| 新快照且状态正常 | 正常执行到位与安全判定 |
| 单次失败但仍在 stale 阈值内 | 可继续等待，记录可见错误，不立即使用失败字段覆盖可信状态 |
| 快照 stale | `StatusUnavailable`，按既有急停/停止语义处理 |
| monitor 未启动/无首帧 | 不允许开始依赖运动状态的任务，返回状态不可用 |
| Disconnect/EStop | 立即发布保守快照并请求硬件确认 |
| `ErrGenerationChanged` | abort 当前点，按安全策略复位或重新发起等待 |

不得通过增大 stale 阈值掩盖长期通信故障。

## Driver Responsibilities

### 共通要求

- `Status(ctx)` 返回单轮尽可能一致的控制器快照。
- 并发 `Status()` 不得重复放大硬件 I/O；重叠调用可共享同一轮结果。
- Move/Stop/Disconnect 与在途 Status 必须有明确定义的状态版本或锁顺序，旧查询不得覆盖新命令结果。
- context 超时/连接变更时返回最后可信快照和错误。
- 状态查询不得长期独占命令连接：WTNMC4A 维持单次 DLL 调用粒度让出 `ioMu`；B140 维持单条 TCP 命令粒度让出 `connMu`。
- B140 TCP 必须通过 `SetDeadline` 让单次状态 I/O 在 500ms 内强制返回；因此 B140 的 Stop/EStop 排队保证为 600ms。
- WTNMC4A 使用 SDK `DEV_CreateA` 的 200ms send/recv timeout，但 DLL 是否严格可中断必须通过实机故障注入验证。验证通过前，600ms 仅为目标/慢安全命令告警阈值，不作为 WTNMC4A 硬保证。
- 若底层 DLL 调用超过 deadline/告警阈值仍未返回，记录 CRITICAL 日志、令快照失败/stale，并在调用最终返回后优先执行已排队 Stop/EStop；不得并发发送第二条命令绕过协议串行约束，也不得向操作员显示"急停已下发成功"。
- 超阈值事件必须触发 Decision 15 的急停不可信协议（命令冻结、告警锁存、SOP 提示、恢复条件）。

### Connection Priority Coordinator

驱动必须实现一个连接优先级协调器，包装在 `connMu`/`ioMu` 之上，承担"安全命令优先于普通状态/补偿"的调度。本规格只规定可观察行为不变量，具体类型与实现放 Phase 2 计划。

**优先级类别**：

| 优先级 | 命令类型 | 说明 |
|---|---|---|
| Critical | Stop、EStop | 必须最先执行 |
| High | B140 补偿 TP 直读、补偿命令 | 闭环控制测量，不可被普通状态轮询阻塞 |
| Normal | monitor `Status()` 轮询、HTTP status 读取 | 可被抢占 |

**不变量**：

1. **抢占语义**：Critical 等待时，当前 Normal/High 单次 I/O 完成后立即让出给 Critical；Critical 之间 FIFO。
2. **防插队**：Critical 等待期间到达的 Normal/High 不得在 Critical 完成前抢占；High 之间 FIFO，不抢占 Normal。
3. **可取消**：Critical 等待者响应 context 取消；取消后驱动不得继续发送该 Stop/EStop 命令帧，但已发送的不可撤回。
4. **防饥饿**：Critical 长时间等待时，Normal/High 不得持续占用连接；若 Critical 等待超过该驱动的告警阈值（B140 600ms / WTNMC4A 600ms 目标），必须放弃当前 Normal/High 单次 I/O（B140 通过 SetDeadline 强制返回；WTNMC4A 通过 SDK 200ms timeout 让出），让 Critical 推进。
5. **可观测**：协调器必须暴露当前等待者数量、各优先级等待时长、抢占/放弃次数等指标，供测试断言（参见 [Observability](#observability)）。
6. **不替换驱动串行锁**：协调器在 `connMu`/`ioMu` 之上，不替换；驱动串行锁仍负责协议一致性。

**Phase 2 计划必须给出**：

- 具体接口签名（不强制采用任何特定形态，但必须满足上述不变量）；
- 并发测试矩阵（Critical/High/Normal × 多种到达顺序 × context 取消 × 饥饿恢复）；
- 竞态测试用例（`-race -count=10`）；
- 性能门禁（Critical 抢占延迟、协调器开销上限）。

### B140 专属

- TCP 命令继续由 `connMu` 严格串行。
- 增加整轮 `Status()` single-flight，避免多个调用者重复发送 `TD/TS/MG/TP`。
- 保留"TS 显示刚停止后重读 TD/TP"的最终位置一致性逻辑。
- 编码器补偿状态机等待停止与检查限位时消费 monitor 的目标轴快照，不再独立轮询 `TS/MG`。
- 补偿 `checking/compensating` 阶段保留直接 `TP` 读取和补偿命令发送，因为它们是目标轴闭环控制测量，不是面向业务消费者的通用状态采集；这些调用注册为 High 优先级，与 monitor Normal Status 和 Critical Stop/EStop 共用连接优先级协调器。
- B140 的 `SetDeadline` 同时用于：(a) 单条 TCP 命令的 500ms 强制返回；(b) 防饥饿场景下放弃 Normal I/O 让 Critical 推进。

### WTNMC4A 专属

- 保留 RR0 判断 Moving、RR1 判断限位、ReadLP 读取位置。
- 保留现有 `ioMu`、状态版本与 Status single-flight。
- monitor 不能重新引入直接 DLL 并发。
- WTNMC4A 的 SDK 200ms timeout 同时用于：(a) 单次 DLL 调用的超时；(b) 防饥饿场景下放弃 Normal I/O 让 Critical 推进。
- DLL 是否严格可中断必须通过实机故障注入验证（参见 [Real Hardware Read-Only Tests](#real-hardware-read-only-tests)）；验证通过前，600ms 仅作为目标/告警阈值，超阈值时按 Decision 15 进入急停不可信协议。

### Simulated 专属

- 使用同一 monitor 契约，以便测试轮询、订阅和新鲜度，不允许产品代码绕过 monitor 形成仿真/实机两套流程。

## Motion Safety Integration

到位与安全规则不因 monitor 改造而改变。但安全判定必须先通过元数据门禁，再执行业务字段判定：

```go
// JudgeArrival 是校准/遍历通用到位判定，必须在 monitor 快照上下文中调用。
// 返回 arrived=true 仅当四层门禁全部通过；任何一层失败按对应状态处理。
func JudgeArrival(
    ctx context.Context,
    reader MotionStatusReader,
    controllerID string,
    axisID string,
    target float64,
    arrivalTolerance float64,
    awaitedSeq uint64, // 上次观察到的 sequence；0 表示首帧
    awaitedGen uint64, // 上次观察到的 generation
) (arrived bool, status JudgeStatus, snap MotionControllerSnapshot) {

    // 第 1 层：Generation 一致性
    snap, err := reader.WaitNext(ctx, controllerID, awaitedSeq)
    if errors.Is(err, ErrGenerationChanged) {
        return false, JudgeGenerationChanged, snap // 业务必须 abort 当前点
    }
    if err != nil {
        return false, JudgeError, snap // ctx 取消或其他错误
    }
    if snap.Generation != awaitedGen {
        // 双重保护：WaitNext 应已返回 ErrGenerationChanged，此处兜底
        return false, JudgeGenerationChanged, snap
    }

    // 第 2 层：Freshness
    if snap.Freshness.IsStale {
        return false, JudgeStatusUnavailable, snap // 不得使用旧 Moving=false
    }

    // 第 3 层：Sequence 推进
    if snap.Sequence <= awaitedSeq {
        return false, JudgeWaitMore, snap // 还在等同一轮，不应发生（WaitNext 语义保证）
    }

    // 第 4 层：业务字段
    axis, ok := snap.Status.FindAxis(axisID)
    if !ok {
        return false, JudgeError, snap
    }
    arrived := !axis.Moving && math.Abs(axis.Position-target) <= arrivalTolerance
    if !arrived {
        return false, JudgeNotArrived, snap
    }
    return true, JudgeArrived, snap
}
```

补充规则：

- `Moving=true` 且已进入到位容差：继续等待 `Moving=false`，不触发 NoProgress。
- `Moving=true` 且容差外长时间无进展：触发 NoProgress。
- 快照 stale：触发 StatusUnavailable，不使用旧 `Moving=false` 提前进入驻留/稳定/采样。
- 校准到位后仍按配置执行 `DwellTimeMs`；遍历仍执行 fixed/adaptive stabilization。
- 安全判定基于目标控制器单个 `ControllerStatusSnapshot.Sequence` + `Generation`，不得混用不同轮次的 Position 与 Moving。
- **Generation 变更时禁止 fallthrough**：业务不得在 `ErrGenerationChanged` 后继续用旧 sequence 等待；必须显式 abort 当前点并通知上层。
- **首次等待**：若 `awaitedSeq=0`，业务必须先调 `LatestController` 获取当前快照作为基准，再调 `WaitNext`；不得在零基准下直接 `WaitNext` 后判定 arrived。

## API and Frontend Contract

### HTTP/Wails

- `/api/motion/status` 改为读取 `Latest()`，不得触发真实硬件查询。
- 保持当前 `[]ControllerStatus` 响应兼容。
- 增加诊断元数据时优先使用独立端点或响应 header；若改响应结构，必须走 API 影响分析和兼容迁移。
- Wails `MotionGetStatus` 同样读取缓存。

### Frontend

- `motionApi` 在每个进程内最多保持一条状态轮询/订阅链。
- `motionStore.attachStatusListener()` 只注册观察者，不创建新的后端轮询链。
- `motionStore.refreshStatus()` 读取后端缓存，仅用于首次加载/显式刷新；不得被页面定时器周期调用。
- 删除五孔、三孔、总压 Main 的 300ms `motionStatusPollTimer`；总温如新增同类轮询亦禁止。
- Calibration Main、Traversal Main、MotionControlPanel 都只消费 `motionStore.statusList`。
- 独立 motion 窗口仍通过主进程 HTTP 轮询，但该请求只读取 monitor 缓存，不访问硬件。

### 独立 motion 窗口失联降级协议

独立 motion 窗口依赖主进程 HTTP 状态接口。主进程崩溃或网络中断时，独立窗口必须按下列协议降级，禁止静默显示旧状态：

| 触发条件 | UI 行为 | 命令可用性 |
|---|---|---|
| HTTP 请求超时（建议 1s） | 显示"主进程连接丢失，状态不可信"横幅 | 禁用所有运动命令输入 |
| 连续失败 N=3 次（≈1.5s） | 锁定 UI 为只读模式，红色横幅持续显示 | 禁用所有运动命令，仅允许"重连"按钮 |
| 重连成功 | 横幅变绿"已重连"2s 后消失，刷新当前快照 | 恢复运动命令输入 |
| 主进程监控到自身 monitor 故障 | 通过 HTTP 503 + 错误体返回，独立窗口识别后等同失联处理 | 同上 |

约束：

- 独立窗口必须维护本地"上次成功响应时间"和"连续失败计数"，超时阈值与重试间隔可配置。
- 失联期间前端不得使用最后一次成功响应的快照作为"实时状态"显示；必须显式标注"数据时间：YYYY-MM-DD HH:MM:SS（已过期）"。
- 失联期间若用户尝试发送运动命令，必须立即拒绝并提示"主进程失联，命令不可发送"。
- "重连"按钮触发一次主动 HTTP 请求；成功后清空失败计数，失败则继续锁定。
- 失联状态必须通过 IPC 通知主进程（若 Wails 模式支持），便于主进程记录独立窗口失联事件。

### WindLabX4 Backend Port

WindLabX4 的安全用例不能继续只依赖 `StatusAll(ctx) []ControllerStatus`，否则无法判断 sequence、generation、失败和 freshness。项目侧新增只读端口，adapter 将 shared monitor 快照转换为项目类型：

```go
type MotionStatusReader interface {
    LatestController(controllerID string) (MotionControllerSnapshot, bool)
    WaitNext(ctx context.Context, controllerID string, afterSequence uint64) (MotionControllerSnapshot, error)
    FreshnessPolicy() FreshnessPolicy
}

// MotionControllerSnapshot 是项目侧快照，必须保留 Generation 与 Freshness，
// 否则重连后等待语义和 stale 判定将不完整（参见 Data Model Generation/Sequence 重连语义）。
type MotionControllerSnapshot struct {
    Generation  uint64
    Sequence    uint64
    AttemptedAt time.Time
    SucceededAt  time.Time
    ValidUntil   time.Time
    Status       motion.ControllerStatus
    Err          error
    Freshness    Freshness // reader 调用 FreshnessPolicy 时计算注入，不固化在 monitor 缓存中
}

// ErrGenerationChanged 在 WaitNext 期间 controller 发生 Disconnect/ApplyConfig 时返回，
// 消费者必须按安全策略 abort 当前点，不得用旧 sequence 继续。
var ErrGenerationChanged = errors.New("motion: controller generation changed")
```

约束：

- adapter 在 `LatestController` / `WaitNext` 返回前必须调用 `FreshnessPolicy.Freshness(now, snap)` 计算并填充 `Freshness` 字段；不得将 monitor 缓存中的快照（若存在静态 Age）原样投影。
- `Generation` 必须从 shared `ControllerStatusSnapshot.Generation` 透传，禁止在投影时丢失。
- `WaitNext` 必须透传 monitor 的 `ErrGenerationChanged`，包装为项目侧 `ErrGenerationChanged`，保留 OldGen/NewGen 信息（可通过 `errors.As` 取出）。
- HTTP/Wails 兼容路径可以把该快照投影为现有 `[]ControllerStatus`（仅业务字段，不含 Generation/Freshness）；校准和遍历必须使用 `MotionStatusReader` 而非兼容路径，保留元数据并执行 stale 门禁。

## Tech Stack

- Go 1.25，标准库 `context`、`sync`、`time`、`log/slog`
- Vue 3 + TypeScript + Pinia
- Wails v3 alpha.95
- Go 标准 `testing`；前端 Vitest
- 不新增第三方依赖

## Commands

```powershell
# shared motion-control
cd shared\motion-control\go
$env:GOWORK="off"
go test ./...
go vet ./...

# shared device-sdk
cd shared\device-sdk\go
$env:GOWORK="off"
go test ./...
go vet ./...

# WindLabX4 backend
cd projects\WindLabX4\services\api-go
go test ./internal/... ./api/...
go vet ./internal/... ./api/...
go build -buildvcs=false ./...

# WindLabX4 frontend
cd projects\WindLabX4\apps\desktop-wails\frontend
npm run typecheck
npm run test
npm run build

# Wails binding/API consistency
cd projects\WindLabX4\apps\desktop-wails
task check-bindings

# 结构校验（工作区根目录）
powershell -ExecutionPolicy Bypass -File scripts\validate-structure.ps1

# motion-controller backend service
cd projects\motion-controller\services\api-go
go test ./...
go vet ./...
go build -buildvcs=false ./...

# motion-controller desktop shell
cd projects\motion-controller\apps\desktop-wails
go test ./...
go vet ./...
go build -buildvcs=false ./...

# motion-controller frontend
cd projects\motion-controller\apps\desktop-wails\frontend
npm run typecheck
npm run test
npm run build

# 并发竞态门禁（在支持 race 的环境执行）
cd shared\motion-control\go
$env:GOWORK="off"
go test -race ./monitor/... ./manager/... -count=10

cd shared\device-sdk\go
$env:GOWORK="off"
go test -race ./motion/adapters/hardware -run 'Test(B140|WTNMC4A).*(Concurrent|Status|Disconnect)' -count=10
```

当前 Windows 开发机若因 C 运行库 side-by-side 配置无法启动 `-race` 二进制，必须在支持 race 的 CI/开发环境执行并记录结果；不得以普通测试代替竞态门禁。

只读实机验证（禁止发送运动命令）：

```powershell
cd shared\device-sdk\go
$env:GOWORK="off"
$env:WTNMC4A_READONLY_IP="192.168.3.141"
$env:WTNMC4A_READONLY_ITERATIONS="300"
go test ./motion/adapters/hardware -run '^TestWTNMC4AReadOnly(Stability|ConcurrentStatus)$' -count=1 -v -timeout 120s
```

B140 实机地址未确认前，只运行 fake TCP server 与仿真测试，不默认执行真实运动。

## Project Structure

预期结构（Phase 2 可微调文件名，不得改变层级职责）：

```text
shared/
├── device-sdk/go/motion/
│   ├── core/types.go
│   │   └── 原始 AxisStatus / ControllerStatus（保持硬件无关）
│   ├── ports/motion.go
│   │   └── 原始 MotionController 能力
│   └── adapters/hardware/
│       ├── b140_motion.go          # 协议 + single-flight
│       └── wtnmc4a_motion.go       # 协议 + single-flight
└── motion-control/go/
    ├── manager/motion_manager.go   # 控制器生命周期与命令编排
    ├── monitor/
    │   ├── monitor.go              # StatusMonitor 实现
    │   ├── snapshot.go             # 快照/新鲜度模型
    │   └── monitor_test.go
    └── events/status_poller.go     # 删除、弃用或变为 monitor 薄包装

projects/windlabx4/
├── services/api-go/internal/
│   ├── ports/motion.go             # MotionStatusReader：保留 sequence/freshness/error
│   ├── adapters/motion/wrapper.go  # shared → project 类型转换
│   └── usecase/
│       ├── calibration.go          # WaitNext 替代直读 ticker
│       └── traversal_acquisition.go# WaitNext 替代直读 ticker
└── apps/desktop-wails/frontend/src/
    ├── api/motionApi.ts            # 单一状态源
    ├── stores/motionStore.ts       # 多组件共享状态
    └── components/calibration/*Main.vue
        └── 删除页面级运动状态轮询
```

## Code Style

- 接口小而稳定，避免把 monitor 变成“万能运动服务”。
- Go 导出类型和方法必须有注释；注释解释时序/安全原因。
- 不使用全局单例；生命周期由 app context/manager 显式持有。
- channel 只用于通知与快照分发，不通过 channel 传递可变共享对象。
- 所有 goroutine 必须有 context 退出路径；测试必须验证退出。
- 不使用 `time.Sleep` 断言轮询时序；注入 clock/ticker 或使用可控触发器。
- TypeScript 不使用 `any`、`@ts-ignore` 或页面级重复 timer。

示例：

```go
func (m *Monitor) WaitNext(ctx context.Context, controllerID string, after uint64) (ControllerStatusSnapshot, error) {
    // 实现必须在同一临界区内完成 sequence 检查与 waiter 注册，
    // 避免发布恰好发生在两者之间导致丢失唤醒。
    return m.waitNextLocked(ctx, controllerID, after)
}
```

## Testing Strategy

### Unit Tests

Monitor：

- 多个并发消费者只触发一轮 controller `Status()`。
- `Latest()` 不触发硬件读取。
- `WaitNext()` 只返回更大 sequence，context 取消及时退出。
- `WaitNext()` 在调用瞬间已有更大 sequence 时立即返回，不等待下一轮（TOCTOU 测试）。
- 慢订阅者不阻塞采集；只收到最新快照。
- 快照深拷贝，消费者修改不污染缓存。
- 运动/空闲频率切换正确。
- `RequestRefresh(controllerID)` 可合并且不会重入或刷新无关控制器；不触发 2s 快速窗口。
- `NotifyCommandExecuted(controllerID, CmdKindMove)` 触发 2s 快速窗口 + 一轮额外采集；窗口结束后频率回落。
- `NotifyCommandExecuted(controllerID, CmdKindStop)` 仅触发单轮 refresh，不进入快速窗口。
- `NotifyCommandExecuted(controllerID, CmdKindConfig)` 触发 generation 重置 + 单轮 refresh。
- 采集失败、恢复、stale 判定正确。
- Disconnect/Shutdown 无 goroutine 泄漏。
- `WaitNext` 检查/注册交界处发布不会丢失唤醒。
- 发布与订阅取消并发不会 send-on-closed 或数据竞争。
- `Latest/WaitNext/Subscribe/RequestRefresh/NotifyCommandExecuted/Disconnect` 混合并发通过 race 测试。

FreshnessPolicy：

- 注入 `FakeClock`，验证 `Age = now - SucceededAt` 在不同时刻的计算结果。
- 验证 `IsStale` 在 `now > ValidUntil` 时为 true，在 `now <= ValidUntil` 时为 false。
- 验证运动/空闲模式切换时 staleThreshold 正确切换。
- 验证 Err != nil 但 SucceededAt 仍新鲜时 IsStale=false。
- 验证快照中不存在静态 `IsStale` 字段被消费者误读。
- 验证消费者禁止自行硬编码阈值（通过代码审查或 lint 规则）。

Generation/Sequence 重连语义：

- Disconnect 后 Generation 单调递增；Sequence 重置为 0。
- 重连后首帧快照 Sequence=1，Generation 大于上一次。
- 重连完成时旧 generation 的快照缓存被清空。
- 在途 `WaitNext(ctx, id, oldSeq)` 在 generation 切换时返回 `ErrGenerationChanged{OldGen, NewGen}`，不返回新 generation 快照。
- 业务捕获 `ErrGenerationChanged` 后 abort 当前点；测试用例模拟校准/遍历在重连后正确 abort。
- 首帧语义：重连后尚无首帧时 WaitNext 阻塞，不立即返回零值快照。
- `ApplyConfig` 触发 generation 重置，行为等同 Disconnect+Connect。
- 旧 generation 的 Sequence 不被新 generation 复用（不会出现 Sequence 回退）。

Driver：

- B140 并发 Status 只发送一套 `TD/TS/MG/TP`。
- B140 命令与 Status 并发时协议响应不串线，旧状态不覆盖新命令。
- B140 补偿 `TP`/补偿命令与 Stop/EStop 竞争时，当前单次 I/O 后 Stop/EStop 优先，后续补偿与 monitor 命令不得插队。
- WTNMC4A 并发 Status 继续只执行一轮 DLL 读取。
- 两种驱动停止边界都返回最终位置 + `Moving=false` 的一致快照。

Connection Priority Coordinator（驱动层）：

- Critical 等待时，当前 Normal/High 单次 I/O 完成后立即让出给 Critical。
- Critical 等待期间到达的 Normal/High 不得在 Critical 完成前抢占。
- Critical 之间 FIFO；High 之间 FIFO，不抢占 Normal。
- Critical 等待者响应 context 取消；取消后驱动不继续发送该 Stop/EStop 命令帧。
- Critical 等待超过告警阈值（B140 600ms / WTNMC4A 600ms 目标）时，放弃当前 Normal I/O 让 Critical 推进。
- 协调器暴露的指标（等待者数量、各优先级等待时长、抢占/放弃次数）可被测试断言。
- 协调器在 `connMu`/`ioMu` 之上，不替换驱动串行锁；测试验证串行锁仍负责协议一致性。
- 协调器混合并发（Critical/High/Normal × 多种到达顺序 × context 取消 × 饥饿恢复）通过 `-race -count=10`。

急停不可信协议（Decision 15）：

- Stop/EStop 排队或下发耗时超过驱动声明阈值时，MotionManager 进入急停不可信模式。
- 命令冻结：Move/Jog/Home/ApplyConfig 被禁用；Stop/EStop/Reset/Disconnect 仍可用。
- 告警锁存：UI 红色横幅显示；状态恢复后不自动清除。
- SOP 来源：未配置 `emergency_sop` 的 profile 启动运动任务时返回错误。
- 确认按钮：点击后只记录操作员已知悉事件并打日志，不关闭告警或恢复运动命令。
- 恢复条件：必须同时满足 (a) Disconnect+重连成功、(b) monitor 首帧 SucceededAt 新鲜、(c) 操作员显式"复位"。
- CRITICAL 日志包含 controller ID、排队耗时、是否已确认、是否已复位等字段。
- UI 不显示"急停已下发成功"。

### Integration Tests

- MotionManager + Simulated + Monitor：连接、运动、停止、断开产生预期 sequence。
- 校准等待从 monitor 获取快照，到位/NoProgress/StatusUnavailable 语义不变。
- 遍历等待与稳定复检消费 monitor，不直接调用硬件 mock。
- `/api/motion/status` 高频并发请求不会增加 fake controller `Status()` 调用次数。
- 主窗口 + 独立窗口同时读取状态时，只有主进程 monitor 采集硬件。
- `JudgeArrival` 四层门禁（Generation/Freshness/Sequence/业务字段）按预期触发对应状态。
- 校准/遍历在 `ErrGenerationChanged` 后正确 abort 当前点，不使用旧 sequence 继续等待。

### Frontend Tests

- 多个 `attachStatusListener` 只建立一个底层状态源。
- 最后一个 listener 取消后正确停止前端轮询链。
- 四类校准 Main 不再创建 `motionStatusPollTimer`。
- 组件仍能从 motionStore 显示位置、Moving、限位。

### 独立窗口失联降级测试

- HTTP 请求超时（1s）后 UI 显示"主进程连接丢失，状态不可信"横幅，禁用运动命令输入。
- 连续失败 3 次后 UI 锁定为只读模式，仅允许"重连"按钮。
- 失联期间前端不使用最后成功响应的快照作为实时状态；显式标注"数据时间：YYYY-MM-DD HH:MM:SS（已过期）"。
- 失联期间发送运动命令被立即拒绝并提示"主进程失联，命令不可发送"。
- "重连"按钮触发 HTTP 请求；成功后清空失败计数并恢复，失败则继续锁定。
- 主进程返回 HTTP 503 时独立窗口识别并等同失联处理。

### Real Hardware Read-Only Tests

- WTNMC4A 192.168.3.141：单线程稳定性、3 并发状态共享、最大耗时记录。
- 同时报告：单调用 p50/p95、3 并发批次 p50/p95、排队增量比；区分底层单轮真实耗时和排队耗时。
- 禁止测试发送 Move/Jog/Home/Stop/Reset/DefinePosition。
- WTNMC4A DLL 200ms timeout 故障注入测试：模拟 DLL 调用超过 deadline，验证是否触发 CRITICAL 日志和急停不可信协议（Decision 15）。
- B140 真实地址确认后增加同等只读测试；此前由 fake TCP server 覆盖命令数和并发。

## Boundaries

### Always Do

- 修改任何 `Status`、manager 或 monitor 符号前运行 GitNexus impact。
- 行为改动先写失败测试，再实现。
- 保持 core 无 I/O、ports 只有接口、hardware adapter 无业务安全策略。
- 快照必须携带 sequence、generation、时间和错误语义。
- Freshness 必须通过注入的 `FreshnessPolicy` 在调用瞬间计算，禁止固化在快照中。
- `MotionControllerSnapshot` 必须保留 `Generation`，禁止在 shared → project 投影时丢失。
- 提交前运行 shared、WindLabX4 后端和前端定向/全量验证。
- 实机测试默认只读，并在输出中明确记录是否发送运动命令。

### Ask First

- 改变 `/api/motion/status` JSON 顶层形状。
- 修改 `core.ControllerStatus` 公共字段或 Wails binding。
- 引入第三方事件总线/响应式库。
- 改变默认轮询/新鲜度阈值超出本规格范围。
- 让 B140 补偿完全依赖 monitor 快照，或改变其闭环频率。
- 执行任何真实硬件运动、停止、回零、复位或位置定义测试。
- 上调 WTNMC4A 的 stale 阈值（必须先有实机只读测试报告区分底层真实耗时和排队耗时，再单独 ADR）。
- 改变急停不可信协议的恢复条件或确认按钮语义。

### Never Do

- 在前端、Wails backend 薄桥接层或 API handler 中直接访问硬件。
- 让每个订阅者各建一条后端硬件轮询。
- 使用旧快照的 `Moving=false` 提前进入驻留/稳定/采样。
- 用增大 NoProgress/stale 超时掩盖状态采集拥塞。
- 将 B140 TCP 命令或 WTNMC4A RR 寄存器逻辑放入通用 monitor。
- 删除驱动层串行锁/single-flight 作为"统一 monitor 已足够"的简化。
- 在没有 context 退出和 goroutine 泄漏测试的情况下启动永久后台循环。
- 在快照结构体中预存 `IsStale bool` 静态字段，或让消费者自行硬编码 stale 阈值。
- 在 `MotionControllerSnapshot` 投影时丢失 `Generation` 或 `ErrGenerationChanged`。
- 在 `ErrGenerationChanged` 后继续用旧 sequence 等待或判定 arrived。
- 用"已物理急停"确认按钮关闭急停不可信告警或恢复运动命令。
- 在急停不可信模式下允许 Move/Jog/Home/ApplyConfig 命令排队或执行。
- 独立窗口失联期间静默显示旧状态作为实时状态，或允许发送运动命令。
- 用 `motion.use_legacy_status` 等长期配置开关作为回滚手段（可能重新引入双轮询）；回滚必须基于可逆提交 + 原子 controller ownership 切换。

## Migration Strategy

迁移必须纵向切片，任一步都保持可运行：

1. 驱动兜底：完成 B140 single-flight，保留 WTNMC4A single-flight。
2. 新建 monitor：先与现有直读代码共存但不同时拥有同一 controller；通过构造注入在测试/诊断路径启用，不改变业务。
3. API 切换：`/api/motion/status` 与 Wails status 读取 monitor 缓存。
4. 前端收敛：每进程只保留一条状态源，删除页面级 timer。
5. 校准迁移：等待 monitor sequence；验证四类校准与运动安全。
6. 遍历迁移：运动等待与稳定复检消费 monitor。
7. 清理旧路径：删除/弃用无消费者的 `StartStatusPoller` 和直读 ticker。

迁移期间 controller 注册采用进程内唯一所有权；重复注册必须返回错误并记录当前 owner。monitor 与 legacy poller 不得同时绑定同一 controller，禁止仅靠约定避免双轮询。ownership 切换必须原子：`unregisterControllerOwner(id, "legacy")` 与 `registerControllerOwner(id, "monitor")` 必须在同一锁内完成，禁止中间窗口出现无主状态；启动期若发现重复注册，fail-fast 并拒绝启动 monitor。

## Observability

至少提供以下 debug/metric 字段：

- controller ID/type
- snapshot sequence + generation
- poll mode（moving/fast-grace/idle）
- query duration
- snapshot age / last success age / ValidUntil 剩余时间
- subscriber count
- coalesced refresh count
- status error count / consecutive failures
- skipped/overwritten subscriber snapshot count
- 急停不可信状态（is_locked、acknowledged_at、recovered_at、排队耗时）
- 优先级协调器等待者数量（Critical/High/Normal 分别统计）
- 各优先级等待时长 p50/p95
- 抢占次数 / 放弃 Normal I/O 次数
- generation 切换次数 / `ErrGenerationChanged` 派发次数

慢查询日志只在超过阈值时输出，避免 100ms 轮询刷屏。日志不得包含敏感网络凭据。

## Success Criteria

### Functional

- [ ] B140、WTNMC4A、Simulated 均通过同一 monitor 契约发布状态。
- [ ] 每台已连接控制器同一时刻最多一轮 `Status()` 在途。
- [ ] `StatusAll()`、HTTP/Wails status 请求不再直接触发额外硬件读取。
- [ ] 校准与遍历不再包含周期性 `StatusAll()` 硬件读取循环。
- [ ] 四类校准 Main 与运动/遍历组件只观察 motionStore。
- [ ] Move/Stop/EStop 后能主动触发状态刷新。
- [ ] stale 快照触发 StatusUnavailable，不会提前采集。
- [ ] **Freshness 通过 `FreshnessPolicy` 在调用瞬间计算，不在快照中固化静态 `IsStale` 字段**。
- [ ] **`MotionControllerSnapshot` 包含 `Generation` 字段，重连后 generation 单调递增，sequence 重置为 0**。
- [ ] **`WaitNext` 在 generation 切换时返回 `ErrGenerationChanged`，校准/遍历正确 abort 当前点**。
- [ ] **`NotifyCommandExecuted` 区分 Move/Stop/Config 触发不同 refresh 策略**。
- [ ] **急停超阈值触发 Decision 15 急停不可信协议：命令冻结、告警锁存、SOP 提示、恢复条件**。
- [ ] **独立窗口失联时 UI 显示横幅并禁用运动命令；重连成功后恢复**。

### Performance

- [ ] 3 个并发状态消费者不会把单轮硬件命令数放大到 3 倍。
- [ ] WTNMC4A 192.168.3.141 只读并发测试无 500ms 以上排队放大；若单次硬件调用本身超过 500ms，报告原始 query duration 而非误判为排队。
- [ ] B140 fake server 按模式验收命令预算：四轴寄存器稳定轮为 10 条；四轴编码器稳定轮为 14 条；刚停止时允许每个停止轴额外一次 TD/TP 最终位置刷新；错误重试单独计数。三个并发消费者不得把任一模式预算乘以 3。
- [ ] 空闲时真实控制器目标查询频率不高于 2Hz；运动时调度目标为 10Hz。若单轮硬件读取超过 100ms，不重入、不积压，并报告实际完成频率。
- [ ] WTNMC4A 192.168.3.141 性能门禁固定预热 10 轮、测量 300 轮；3 并发消费者测量 100 批。并发批次 p95 排队增量不得超过单调用 p95 的 25%，且不得出现因重复串行轮询造成的 3 倍调用量。同时报告单调用 p50/p95 与排队增量比，区分底层真实耗时和排队耗时。

### Reliability

- [ ] monitor 关闭、控制器断开、context 取消均无 goroutine 泄漏。
- [ ] 慢消费者不阻塞采集器。
- [ ] 每控制器快照序号单调，Position/Moving/Limit 来自该控制器同一发布轮次；全局聚合视图明确为非原子。
- [ ] 连续通信失败后快照变 stale，校准/遍历按安全策略中断。
- [ ] B140 Stop/EStop 在可控阻塞 fake 下从进入驱动到开始发送硬件命令不超过 600ms，不能被整轮 Status、补偿或下一条状态命令插队阻塞。
- [ ] WTNMC4A 在正常网络和可恢复超时场景记录 Stop/EStop 排队耗时；实机证明 DLL 调用受 200ms timeout 约束后才可把 600ms 从目标提升为保证。超阈值时必须产生 CRITICAL 日志且 UI 不得显示"急停已下发成功"。
- [ ] **急停不可信模式下命令冻结生效：Move/Jog/Home/ApplyConfig 被拒绝，Stop/EStop/Reset/Disconnect 可用**。
- [ ] **急停不可信模式恢复必须同时满足 Disconnect+重连、首帧新鲜快照、操作员显式复位**。
- [ ] **连接优先级协调器满足 Decision 16 六条不变量；Critical 抢占延迟在 B140 上界 600ms 内，WTNMC4A 目标 600ms**。
- [ ] 现有 NoProgress 到位容差修复和 RR0 Moving 修复全部回归测试通过。

### Compatibility

- [ ] `/api/motion/status` 现有消费者无需修改即可继续读取 `[]ControllerStatus`。
- [ ] 现有 motion profiles、Wails bindings、校准/遍历配置 JSON 不发生破坏性变化。
- [ ] WindLabX4 与 motion-controller 两个项目均通过构建和测试。

## Open Questions

以下问题需要人工批准后才能进入 Phase 2：

1. **API 元数据**：首期已按"只供后端和日志"编写。是否还需要新增 `/api/motion/status/snapshot` 暴露 sequence/attemptedAt/succeededAt/stale？推荐暂不新增，避免扩大 API 面。
2. **默认频率**：是否批准 moving=100ms、idle=500ms、命令快速窗口=2s？这些值与现有业务 100ms 运动判断和前端 500ms 心跳对齐。
3. **stale 阈值**：是否批准运动 1s、空闲 2s 的最低阈值？WTNMC4A 设备专属阈值上调必须先有实机只读测试报告区分底层真实耗时和排队耗时，再单独 ADR。
4. **迁移范围**：本轮是否同时改造独立 motion-controller 项目，还是先在 shared + WindLabX4 落地，再单独迁移 motion-controller？推荐同一规格覆盖，按任务分批实现。
5. **状态事件传输**：前端仍轮询缓存 API，还是主窗口恢复 Wails 事件推送？推荐先保留 HTTP 缓存轮询，避免重新引入 Wails 反射序列化故障；观察者模式先在后端内部成立。
6. **`emergency_sop` 配置字段**：物理急停按钮位置与断电程序是否纳入 `motion_profiles` 必填字段？未配置的 profile 是拒绝启动运动任务还是降级为默认 SOP 文案？推荐设为必填，未配置拒绝启动。
7. **独立窗口失联重试间隔**：默认 500ms 是否合适？是否需要指数退避？推荐固定 500ms + 上限 3 次连续失败锁定，避免退避过长延误状态恢复感知。
8. **连接优先级协调器接口形态**：Phase 2 计划是否采用中心等待队列 + 选取器形态，还是基于 channel + 优先级 select？推荐 Phase 2 给出 2-3 个候选实现并对比性能后再定。

## Approval Gate

本文件当前只完成 Phase 1 `SPECIFY`。在人工明确回复“批准规格”或给出修订意见前：

- 不创建 `tasks/plan.md`
- 不创建 `tasks/todo.md`
- 不实现 monitor
- 不迁移校准/遍历/前端轮询

规格批准后进入 Phase 2，使用 `planning-and-task-breakdown` 生成依赖图、实施顺序、风险与验证检查点。
