// Package realtime 提供实时插值计算的缓存与节流支持。
//
// 对应 Cursor DAQ 中的 InterpolationCache.ts + OptimizedRealtimeInterpolator.ts +
// RealtimeUpdateThrottler.ts 三个文件的合并 Go 实现。
//
// 用途：高频实时显示场景下：
//   - InterpolationCache：按压力量化键 + 容差匹配，避免相近输入重复算
//   - Throttler：限制最高输出频率，丢弃中间帧、保留首尾，避免 UI 刷爆
package realtime

import (
	"math"
	"sort"
	"strconv"
	"sync"
	"time"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
)

// ---------- InterpolationCache ----------

// cacheEntry 单条缓存项
type cacheEntry struct {
	key        string
	input      coreinterp.InterpolationInput
	result     coreinterp.InterpolationResult
	hitCount   int64
	lastAccess time.Time
}

// InterpolationCache 插值结果 LRU 缓存
// 量化键：将输入压力按 tolerance 量化后拼接为 string；
// 当精确未命中时，遍历缓存做容差匹配（每维差 ≤ 2*tolerance）。
type InterpolationCache struct {
	mu        sync.Mutex
	entries   map[string]*cacheEntry
	maxSize   int
	tolerance float64
	hits      int64
	misses    int64
}

// NewInterpolationCache 创建插值缓存
//   - maxSize ≤0 时取默认 256
//   - tolerance ≤0 时取默认 1.0 Pa（与 Cursor DAQ 默认一致）
func NewInterpolationCache(maxSize int, tolerance float64) *InterpolationCache {
	if maxSize <= 0 {
		maxSize = 256
	}
	if tolerance <= 0 {
		tolerance = 1.0
	}
	return &InterpolationCache{
		entries:   make(map[string]*cacheEntry, maxSize),
		maxSize:   maxSize,
		tolerance: tolerance,
	}
}

// quantizeKey 把压力量化为字符串键（量化后 round 到 tolerance 倍数）
func (c *InterpolationCache) quantizeKey(in coreinterp.InterpolationInput) string {
	q := func(v float64) float64 {
		return math.Round(v/c.tolerance) * c.tolerance
	}
	// 用 strconv 保证精度可控
	return formatFloatKey(q(in.P1)) + "|" +
		formatFloatKey(q(in.P2)) + "|" +
		formatFloatKey(q(in.P3)) + "|" +
		formatFloatKey(q(in.P4)) + "|" +
		formatFloatKey(q(in.P5)) + "|" +
		formatFloatKey(q(in.PAtm)) + "|" +
		formatFloatKey(q(in.TAtm))
}

// Find 命中返回 (result, true)，否则 (zero, false)
func (c *InterpolationCache) Find(in coreinterp.InterpolationInput) (coreinterp.InterpolationResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := c.quantizeKey(in)
	if entry, ok := c.entries[key]; ok {
		entry.hitCount++
		entry.lastAccess = time.Now()
		c.hits++
		return entry.result, true
	}
	// 容差扫描：精确未命中时遍历，每维差 ≤ 2*tolerance
	tol2 := 2 * c.tolerance
	for _, entry := range c.entries {
		if absDiff(entry.input.P1, in.P1) <= tol2 &&
			absDiff(entry.input.P2, in.P2) <= tol2 &&
			absDiff(entry.input.P3, in.P3) <= tol2 &&
			absDiff(entry.input.P4, in.P4) <= tol2 &&
			absDiff(entry.input.P5, in.P5) <= tol2 &&
			absDiff(entry.input.PAtm, in.PAtm) <= tol2 &&
			absDiff(entry.input.TAtm, in.TAtm) <= tol2 {
			entry.hitCount++
			entry.lastAccess = time.Now()
			c.hits++
			return entry.result, true
		}
	}
	c.misses++
	return coreinterp.InterpolationResult{}, false
}

// Store 写入结果，超容量时触发 LRU 淘汰
func (c *InterpolationCache) Store(in coreinterp.InterpolationInput, res coreinterp.InterpolationResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := c.quantizeKey(in)
	if len(c.entries) >= c.maxSize {
		c.evictLRULocked()
	}
	c.entries[key] = &cacheEntry{
		key:        key,
		input:      in,
		result:     res,
		hitCount:   0,
		lastAccess: time.Now(),
	}
}

// Clear 清空缓存
func (c *InterpolationCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*cacheEntry, c.maxSize)
	c.hits = 0
	c.misses = 0
}

// Stats 获取缓存统计
func (c *InterpolationCache) Stats() (size int, hits, misses int64, hitRate float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	total := c.hits + c.misses
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}
	return len(c.entries), c.hits, c.misses, hitRate
}

// evictLRULocked 淘汰最久未访问且命中次数最少的一条（需持锁）
func (c *InterpolationCache) evictLRULocked() {
	if len(c.entries) == 0 {
		return
	}
	// 按 lastAccess 升序 + hitCount 升序，淘汰排序后的第一条
	keys := make([]string, 0, len(c.entries))
	for k := range c.entries {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ei, ej := c.entries[keys[i]], c.entries[keys[j]]
		if ei.hitCount != ej.hitCount {
			return ei.hitCount < ej.hitCount
		}
		return ei.lastAccess.Before(ej.lastAccess)
	})
	delete(c.entries, keys[0])
}

// ---------- Throttler ----------

// Throttler 按最小间隔节流的通用通道
//   - 每次 Push 都更新最新值
//   - 距上次 Flush 已达 minInterval → 立即 Flush
//   - 否则 skipped++；当 skipped >= maxSkipped 时强制 Flush
//   - 否则在剩余时间后触发一次延迟 Flush
type Throttler[T any] struct {
	mu           sync.Mutex
	minInterval  time.Duration
	maxSkipped   int
	handler      func(T, int)
	lastFlush    time.Time
	pendingValue T
	hasPending   bool
	skippedCount int
	pendingTimer *time.Timer
}

// NewThrottler 创建节流器
//   - frequencyHz: 目标最高频率（≤0 取 10Hz）
//   - maxSkipped:  连续跳过的最大次数（≤0 取 10）
//   - handler:     真正分发回调；第二参数为本次 flush 已跳过的次数
func NewThrottler[T any](frequencyHz float64, maxSkipped int, handler func(T, int)) *Throttler[T] {
	if frequencyHz <= 0 {
		frequencyHz = 10
	}
	if maxSkipped <= 0 {
		maxSkipped = 10
	}
	interval := time.Duration(float64(time.Second) / frequencyHz)
	return &Throttler[T]{
		minInterval: interval,
		maxSkipped:  maxSkipped,
		handler:     handler,
	}
}

// Push 提交一个新值
func (t *Throttler[T]) Push(value T) {
	t.mu.Lock()
	t.pendingValue = value
	t.hasPending = true
	elapsed := time.Since(t.lastFlush)
	if elapsed >= t.minInterval {
		t.flushLocked()
		t.mu.Unlock()
		return
	}
	t.skippedCount++
	if t.skippedCount >= t.maxSkipped {
		t.flushLocked()
		t.mu.Unlock()
		return
	}
	// 安排延迟刷新（替换旧 timer，避免重复触发）
	if t.pendingTimer != nil {
		t.pendingTimer.Stop()
	}
	delay := t.minInterval - elapsed
	t.pendingTimer = time.AfterFunc(delay, func() {
		t.mu.Lock()
		if t.hasPending {
			t.flushLocked()
		}
		t.mu.Unlock()
	})
	t.mu.Unlock()
}

// Flush 强制立即分发当前 pending（如果有）
func (t *Throttler[T]) Flush() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.hasPending {
		t.flushLocked()
	}
}

// flushLocked 必须在持锁下调用
func (t *Throttler[T]) flushLocked() {
	skipped := t.skippedCount
	value := t.pendingValue
	t.skippedCount = 0
	t.hasPending = false
	t.lastFlush = time.Now()
	if t.pendingTimer != nil {
		t.pendingTimer.Stop()
		t.pendingTimer = nil
	}
	if t.handler != nil {
		// 在锁外回调，避免 handler 重新 Push 时死锁
		go t.handler(value, skipped)
	}
}

// ---------- helpers ----------

func absDiff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}

// formatFloatKey 量化键专用：固定 6 位小数，跨平台一致
func formatFloatKey(v float64) string {
	return strconv.FormatFloat(v, 'f', 6, 64)
}
