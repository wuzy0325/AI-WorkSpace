// Package monitor 提供位移机构统一状态监视与订阅能力。
//
// 设计依据：spec-motion-status-monitor.md
// 核心契约：
//   - 每台已连接控制器同一时刻最多一轮 Status() 在途（Decision 2）
//   - 快照不可变，包含 Sequence/Generation/AttemptedAt/SucceededAt/ValidUntil 元数据（Decision 4）
//   - 新鲜度是安全契约：过期快照不得当作实时状态用于到位/安全判定（Decision 4）
//   - Freshness 不固化在快照中：Age/IsStale 是时间敏感的瞬时值，
//     发布瞬间写入会立即失真，必须通过 FreshnessPolicy 在调用瞬间计算（Decision 4 修订）
//   - Generation/Sequence 重连语义：Disconnect/ApplyConfig 递增 Generation，
//     Sequence 重置为 0，旧 generation 在途结果直接丢弃（Data Model 重连语义）
package monitor

import (
	"sync"
	"time"

	"shared.local/device-sdk/go/motion/core"
	"shared.local/device-sdk/go/pkg/slog"
)

// ControllerStatusSnapshot 表示一台控制器的一次完整观察结果。
//
// 字段语义：
//   - ControllerID: 控制器实例 ID，对应 MotionControllerProfile.ID
//   - Generation:   代际号，Disconnect/ApplyConfig 时单调递增；用于丢弃旧 generation 的在途结果
//   - Sequence:     每控制器本 generation 内单调递增的序号；每轮采集尝试（含失败）后递增
//   - AttemptedAt:   本轮采集开始时间；无论成功失败都更新
//   - SucceededAt:   本轮采集成功时间；失败时保持上一轮值不变（保留最后可信采样时间）
//   - ValidUntil:    本快照被认为新鲜的上限时间；由 FreshnessPolicy 计算时使用
//   - Status:        控制器业务状态（Position/Moving/Limit 等）；失败时保留上一轮可信值
//   - Err:           本轮采集错误；nil 表示成功
//
// 不可变性约束：发布和返回前必须深拷贝 Status 与 Axes，消费者不得修改 monitor 内部缓存。
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

// StatusSnapshot 是各控制器最新快照的聚合视图。
//
// 重要语义：聚合视图明确是"各控制器最新值"，不承诺不同控制器来自同一采样时刻。
// 校准/遍历按 controller ID 使用对应 ControllerStatusSnapshot 的 sequence/freshness，
// 不得依赖跨控制器原子同采样。
//
// 字段语义：
//   - Sequence:    聚合视图发布版本号，每次有控制器快照更新时递增；与单控制器 Sequence 不同
//   - PublishedAt: 聚合视图发布时间
//   - Controllers: 各控制器最新快照列表；顺序由 monitor 内部维护
type StatusSnapshot struct {
	Sequence    uint64
	PublishedAt time.Time
	Controllers []ControllerStatusSnapshot
}

// Freshness 描述快照的新鲜度。
//
// 不可变性约束：Freshness 是调用瞬间的计算结果，不固化在快照中。
// 发布瞬间写入 Age/IsStale 会立即失真（发布瞬间 Age≈0，之后无新发布时永远显示 fresh）。
// 消费者必须通过 FreshnessPolicy.Freshness(now, snap) 在调用瞬间计算。
type Freshness struct {
	// Age 是 now - SucceededAt；表示距离上次成功采集的时长。
	Age time.Duration
	// IsStale 由 policy 按运动/空闲阈值判定；true 表示快照已过期，不得用于到位/安全判定。
	IsStale bool
}

// FreshnessPolicy 是 monitor 注入的新鲜度判定策略。
//
// 设计理由：消费者禁止自行硬编码 stale 阈值，必须通过注入的 FreshnessPolicy 计算。
// 这样 monitor 可以根据运动/空闲状态动态切换阈值，且测试可以注入 fake policy。
//
// 实现要求：
//   - ValidUntil 由 monitor 在发布时计算写入（Task 6 实现，按运动/空闲状态动态切换阈值），
//     策略只需比较 now 与 ValidUntil 即可判定 IsStale
//   - Task 5 期间 monitor 尚未写入 ValidUntil（零值），DefaultFreshnessPolicy 走 StaleThreshold 兜底路径
//   - 若需要按业务上下文（运动/空闲）动态切换阈值，可通过 policy 自行实现（Task 6 默认策略的预期行为）
//   - Err != nil 但 SucceededAt 仍新鲜时 IsStale 应为 false（保留最后可信状态用于诊断）
type FreshnessPolicy interface {
	// Freshness 在调用瞬间根据当前时钟和快照元数据计算新鲜度。
	// 返回值 Age 是 now-SucceededAt；IsStale 由 policy 按运动/空闲阈值判定。
	Freshness(now time.Time, snap ControllerStatusSnapshot) Freshness
}

// DefaultFreshnessPolicy 是默认的新鲜度判定策略。
//
// 判定规则：
//   - IsStale = now > ValidUntil（ValidUntil 由 monitor 在发布时计算写入，Task 6 实现）
//   - Age = now - SucceededAt（SucceededAt 为零值时 Age=0 且 IsStale=true）
//   - Err != nil 不影响 IsStale：保留最后可信状态用于诊断，但 monitor 应在连续失败后
//     将 ValidUntil 提前，使快照自然变 stale（Task 6 实现）
//
// StaleThreshold 是兜底阈值：当 ValidUntil 为零值（首帧未产生，或 Task 5 期间 monitor 尚未写入）
// 时用 StaleThreshold 判定。
type DefaultFreshnessPolicy struct {
	// StaleThreshold 是兜底 stale 阈值；ValidUntil 为零值时使用。
	// 监视器应在发布时根据运动/空闲状态计算 ValidUntil，不依赖此字段。
	StaleThreshold time.Duration
}

// Freshness 实现 FreshnessPolicy 接口。
func (p DefaultFreshnessPolicy) Freshness(now time.Time, snap ControllerStatusSnapshot) Freshness {
	// SucceededAt 为零值（首帧未产生）：强制 stale，防止消费者把"未启动"状态当作实时状态
	if snap.SucceededAt.IsZero() {
		return Freshness{Age: 0, IsStale: true}
	}

	age := now.Sub(snap.SucceededAt)

	// ValidUntil 已写入：比较 now 与 ValidUntil
	if !snap.ValidUntil.IsZero() {
		return Freshness{Age: age, IsStale: now.After(snap.ValidUntil)}
	}

	// ValidUntil 为零值（兼容路径）：用 StaleThreshold 兜底
	if p.StaleThreshold <= 0 {
		// 无阈值：永远 fresh。仅用于测试，生产环境误用会让过期数据永远被当作新鲜，
		// 安全契约（Decision 4）失效。通过 once + slog.Warn 提醒一次，避免日志洪泛。
		defaultFreshnessZeroThresholdWarnOnce.Do(func() {
			slog.Warn("monitor: DefaultFreshnessPolicy.StaleThreshold<=0 — 快照永远 fresh，仅测试可用，生产环境请设置 ValidUntil 或 StaleThreshold",
				"controller_id", snap.ControllerID)
		})
		return Freshness{Age: age, IsStale: false}
	}
	return Freshness{Age: age, IsStale: age > p.StaleThreshold}
}

// defaultFreshnessZeroThresholdWarnOnce 确保 StaleThreshold<=0 的告警在每个进程只打一次，
// 避免 monitor 高频调用 Freshness 时刷屏。
var defaultFreshnessZeroThresholdWarnOnce sync.Once
