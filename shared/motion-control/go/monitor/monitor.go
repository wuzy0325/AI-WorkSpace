package monitor

import (
	"context"
	"sort"
	"sync"
	"time"

	"shared.local/device-sdk/go/motion/core"
	"shared.local/device-sdk/go/motion/ports"
)

// Config 是 MotionStatusMonitor 的运行参数。
//
// 设计理由：把"频率"和"快速窗口时长"从 monitor 内部硬编码提取到配置，
// 便于不同部署场景（生产/测试）按硬件实际能力调整，且测试可注入极小值
// 避免 time.Sleep 等真实时间。所有字段必须为正数；DefaultConfig 给出生产默认值。
type Config struct {
	// MovingInterval 是任一轴 Moving/Compensating 时的轮询间隔。
	// 默认 100ms：支持到位判定与安全检查的 10Hz 节奏。
	MovingInterval time.Duration
	// IdleInterval 是全部连接控制器空闲时的轮询间隔。
	// 默认 500ms：UI 心跳节奏，不得超过 stale 阈值，否则空闲快照会被误判过期。
	IdleInterval time.Duration
	// FastWindowInterval 是命令后快速观察窗口期间的轮询间隔。
	// 默认 100ms：与 MovingInterval 一致，避免硬件启动位尚未拉起时误降频。
	FastWindowInterval time.Duration
	// FastWindowDuration 是 CmdKindMove 触发的快速观察窗口持续时长。
	// 默认 2s：覆盖典型运动命令启动到位的时间窗。
	FastWindowDuration time.Duration
}

// DefaultConfig 返回生产默认配置。
//
// 默认值依据 spec Polling Policy 章节：
//   - moving/fast-grace: 100ms
//   - idle: 500ms
//   - 命令快速窗口: 2s
func DefaultConfig() Config {
	return Config{
		MovingInterval:     100 * time.Millisecond,
		IdleInterval:       500 * time.Millisecond,
		FastWindowInterval: 100 * time.Millisecond,
		FastWindowDuration: 2 * time.Second,
	}
}

// MotionStatusMonitor 是位移机构状态监视器。
//
// 核心契约（spec Interface Contract 与 Data Model）：
//   - 每台已连接控制器同一时刻最多一轮 Status() 在途（Decision 2，由单 goroutine + select 自然保证）
//   - 快照不可变；Latest/LatestController 返回深拷贝，消费者修改不影响 monitor 内部状态
//   - Latest/LatestController 不访问硬件，只读内存
//   - FreshnessPolicy 由构造时注入；调用瞬间计算新鲜度，不固化在快照中（Decision 4）
//
// 字段并发约定：
//   - mu 保护 controllers map、subscribers map 与聚合发布元数据（aggregateSequence/publishedAt）
//   - 每个 controllerState 拥有独立 mu，避免单控制器的快照更新阻塞其他控制器的 Latest 查询
//   - 锁顺序：m.mu → controllerState.mu；发布路径同时持有两把锁以保证 lastSnap 与 aggregate 一致
//
// Subscribe send/close 串行化约定：
//   - 所有向订阅者 channel 的 send 与 close 必须在持 m.mu 锁时进行
//   - ctx 取消清理 goroutine 也持 m.mu 后再 delete + close
//   - 这样保证不会出现 send-on-closed：send 与 close 互斥
//
// goroutine 生命周期：
//   - rootCtx/rootCancel: 全局根 context，Shutdown 时取消所有派生 goroutine
//   - 每控制器 pollLoop 运行在独立 goroutine，持有派生 ctx 与 done channel
//   - UnregisterController 取消派生 ctx 并等待 done 关闭，保证返回时 goroutine 已退出
//   - 每订阅者一个清理 goroutine，等待 ctx.Done 后持锁删除并 close channel
type MotionStatusMonitor struct {
	config Config
	clock  Clock
	policy FreshnessPolicy

	rootCtx    context.Context
	rootCancel context.CancelFunc

	mu                sync.RWMutex
	controllers       map[string]*controllerState
	subscribers       map[*subscriber]struct{}
	aggregateSequence uint64
	// publishedAt 是聚合视图最近一次发布时间，由发布路径在写入 lastSnap 时同步更新。
	// Latest 只读取此字段，不现场打时间戳——保证未发布时为零值，消费者可据 PublishedAt.IsZero()
	// 判断 monitor 是否曾发布过任何快照（spec: 未产生首个快照时返回零序号，调用者按 unavailable 处理）。
	publishedAt time.Time
}

// subscriber 是单个订阅者的内部状态。
//
// ch 容量为 1，配合 publish 路径的 drain+send 实现 latest-only 语义：
//   - 新快照到达时先 drain 旧值（若未消费），再 send 新值
//   - 持 m.mu 锁执行，保证多 publish 不并发 drain 同一 channel
//   - ctx 取消时清理 goroutine 持 m.mu 后 close(ch)，与 publish send 互斥，避免 send-on-closed
type subscriber struct {
	ch  chan StatusSnapshot
	ctx context.Context
}

// controllerState 是单台控制器的监视状态。
//
// 拥有独立 mu 而非复用 monitor 顶层锁的设计理由：
//   - 单控制器慢硬件读取不应阻塞其他控制器的 Latest 查询
//   - 发布订阅通道的写入只需持本控制器锁，不持顶层锁
//
// 字段语义：
//   - controller: MotionController 实例，轮询循环调用其 Status()
//   - cancel:     取消本控制器派生 context；UnregisterController 调用
//   - done:       pollLoop 退出时关闭；UnregisterController 等待此 channel 保证同步清理
//   - lastSnap:   最近一次发布的快照；nil 表示首帧未产生
//   - generation: 代际号；Disconnect/ApplyConfig 时递增，旧 generation 在途结果丢弃
//   - sequence:   本 generation 内单调递增的序号
//   - refreshCh:  RequestRefresh 的唤醒信号 channel，容量 1 实现 latest-only 合并
//   - fastWindowUntil: 快速观察窗口的结束时间；CmdKindMove 触发后设为 now + FastWindowDuration，
//     Slice 7 的 pollLoop 据此选择 moving 间隔；零值表示未激活
//   - currentInterval: 当前 pollLoop ticker 的轮询间隔。Slice 7 自适应频率的核心状态：
//     pollLoop 每轮 pollOnce 后通过 computeNextInterval 计算下一间隔，
//     若与 currentInterval 不同则 Stop 旧 ticker + 创建新 ticker + 更新此字段。
//     测试通过此字段白盒断言间隔已切换，避免依赖 time.Sleep 或 FakeTicker 类型断言。
//   - mu:         保护 lastSnap/sequence/generation/fastWindowUntil/currentInterval 及后续扩展字段
type controllerState struct {
	controller ports.MotionController
	cancel     context.CancelFunc
	done       chan struct{}

	mu              sync.Mutex
	lastSnap        *ControllerStatusSnapshot
	generation      uint64
	sequence        uint64
	fastWindowUntil time.Time
	currentInterval time.Duration

	// refreshCh 是 RequestRefresh 投递的唤醒信号。
	// 容量 1 + 非阻塞 send 实现"在途时合并"语义：多次 RequestRefresh 只占一个槽位，
	// pollLoop 从 select 拿到信号后清空槽位，下一轮 pollOnce 才会执行。
	// 不需要单独的 pendingRefresh bool 标志——channel 的"已投递未消费"状态本身就是 pending 位。
	refreshCh chan struct{}
}

// NewMotionStatusMonitor 创建监视器实例。
//
// 参数：
//   - config: 运行参数；零值字段将被替换为 DefaultConfig 对应字段（避免 time.Duration 零值导致除零或 ticker panic）
//   - clock:  时间源；测试注入 FakeClock 避免依赖真实时间
//   - policy: 新鲜度判定策略；消费者通过 FreshnessPolicy() 获取并在调用瞬间计算 Freshness
func NewMotionStatusMonitor(config Config, clock Clock, policy FreshnessPolicy) *MotionStatusMonitor {
	// 零值字段兜底：避免调用方只设置部分字段导致 ticker 间隔为 0（time.NewTicker 会 panic）
	// 或 fast window 立即过期。仅替换零值字段，保留调用方显式设置的值。
	defaults := DefaultConfig()
	if config.MovingInterval <= 0 {
		config.MovingInterval = defaults.MovingInterval
	}
	if config.IdleInterval <= 0 {
		config.IdleInterval = defaults.IdleInterval
	}
	if config.FastWindowInterval <= 0 {
		config.FastWindowInterval = defaults.FastWindowInterval
	}
	if config.FastWindowDuration <= 0 {
		config.FastWindowDuration = defaults.FastWindowDuration
	}

	if clock == nil {
		clock = RealClock{}
	}

	// 根 context 用于 Shutdown 时一次性取消所有 pollLoop goroutine。
	// 不能用 context.Background() 直接派生——Shutdown 需要可取消的根。
	rootCtx, rootCancel := context.WithCancel(context.Background())

	return &MotionStatusMonitor{
		config:      config,
		clock:       clock,
		policy:      policy,
		rootCtx:     rootCtx,
		rootCancel:  rootCancel,
		controllers: make(map[string]*controllerState),
	}
}

// RegisterController 注册一台控制器并启动其唯一轮询 goroutine。
//
// 行为：
//   - 创建派生 context（从 rootCtx 派生），用于 Unregister 时取消本控制器 goroutine
//   - 同步创建 ticker（保证 RegisterController 返回后测试可通过 LatestTicker 取到）
//   - 启动 pollLoop goroutine，按 IdleInterval 轮询（Slice 7 引入自适应频率）
//   - 重复注册：先停止旧 goroutine 再覆盖；ownership 与 fail-fast 留给 Task 7
//
// 并发安全：调用方可在任意时刻注册新控制器；不影响其他控制器的轮询。
func (m *MotionStatusMonitor) RegisterController(id string, controller ports.MotionController) {
	// 检查是否已存在旧 controllerState，若存在先同步停止其 goroutine
	m.mu.Lock()
	old, exists := m.controllers[id]
	m.mu.Unlock()
	if exists {
		m.stopControllerState(old)
	}

	cs := &controllerState{
		controller:      controller,
		done:            make(chan struct{}),
		refreshCh:       make(chan struct{}, 1),
		currentInterval: m.config.IdleInterval,
	}
	ctx, cancel := context.WithCancel(m.rootCtx)
	cs.cancel = cancel

	// 同步创建 ticker：pollLoop goroutine 调度时机不确定，若在 goroutine 内创建，
	// 测试在 RegisterController 返回后立即取 LatestTicker() 会拿到 nil。
	// 在主流程创建保证 RegisterController 返回时 ticker 已注册到 FakeClock。
	// 初始间隔为 IdleInterval：首帧前无 Moving 信息可用，用保守的 idle 间隔；
	// 首帧后 pollLoop 会根据 fastWindow/Moving 自适应切换（Slice 7）。
	ticker := m.clock.NewTicker(m.config.IdleInterval)

	m.mu.Lock()
	m.controllers[id] = cs
	m.mu.Unlock()

	go m.pollLoop(ctx, id, cs, ticker)
}

// UnregisterController 停止指定控制器的轮询 goroutine 并移除注册。
//
// 同步语义：返回时该控制器的 pollLoop goroutine 已退出（通过 done channel 等待）。
// 这让测试可以确定性断言"Unregister 后无新 Status() 调用"，无需 time.Sleep。
//
// 未注册的 ID 视为 no-op，不报错——上层 manager 可能并发调用 Disconnect 与 Unregister。
func (m *MotionStatusMonitor) UnregisterController(id string) {
	m.mu.Lock()
	cs, ok := m.controllers[id]
	if ok {
		delete(m.controllers, id)
	}
	m.mu.Unlock()

	if ok {
		m.stopControllerState(cs)
	}
}

// Shutdown 取消根 context 并等待所有 pollLoop goroutine 退出。
//
// 调用 Shutdown 后不得再注册新控制器（rootCtx 已取消，新派生 ctx 立即生效为取消态）。
// 可重复调用，幂等。
func (m *MotionStatusMonitor) Shutdown() {
	m.rootCancel()

	// 收集所有 controllerState 的 done channel，等待全部退出
	m.mu.Lock()
	states := make([]*controllerState, 0, len(m.controllers))
	for _, cs := range m.controllers {
		states = append(states, cs)
	}
	m.controllers = make(map[string]*controllerState)
	m.mu.Unlock()

	for _, cs := range states {
		<-cs.done
	}
}

// stopControllerState 取消 ctx 并等待 goroutine 退出。
// UnregisterController 与 RegisterController（覆盖旧条目）共用此清理路径，
// 保证 goroutine 同步退出，避免泄漏。
func (m *MotionStatusMonitor) stopControllerState(cs *controllerState) {
	cs.cancel()
	<-cs.done
}

// pollLoop 是单控制器轮询主循环。
//
// 设计要点：
//   - 单 goroutine + select 自然保证 single-flight（spec Decision 2）：
//     goroutine 在 Status() 中阻塞时无法接收新 tick，FakeTicker 缓冲为 1，
//     新 tick 被丢弃，不积压
//   - ticker 由 RegisterController 同步创建后传入，保证测试可在 Register 后立即发现；
//     Slice 7 自适应频率：每轮 pollOnce 后检查间隔，变化则 Stop 旧 ticker + 创建新 ticker
//   - ctx 取消立即返回（UnregisterController/Shutdown 路径）
//   - defer close(done) 让 Unregister 同步等待 goroutine 退出
//   - defer ticker.Stop() 兜底关闭最后一个 ticker（前面的已 Stop，幂等）
//   - refreshCh 与 ticker.C 平等参与 select——Go select 随机选择就绪 case，
//     两者都就绪时各 50% 概率，RequestRefresh 不会饿死周期轮询
//
// Slice 7 自适应频率：
//   - 每轮 pollOnce 完成后调用 computeNextInterval 计算下一间隔
//   - 若与 currentInterval 不同，Stop 旧 ticker、创建新 ticker、更新 currentInterval
//   - 间隔切换在 pollOnce 之后而非之前——本轮用旧间隔采集，下一轮用新间隔
//   - 这样 fastWindow 激活（Notify Move）后，refresh 触发的 pollOnce 完成即切换；
//     fastWindow 过期后，下一轮 pollOnce 完成即回落
func (m *MotionStatusMonitor) pollLoop(ctx context.Context, id string, cs *controllerState, ticker Ticker) {
	defer close(cs.done)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			m.pollOnce(ctx, id, cs)
		case <-cs.refreshCh:
			// RequestRefresh 唤醒：立即执行一轮额外采集。
			// 多次 RequestRefresh 在途时合并为 1 个槽位（channel 容量 1），
			// pollLoop 拿到信号后清空槽位，下一轮 RequestRefresh 才能再次投递。
			m.pollOnce(ctx, id, cs)
		}

		// Slice 7: 每轮 pollOnce 后检查并切换 ticker 间隔
		// 必须在 pollOnce 之后——本轮采集用旧间隔，下一轮用新间隔
		next := m.computeNextInterval(cs)
		cs.mu.Lock()
		if cs.currentInterval == next {
			cs.mu.Unlock()
			continue
		}
		cs.currentInterval = next
		cs.mu.Unlock()

		// 切换 ticker：先创建新 ticker，再 Stop 旧 ticker。
		// 顺序很重要——先 Stop 再创建会有窗口期 LatestTicker 返回 nil，
		// 测试持有的旧 ticker 引用 Fire 时转发失败，tick 丢失（flaky）。
		// 先创建新 ticker 保证 LatestTicker 始终能找到未停止的 ticker，
		// 配合 FakeTicker.Fire 始终转发到 LatestTicker 的语义，
		// 测试旧引用 Fire 不会丢 tick。生产代码不 Fire ticker，顺序不影响生产行为。
		newTicker := m.clock.NewTicker(next)
		ticker.Stop()
		ticker = newTicker
	}
}

// computeNextInterval 根据当前状态计算下一轮轮询间隔。
//
// 优先级（spec Polling Policy）：
//  1. fastWindow 激活（fastWindowUntil > now）→ FastWindowInterval
//     覆盖运动命令启动到位的时间窗，避免硬件启动位尚未拉起时误降频
//  2. 任一轴 Moving → MovingInterval
//     支持到位与安全判定的 10Hz 节奏
//  3. 否则 → IdleInterval
//     UI 位置与限位心跳，不得超过 stale 阈值
//
// fastWindow 优先于 Moving 的理由：spec NotifyCommandExecuted 明确
// "快速窗口期间轮询频率按 Polling Policy 的 moving 间隔执行"——但 fastWindow
// 与 Moving 通常同时发生（Move 命令后轴开始 Moving）。当 Moving=false 但 fastWindow
// 激活时（命令刚下发，硬件 Moving 位尚未拉起），仍需用快速间隔避免误降频。
// 当 fastWindow 过期但 Moving=true 时，回落到 MovingInterval（与 FastWindowInterval
// 默认值相同，但语义不同，测试用不同值区分）。
//
// 首帧前 lastSnap=nil：无 Moving 信息，fastWindow 未激活则返回 IdleInterval，
// 与 RegisterController 的初始间隔一致——首帧前用保守 idle 间隔。
func (m *MotionStatusMonitor) computeNextInterval(cs *controllerState) time.Duration {
	now := m.clock.Now()
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// 1. fastWindow 激活优先
	if cs.fastWindowUntil.After(now) {
		return m.config.FastWindowInterval
	}

	// 2. 任一轴 Moving（基于最近一次成功快照）
	if cs.lastSnap != nil {
		for i := range cs.lastSnap.Status.Axes {
			if cs.lastSnap.Status.Axes[i].Moving {
				return m.config.MovingInterval
			}
		}
	}

	// 3. 否则 idle
	return m.config.IdleInterval
}

// pollOnce 执行单轮采集并发布快照。
//
// 步骤：
//  1. 捕获 generation（用于检测 Status() 期间发生的 generation 变化）
//  2. 调用 controller.Status(ctx)——慢 I/O，不持任何锁
//  3. 持 m.mu + cs.mu 写入 lastSnap + 递增 sequence
//     （双锁保证 lastSnap 与 aggregateSequence 一致，避免 Latest 看到新 lastSnap + 旧 sequence）
//  4. 释放 cs.mu（仍持 m.mu），调用 publishToSubscribersLocked 推送聚合视图
//     ——必须释放 cs.mu 因为 publishToSubscribersLocked 内部 collectSnapshotLocked 会重新加 cs.mu，
//     Go sync.Mutex 不可重入，不释放会自死锁
//  5. 释放 m.mu
//
// generation 变化处理：spec Data Model 重连语义——旧 generation 的在途结果直接丢弃，
// 不更新 lastSnap 也不递增 sequence。典型触发场景：Status() 期间发生 Disconnect/ApplyConfig。
func (m *MotionStatusMonitor) pollOnce(ctx context.Context, id string, cs *controllerState) {
	cs.mu.Lock()
	gen := cs.generation
	cs.mu.Unlock()

	// 慢 I/O 不持锁，避免阻塞其他控制器的 Latest 查询
	status, err := cs.controller.Status(ctx)
	if ctx.Err() != nil {
		// ctx 取消（Unregister/Shutdown）：丢弃结果，不发布
		return
	}

	now := m.clock.Now()

	// 双锁发布：m.mu → cs.mu（锁顺序与 Latest 一致，无死锁风险）
	m.mu.Lock()
	defer m.mu.Unlock()

	cs.mu.Lock()
	if cs.generation != gen {
		// generation 已变化（Disconnect/ApplyConfig）；丢弃旧 generation 的在途结果
		cs.mu.Unlock()
		return
	}

	cs.sequence++
	seq := cs.sequence

	snap := &ControllerStatusSnapshot{
		ControllerID: id,
		Generation:   gen,
		Sequence:     seq,
		AttemptedAt:  now,
		Status:       status,
		Err:          err,
	}
	if err == nil {
		snap.SucceededAt = now
	} else if cs.lastSnap != nil {
		// 失败时保留上一轮 SucceededAt（spec Failure Semantics）：
		// 单次失败不立即推翻可信状态，Freshness 仍按 SucceededAt 判定新鲜度
		snap.SucceededAt = cs.lastSnap.SucceededAt
	}
	cs.lastSnap = snap
	// 显式释放 cs.mu：publishToSubscribersLocked 内部 collectSnapshotLocked 会重新加 cs.mu，
	// Go sync.Mutex 不可重入，必须先释放。lastSnap 已写入，后续 Latest/LatestController 持
	// m.mu.RLock + cs.mu.Lock 读到的都是新值，一致性由 m.mu 保证。
	cs.mu.Unlock()

	// 聚合视图发布版本号与时间戳：每次有控制器快照更新时递增
	m.aggregateSequence++
	m.publishedAt = now

	// 向所有订阅者推送最新聚合视图（latest-only：drain 旧值 + send 新值）
	// 必须在持 m.mu 时进行，与 Subscribe 的 close 互斥，避免 send-on-closed
	m.publishToSubscribersLocked()
}

// publishToSubscribersLocked 在调用方持 m.mu 时向所有订阅者推送最新聚合视图。
//
// latest-only 语义实现：
//  1. 构造聚合视图（深拷贝保证订阅者修改不影响 monitor 内部状态）
//  2. 对每个订阅者 channel：先非阻塞 drain 旧值（若未消费），再非阻塞 send 新值
//
// 持锁保证 drain+send 是原子的：多 publish 不会并发 drain 同一 channel，
// 订阅者看到的总是"上一轮 drain 后的下一帧新值"，不会丢失最新。
//
// 慢订阅者不阻塞采集循环：drain 与 send 全部用 select default，最坏情况丢弃新值
// （但 drain 已先腾空，正常情况新值总能 send 成功）。
func (m *MotionStatusMonitor) publishToSubscribersLocked() {
	if len(m.subscribers) == 0 {
		return
	}
	snap := m.collectSnapshotLocked()
	for sub := range m.subscribers {
		// drain 旧值：若订阅者未消费上一帧，先非阻塞取出丢弃
		// 这是 latest-only 的关键——不 drain 直接 send 会被阻塞或丢弃新值
		select {
		case <-sub.ch:
		default:
		}
		// send 新值：channel 容量 1，drain 后必为空，send 应立即成功
		// default 兜底防止极端竞态（如 Subscribe 期间已投递但未消费）
		select {
		case sub.ch <- snap:
		default:
		}
	}
}

// FreshnessPolicy 返回构造时注入的新鲜度策略。
//
// 消费者调用 m.FreshnessPolicy().Freshness(now, snap) 在调用瞬间计算 Freshness，
// 而非依赖快照中固化的 IsStale 字段（Decision 4：Age 是时间敏感值，发布时固化会立即失真）。
func (m *MotionStatusMonitor) FreshnessPolicy() FreshnessPolicy {
	return m.policy
}

// Latest 返回所有控制器最新快照的聚合视图，深拷贝保证消费者修改不影响 monitor 内部状态。
//
// 语义：
//   - 不访问硬件，只读内存（spec Interface Contract: Latest）
//   - 仅包含已产生首帧的控制器；未首帧的控制器不出现在 Controllers 列表中
//   - 顺序按 ControllerID 字典序稳定，便于测试断言
//   - PublishedAt 与 Sequence 均由 Slice 3 发布路径在写入 lastSnap 时同步更新；
//     Latest 只读，不现场打时间戳——保证未发布时为零值，消费者可据 PublishedAt.IsZero()
//     判断 monitor 是否曾发布过任何快照
func (m *MotionStatusMonitor) Latest() StatusSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.collectSnapshotLocked()
}

// collectSnapshotLocked 在调用方持 m.mu（RLock 或 Lock）时构造聚合视图。
//
// 提取此方法的理由：Subscribe 立即投递当前快照与 Latest 共用同一构造逻辑，
// 避免重复实现导致行为漂移。深拷贝由 deepCopyControllerSnapshot 保证。
//
// 注意：本方法会获取每个 controllerState.mu，调用方必须仅持 m.mu，
// 不能持任何 controllerState.mu（锁顺序 m.mu → cs.mu，反序会死锁）。
func (m *MotionStatusMonitor) collectSnapshotLocked() StatusSnapshot {
	ids := make([]string, 0, len(m.controllers))
	for id := range m.controllers {
		ids = append(ids, id)
	}
	aggregateSeq := m.aggregateSequence
	publishedAt := m.publishedAt

	// 字典序保证顺序稳定；monitor 不依赖控制器注册顺序，测试也不应假设顺序
	sort.Strings(ids)

	snap := StatusSnapshot{
		Sequence:    aggregateSeq,
		PublishedAt: publishedAt,
		Controllers: make([]ControllerStatusSnapshot, 0, len(ids)),
	}
	for _, id := range ids {
		cs := m.controllers[id]
		if cs == nil {
			continue
		}
		cs.mu.Lock()
		if cs.lastSnap == nil {
			cs.mu.Unlock()
			continue
		}
		snap.Controllers = append(snap.Controllers, deepCopyControllerSnapshot(*cs.lastSnap))
		cs.mu.Unlock()
	}
	return snap
}

// LatestController 返回指定控制器的最新快照，深拷贝保证消费者修改不影响 monitor 内部状态。
//
// 返回值：
//   - (zero, false): 控制器未注册，或已注册但首帧未产生
//   - (snap, true):  控制器已注册且有首帧
//
// 不访问硬件，只读内存（spec Interface Contract: LatestController）。
func (m *MotionStatusMonitor) LatestController(id string) (ControllerStatusSnapshot, bool) {
	m.mu.RLock()
	cs := m.controllers[id]
	m.mu.RUnlock()
	if cs == nil {
		return ControllerStatusSnapshot{}, false
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.lastSnap == nil {
		return ControllerStatusSnapshot{}, false
	}
	return deepCopyControllerSnapshot(*cs.lastSnap), true
}

// Subscribe 订阅聚合视图快照流，返回 latest-only 的 channel。
//
// 行为契约（spec Subscribe）：
//   - 首次订阅立即投递当前快照（若存在）。首帧未产生时不投递，等下次 publish 推送。
//   - channel 容量为 1，配合 publish 路径的 drain+send 实现 latest-only 语义：
//     新快照到达时若旧快照未消费，先 drain 旧值再 send 新值，订阅者最终看到最新。
//   - 慢订阅者不阻塞采集循环：所有 send 都在持 m.mu 时非阻塞执行，drain 也是非阻塞。
//   - context 取消后关闭订阅 channel 并从 subscribers map 移除——清理 goroutine 持 m.mu 后 delete+close，
//     与 publish 路径的 send 互斥，保证不会 send-on-closed。
//
// 使用场景区分（spec Subscribe）：Subscribe 适合 UI 显示聚合视图；
// 业务安全判定不得使用 Subscribe，必须用 LatestController + WaitNext 按单控制器 sequence/freshness 判断。
func (m *MotionStatusMonitor) Subscribe(ctx context.Context) <-chan StatusSnapshot {
	ch := make(chan StatusSnapshot, 1)
	sub := &subscriber{ch: ch, ctx: ctx}

	m.mu.Lock()
	if m.subscribers == nil {
		m.subscribers = make(map[*subscriber]struct{})
	}
	m.subscribers[sub] = struct{}{}

	// 立即投递当前快照（若存在）——持锁投递，与 publish/close 互斥，避免 send-on-closed
	snap := m.collectSnapshotLocked()
	if len(snap.Controllers) > 0 {
		select {
		case ch <- snap:
		default:
			// 极端情况：channel 已满（理论不应发生在 Subscribe 期间，但兜底不阻塞）
		}
	}
	m.mu.Unlock()

	// 清理 goroutine：等待 ctx 取消后持锁删除并 close channel
	// 必须持锁 close，与 publish 路径的 send 互斥，否则会 send-on-closed panic
	go func() {
		<-ctx.Done()
		m.mu.Lock()
		// 双重检查：可能已被其他路径删除（如 Shutdown）；若仍在 map 中则 close
		if _, ok := m.subscribers[sub]; ok {
			delete(m.subscribers, sub)
			close(ch)
		}
		m.mu.Unlock()
	}()

	return ch
}

// RequestRefresh 请求指定控制器尽快产生一轮新快照。
//
// 行为契约（spec RequestRefresh）：
//   - 非阻塞：通过 refreshCh 容量 1 + 非阻塞 send 实现，调用方立即返回
//   - 幂等、可合并：在途期间多次调用只占一个槽位，pollLoop 拿到信号后清空，
//     下一轮 pollOnce 完成即视为已处理，不会触发 N 次补轮
//   - 不触发 2s 快速窗口：本方法只投递 refreshCh 信号，不修改 ticker 频率；
//     快速窗口由 NotifyCommandExecuted (Slice 6) 管理
//   - 未注册 ID 静默返回：上层 manager 可能并发调用 Disconnect 与 Unregister，
//     对已注销 ID 报错会引入伪故障
//
// 与周期轮询共存：refreshCh 与 ticker.C 平等参与 pollLoop 的 select，
// Go select 随机选择就绪 case，RequestRefresh 不会饿死周期轮询（spec: "连续请求不得无限推迟既定周期轮询"）。
func (m *MotionStatusMonitor) RequestRefresh(id string) {
	m.mu.RLock()
	cs := m.controllers[id]
	m.mu.RUnlock()
	if cs == nil {
		// 未注册或已注销——静默返回，避免上层并发注销引发伪故障
		return
	}
	// 非阻塞 send：channel 容量 1，满则丢弃新信号（合并语义）
	// 这保证调用方绝不阻塞，且在途期间多次调用只占一个槽位
	select {
	case cs.refreshCh <- struct{}{}:
	default:
	}
}

// NotifyCommandExecuted 通知 monitor 一条运动命令已执行，触发相应的刷新策略。
//
// 行为契约（spec NotifyCommandExecuted）：
//   - CmdKindMove:  设置 fastWindowUntil = now + FastWindowDuration（默认 2s）+ 触发 1 轮 refresh
//     快速窗口让 Slice 7 的自适应频率切换到 moving 间隔（100ms），覆盖典型运动启动到位时间窗
//   - CmdKindStop:  仅触发 1 轮 refresh，不修改 fastWindowUntil——避免 Stop 后高频采集放大硬件压力
//   - CmdKindConfig: 递增 generation + 清空 lastSnap + 重置 sequence + 触发 1 轮 refresh
//     generation 变化让在途的旧 generation 结果被丢弃；新 generation 首帧由 refresh 触发产生
//
// 三种命令共享 refresh 触发路径（refreshCh send），与 RequestRefresh 合并语义一致：
// 若 refreshCh 已有未消费信号，本次 send 被丢弃，但快速窗口/generation 重置等副作用已生效。
func (m *MotionStatusMonitor) NotifyCommandExecuted(id string, cmdKind CommandKind) {
	m.mu.RLock()
	cs := m.controllers[id]
	m.mu.RUnlock()
	if cs == nil {
		// 未注册或已注销——静默返回
		return
	}

	now := m.clock.Now()
	cs.mu.Lock()
	switch cmdKind {
	case CmdKindMove:
		// 每次 Move 都重置 fastWindowUntil（非 max）——连续运动命令不会让窗口意外过期
		cs.fastWindowUntil = now.Add(m.config.FastWindowDuration)
	case CmdKindStop:
		// 不修改 fastWindowUntil——若此前 Move 设置的窗口仍在生效，保留剩余时间
		// spec: "Stop/EStop/Reset 命令仅触发单轮 refresh（不进入快速窗口）"
	case CmdKindConfig:
		// generation 单调递增；sequence 重置为 0；清空 lastSnap 避免消费者读到 stale generation 数据
		// 在途的旧 generation pollOnce 结果会被 generation 检查丢弃（pollOnce 中的 gen != cs.generation 路径）
		cs.generation++
		cs.sequence = 0
		cs.lastSnap = nil
	}
	cs.mu.Unlock()

	// 触发一轮 refresh——与 RequestRefresh 共用 refreshCh，自动合并
	select {
	case cs.refreshCh <- struct{}{}:
	default:
	}
}

// controllerStateLocked 是白盒测试辅助方法，返回指定控制器的内部状态指针。
//
// 仅供同包测试使用：测试需直接注入 lastSnap 验证深拷贝语义，无需启动轮询循环。
// 生产代码禁止调用。
//
// 实现说明：方法名沿用 Go 标准库约定（Locked 后缀），但因测试不持 m.mu，
// 此方法内部获取并释放 m.mu.RLock，确保 map 读取的内存可见性。
// 返回的 *controllerState 是堆指针，map 删除后仍可用，不存在悬挂引用风险。
func (m *MotionStatusMonitor) controllerStateLocked(id string) *controllerState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.controllers[id]
}

// deepCopyControllerSnapshot 深拷贝单控制器快照。
//
// ControllerStatus 唯一的引用类型字段是 Axes []AxisStatus；
// AxisStatus 内部全部为值类型字段，故切片浅拷贝即可保证不可变性。
// 其他字段（ID/Name/Connected 等）均为值类型，结构体值复制自然深拷贝。
func deepCopyControllerSnapshot(s ControllerStatusSnapshot) ControllerStatusSnapshot {
	if s.Status.Axes != nil {
		axesCopy := make([]core.AxisStatus, len(s.Status.Axes))
		copy(axesCopy, s.Status.Axes)
		s.Status.Axes = axesCopy
	}
	return s
}
