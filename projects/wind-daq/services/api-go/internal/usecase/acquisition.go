// acquisition.go 数据采集 hub，负责多设备数据的内存索引与订阅分发。
//
// 设计要点（1kHz × 10 设备场景优化）：
//   - 按 deviceId 哈希分片（16 shard），消除单 mutex 争用
//   - historyByDevice 使用环形缓冲区，避免 slice append 的整段 copy
//   - publishHz 用 atomic.Int64（纳秒间隔）存储，OnData 内 lock-free 读取
//   - 订阅者列表按 deviceId 分片到同一 shard，与 latest/history 共用同一把锁
package usecase

import (
	"fmt"
	"shared.local/device-sdk/go/pkg/slog"
	"sync"
	"sync/atomic"
	"time"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/ports"
)

const (
	minPublishHz           = 1.0
	maxPublishHz           = 500.0 // 从 100 提到 500，覆盖 1kHz 设备的 1/2 采样率直送场景
	defaultHistoryCapacity = 256
	// 订阅者缓冲区满导致丢包时，至多每 dropLogInterval 输出一条聚合日志，
	// 避免高采样率（如 1 kHz）设备遇到慢订阅者时按设备速率刷屏。
	dropLogInterval = 5 * time.Second
	// hubShardCount 分片数量，必须为 2 的幂以用位与替代取模
	hubShardCount = 16
	hubShardMask  = hubShardCount - 1
)

// historyRing 环形缓冲区，替代 slice append。
// 写入时直接覆盖最旧元素，无需整段 copy；读取时按时间升序返回最近 N 条。
type historyRing struct {
	data []device.DataPayload
	head int // 下一个写入位置
	size int // 已写入数量（<= cap）
	cap  int
}

func newHistoryRing(capacity int) *historyRing {
	if capacity < 1 {
		capacity = 1
	}
	return &historyRing{
		data: make([]device.DataPayload, capacity),
		cap:  capacity,
	}
}

// push 写入一个新元素，覆盖最旧元素（如果已满）
func (r *historyRing) push(p device.DataPayload) {
	r.data[r.head] = p
	r.head = (r.head + 1) % r.cap
	if r.size < r.cap {
		r.size++
	}
}

// recent 返回最近 limit 条记录，按时间升序（最旧 -> 最新）
func (r *historyRing) recent(limit int) []device.DataPayload {
	if limit <= 0 || r.size == 0 {
		return nil
	}
	if limit > r.size {
		limit = r.size
	}
	result := make([]device.DataPayload, limit)
	// 最旧的元素索引 = (head - size + cap) % cap
	oldest := (r.head - r.size + r.cap) % r.cap
	// 跳过前 (size - limit) 个，从倒数 limit 个开始
	startIdx := (oldest + (r.size - limit)) % r.cap
	for i := 0; i < limit; i++ {
		idx := (startIdx + i) % r.cap
		result[i] = r.data[idx]
	}
	return result
}

// acquisitionShard 单个分片，保护一组 deviceId 的 latest/history/lastPublishAt/subscribers。
// 同一 deviceId 的所有状态总是落在同一 shard，避免跨 shard 加锁。
type acquisitionShard struct {
	mu            sync.RWMutex
	latest        map[string]device.DataPayload
	history       map[string]*historyRing
	lastPublishAt map[string]time.Time
	subscribers   map[string]map[chan device.DataPayload]struct{}
}

// shardIndexForDevice 用 FNV-1a 哈希把 deviceId 映射到 [0, hubShardCount) 范围。
// 选 FNV-1a 是因为对短字符串（deviceId 通常 < 32 字节）分布良好且无外部依赖。
func shardIndexForDevice(deviceID string) int {
	var h uint32 = 2166136261 // FNV offset basis
	for i := 0; i < len(deviceID); i++ {
		h ^= uint32(deviceID[i])
		h *= 16777619 // FNV prime
	}
	return int(h & hubShardMask)
}

// AcquisitionHub 多设备数据采集 hub。
// 通过 16 分片降低锁争用，支撑 1kHz × 10 设备的并发写入。
type AcquisitionHub struct {
	publisher ports.Publisher

	// publishIntervalNs 用 atomic 存储 publishHz 对应的纳秒间隔，
	// 让 OnData 在不持锁的情况下读取，避免高频路径上的锁开销。
	publishIntervalNs atomic.Int64

	historyCapacity int

	shards [hubShardCount]acquisitionShard

	// drop 节流日志的全局状态（丢弃事件低频，全局 mutex 足够）
	dropMu        sync.Mutex
	dropCount     map[string]int
	dropLastLogAt map[string]time.Time
}

func NewAcquisitionHub(publisher ports.Publisher, publishHz float64) *AcquisitionHub {
	return NewAcquisitionHubWithHistoryCapacity(publisher, publishHz, defaultHistoryCapacity)
}

func NewAcquisitionHubWithHistoryCapacity(publisher ports.Publisher, publishHz float64, historyCapacity int) *AcquisitionHub {
	if publishHz < minPublishHz || publishHz > maxPublishHz {
		publishHz = 20
	}
	if historyCapacity < 1 {
		historyCapacity = defaultHistoryCapacity
	}
	hub := &AcquisitionHub{
		publisher:       publisher,
		historyCapacity: historyCapacity,
		dropCount:       make(map[string]int),
		dropLastLogAt:   make(map[string]time.Time),
	}
	for i := range hub.shards {
		hub.shards[i] = acquisitionShard{
			latest:        make(map[string]device.DataPayload),
			history:       make(map[string]*historyRing),
			lastPublishAt: make(map[string]time.Time),
			subscribers:   make(map[string]map[chan device.DataPayload]struct{}),
		}
	}
	hub.setPublishInterval(publishHz)
	return hub
}

// setPublishInterval 把 publishHz 转换为纳秒间隔并存入 atomic。
// publishHz 越高，间隔越短；上限 500Hz 对应 2ms 间隔。
func (h *AcquisitionHub) setPublishInterval(hz float64) {
	if hz < minPublishHz {
		hz = minPublishHz
	}
	if hz > maxPublishHz {
		hz = maxPublishHz
	}
	interval := time.Duration(float64(time.Second) / hz)
	h.publishIntervalNs.Store(interval.Nanoseconds())
}

// publishHzLocked 返回当前 publishHz。仅用于构造和初始化，不在热路径调用。
func (h *AcquisitionHub) publishHzLocked() float64 {
	ns := h.publishIntervalNs.Load()
	if ns <= 0 {
		return 0
	}
	return float64(time.Second) / float64(ns)
}

// OnData 接收一帧数据并更新 latest/history，按 publishHz 节流推送给订阅者。
//
// 所有权契约（重要）：
//   - 调用方必须保证传入 payload 的 Channels / ChannelIndices 切片在 OnData 返回后
//     不被修改（包括底层底层数组）。若生产者复用切片（如 sync.Pool），调用方必须在
//     调用 OnData 前完成防御性拷贝。
//   - hub 一旦接收 payload，会将其存储到 latest / history ring 中，订阅者 channel
//     也会收到同一份切片的引用；任何后续修改都会破坏数据一致性。
//
// 实践建议：在 dataSink 装配层（bootstrap.go 的 dataSink 闭包）统一做防御性拷贝，
// 让 hub 和 recorder 共享同一份独立副本，避免在 hub 内重复拷贝（recorder 异步路径
// 会把 payload 引用存入 channel，hub 内拷贝无法保护该路径）。
func (h *AcquisitionHub) OnData(payload device.DataPayload) {
	payload.EnsureNonNilSlices()

	shard := &h.shards[shardIndexForDevice(payload.DeviceID)]
	intervalNs := h.publishIntervalNs.Load()
	now := time.Now()

	var subscribers []chan device.DataPayload
	var shouldPublish bool

	shard.mu.Lock()
	// 始终更新最新数据和历史记录
	shard.latest[payload.DeviceID] = payload
	ring, ok := shard.history[payload.DeviceID]
	if !ok {
		ring = newHistoryRing(h.historyCapacity)
		shard.history[payload.DeviceID] = ring
	}
	ring.push(payload)

	// 按 publishHz 节流推送到订阅者
	if intervalNs > 0 {
		interval := time.Duration(intervalNs)
		lastPublish := shard.lastPublishAt[payload.DeviceID]
		shouldPublish = now.Sub(lastPublish) >= interval
		if shouldPublish {
			shard.lastPublishAt[payload.DeviceID] = now
			// 复制订阅者列表，避免在锁外发送时迭代 map
			if subs, ok := shard.subscribers[payload.DeviceID]; ok && len(subs) > 0 {
				subscribers = make([]chan device.DataPayload, 0, len(subs))
				for ch := range subs {
					subscribers = append(subscribers, ch)
				}
			}
		}
	}
	shard.mu.Unlock()

	// 在锁外发送数据，避免阻塞其他设备的数据处理；
	// 缓冲区满时仅累计丢弃计数，按 dropLogInterval 节流输出聚合告警。
	if shouldPublish && len(subscribers) > 0 {
		var dropped int
		for _, ch := range subscribers {
			select {
			case ch <- payload:
			default:
				dropped++
			}
		}
		if dropped > 0 {
			h.recordDrops(payload.DeviceID, dropped)
		}
	}
}

// recordDrops 累计丢弃计数；距离上次告警超过 dropLogInterval 时输出一条聚合日志。
func (h *AcquisitionHub) recordDrops(deviceID string, dropped int) {
	now := time.Now()

	h.dropMu.Lock()
	h.dropCount[deviceID] += dropped
	total := h.dropCount[deviceID]
	last := h.dropLastLogAt[deviceID]
	shouldLog := last.IsZero() || now.Sub(last) >= dropLogInterval
	if shouldLog {
		h.dropLastLogAt[deviceID] = now
		h.dropCount[deviceID] = 0
	}
	h.dropMu.Unlock()

	if shouldLog {
		slog.Warn("AcquisitionHub: 订阅者缓冲区已满，数据被丢弃",
			"device", deviceID, "dropped", total, "interval", dropLogInterval)
	}
}

func (h *AcquisitionHub) GetLatestData(deviceID string) (device.DataPayload, bool) {
	shard := &h.shards[shardIndexForDevice(deviceID)]
	shard.mu.RLock()
	defer shard.mu.RUnlock()
	payload, ok := shard.latest[deviceID]
	return payload, ok
}

func (h *AcquisitionHub) GetLatestTimestamp(deviceID string) (int64, bool) {
	shard := &h.shards[shardIndexForDevice(deviceID)]
	shard.mu.RLock()
	defer shard.mu.RUnlock()
	payload, ok := shard.latest[deviceID]
	if !ok {
		return 0, false
	}
	return payload.Timestamp, true
}

func (h *AcquisitionHub) GetRecentData(deviceID string, limit int) []device.DataPayload {
	shard := &h.shards[shardIndexForDevice(deviceID)]
	shard.mu.RLock()
	defer shard.mu.RUnlock()
	ring, ok := shard.history[deviceID]
	if !ok {
		return nil
	}
	return ring.recent(limit)
}

func (h *AcquisitionHub) Subscribe(deviceID string, buffer int) (<-chan device.DataPayload, func()) {
	if buffer < 1 {
		buffer = 1
	}
	ch := make(chan device.DataPayload, buffer)
	shard := &h.shards[shardIndexForDevice(deviceID)]
	shard.mu.Lock()
	if shard.subscribers[deviceID] == nil {
		shard.subscribers[deviceID] = make(map[chan device.DataPayload]struct{})
	}
	shard.subscribers[deviceID][ch] = struct{}{}
	shard.mu.Unlock()

	unsubscribe := func() {
		shard.mu.Lock()
		if subs := shard.subscribers[deviceID]; subs != nil {
			delete(subs, ch)
			if len(subs) == 0 {
				delete(shard.subscribers, deviceID)
			}
		}
		shard.mu.Unlock()
	}
	return ch, unsubscribe
}

func (h *AcquisitionHub) SetPublishRate(hz float64) error {
	if hz < minPublishHz || hz > maxPublishHz {
		return fmt.Errorf("publish rate must be between %.0f and %.0f Hz", minPublishHz, maxPublishHz)
	}
	h.publishIntervalNs.Store(time.Duration(float64(time.Second) / hz).Nanoseconds())
	return nil
}

func (h *AcquisitionHub) PublishRate() float64 {
	return h.publishHzLocked()
}
