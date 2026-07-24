package logging

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// RingEntry 是 ring buffer 中的一条日志条目，结构与前端 LogEntry 对应。
type RingEntry struct {
	ID        uint64 `json:"id"`
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Category  string `json:"category,omitempty"`
	DeviceID  string `json:"deviceId,omitempty"`
	Source    string `json:"source"`
	Message   string `json:"message"`
	Details   string `json:"details,omitempty"`
}

// RingBuffer 是一个线程安全的固定容量环形缓冲区，存储最近的日志条目。
// 同时支持 SSE 订阅：每个订阅者通过 channel 实时接收新日志。
type RingBuffer struct {
	mu     sync.RWMutex
	buf    []RingEntry
	cap    int
	head   int    // 下一个写入位置
	count  int    // 有效元素数量
	nextID uint64 // 单调递增的条目 ID（从 1 开始）

	subsMu sync.Mutex
	subs   map[uint64]chan RingEntry
	subSeq atomic.Uint64 // 订阅者单调 ID，避免 len(subs) 复用导致的覆盖
}

// NewRingBuffer 创建一个指定容量的 RingBuffer。
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = 2000
	}
	return &RingBuffer{
		buf:  make([]RingEntry, capacity),
		cap:  capacity,
		subs: make(map[uint64]chan RingEntry),
	}
}

// push 向环形缓冲区追加一条条目。同时推送给所有 SSE 订阅者。
// 调用方传入的 entry.ID 会被覆盖为单调递增的全局 ID。
// 仅限包内使用（RingHandler），外部不应直接调用。
func (rb *RingBuffer) push(entry RingEntry) {
	rb.mu.Lock()
	rb.nextID++
	entry.ID = rb.nextID
	rb.buf[rb.head] = entry
	rb.head = (rb.head + 1) % rb.cap
	if rb.count < rb.cap {
		rb.count++
	}
	rb.mu.Unlock()

	// 非阻塞推送订阅者
	rb.subsMu.Lock()
	for _, ch := range rb.subs {
		select {
		case ch <- entry:
		default:
			// 订阅者消费不及时，丢弃避免阻塞
		}
	}
	rb.subsMu.Unlock()
}

// Recent 返回最近的 n 条日志，按时间从旧到新排序。
func (rb *RingBuffer) Recent(n int) []RingEntry {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	if n <= 0 || rb.count == 0 {
		return nil
	}
	if n > rb.count {
		n = rb.count
	}
	result := make([]RingEntry, n)
	start := (rb.head - rb.count + rb.cap) % rb.cap
	for i := 0; i < n; i++ {
		result[i] = rb.buf[(start+i)%rb.cap]
	}
	return result
}

// Subscribe 注册一个 SSE 订阅者，返回的 channel 在推送新日志或 context 取消时收到数据。
// 调用方必须在结束后取消 context 以释放资源。
// channel 缓冲 256 条，避免高日志量时短时消费不及导致丢帧。
func (rb *RingBuffer) Subscribe(ctx context.Context) <-chan RingEntry {
	ch := make(chan RingEntry, 256)
	// 使用单调递增 ID，避免 len(subs) 被复用导致并发订阅互相覆盖。
	subID := rb.subSeq.Add(1)

	rb.subsMu.Lock()
	rb.subs[subID] = ch
	rb.subsMu.Unlock()

	go func() {
		<-ctx.Done()
		rb.subsMu.Lock()
		delete(rb.subs, subID)
		rb.subsMu.Unlock()
		close(ch)
	}()
	return ch
}

// RingHandler 是一个 slog.Handler，将日志消息写入 RingBuffer。
//
// 同时实现了 slog 的 WithAttrs / WithGroup 语义：通过 logging.WithComponent
// 等链式调用追加的属性会被保留下来，Handle 时与 Record.Attrs 一起合并写入 RingEntry。
//
// 写入前会检查 catFilter：若该 category 被关闭，则跳过不写入 ring buffer。
// stderr 和文件日志不受 catFilter 影响（它们在 fanoutHandler 的其他 sink 中独立处理，
// 但被 CategorySkipHandler 包装以跳过高频 category 避免刷屏）。
//
// 级别过滤策略：
//   - hardware-send / hardware-recv 在硬件适配器中用 Info 级别记录（不是 Debug），
//     因此 Enabled 按全局 Info 阈值即可透传，无需特殊例外；
//   - 其他无关 Debug（如数据帧解析）被 Enabled 按全局阈值拦截，零 Record 构造开销；
//   - stderr / file sink 通过 CategorySkipHandler 跳过 hardware-send/recv，
//     避免高频命令帧刷屏文件与终端，同时不影响 ring buffer 写入。
type RingHandler struct {
	ring      *RingBuffer
	level     *slog.LevelVar
	catFilter *categoryFilter // 分类过滤器，nil 表示全放行
	attrs     []slog.Attr     // 来自 WithAttrs 链路上的累积属性
	groups    []string        // 来自 WithGroup 的分组栈（仅作为前缀拼接到 detail key）
}

// NewRingHandler 创建新的 RingHandler。
func NewRingHandler(ring *RingBuffer, level *slog.LevelVar, catFilter *categoryFilter) *RingHandler {
	return &RingHandler{ring: ring, level: level, catFilter: catFilter}
}

// Enabled 按全局级别阈值过滤。
//
// hardware-send / hardware-recv 的命令收发日志在硬件适配器中用 Info 级别记录，
// 因此默认 Info 阈值下即可透传到 ring buffer。其他无关 Debug 被此处拦截，
// 避免 Record 构造开销（关键性能路径：1000Hz 采集下数据帧 Debug 每秒可达 1000+ 条）。
func (h *RingHandler) Enabled(_ context.Context, level slog.Level) bool {
	if h.level == nil {
		return level >= slog.LevelInfo
	}
	return level >= h.level.Level()
}

// CategorySkipHandler 包装另一个 slog.Handler，跳过指定 category 的日志写入。
//
// 用途：包装 stderr / file sink，跳过 hardware-send / hardware-recv 等高频 category，
// 避免命令收发帧刷屏文件与终端。ring buffer 不受影响（由 RingHandler 独立控制）。
//
// category 识别范围：仅 record inline attrs 中的 "category" 字段。
// 通过 slog.With("category", ...) 在 WithAttrs 链路上设置的 category 不识别——
// 因为 slog.Record.Attrs 只暴露 inline attrs，无法回溯 WithAttrs 累积值。
// 当前所有 hardware 适配器均用 inline 传入（slog.Info("...", "category", "hardware-send")），
// 因此该限制不影响实际使用；若未来改为 WithAttrs 链式调用，需扩展此 Handler。
type CategorySkipHandler struct {
	inner slog.Handler
	skip  map[string]struct{} // 需要跳过的 category 集合，初始化后只读
}

// NewCategorySkipHandler 创建包装器。skip 为空时等价于直接透传。
func NewCategorySkipHandler(inner slog.Handler, skip []string) *CategorySkipHandler {
	m := make(map[string]struct{}, len(skip))
	for _, c := range skip {
		m[c] = struct{}{}
	}
	return &CategorySkipHandler{inner: inner, skip: m}
}

func (h *CategorySkipHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *CategorySkipHandler) Handle(ctx context.Context, r slog.Record) error {
	// 遍历 record attrs 提取 category；命中 skip 列表则跳过 inner sink。
	// slog.Attrs 回调返回 false 可停止遍历，无需 panic sentinel。
	if len(h.skip) > 0 {
		var shouldSkip bool
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "category" {
				if _, ok := h.skip[a.Value.String()]; ok {
					shouldSkip = true
					return false // 命中即停止遍历
				}
			}
			return true
		})
		if shouldSkip {
			return nil
		}
	}
	return h.inner.Handle(ctx, r)
}

func (h *CategorySkipHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &CategorySkipHandler{inner: h.inner.WithAttrs(attrs), skip: h.skip}
}

func (h *CategorySkipHandler) WithGroup(name string) slog.Handler {
	return &CategorySkipHandler{inner: h.inner.WithGroup(name), skip: h.skip}
}

func (h *RingHandler) Handle(_ context.Context, r slog.Record) error {
	entry := RingEntry{
		Timestamp: r.Time.Format(time.RFC3339Nano),
		Source:    "backend",
	}

	switch {
	case r.Level >= slog.LevelError:
		entry.Level = "error"
	case r.Level >= slog.LevelWarn:
		entry.Level = "warn"
	case r.Level >= slog.LevelInfo:
		entry.Level = "info"
	default:
		entry.Level = "debug"
	}

	entry.Message = r.Message

	// 用 map 聚合所有 key=value，最后一次性输出，避免丢失非常用字段。
	details := make(map[string]string, 4)
	groupPrefix := ""
	if len(h.groups) > 0 {
		groupPrefix = strings.Join(h.groups, ".") + "."
	}

	// 处理来自 WithAttrs 链路上的属性
	for _, a := range h.attrs {
		applyAttrToEntry(a, groupPrefix, &entry, details)
	}
	// 处理本次调用 inline 传入的属性
	r.Attrs(func(a slog.Attr) bool {
		applyAttrToEntry(a, groupPrefix, &entry, details)
		return true
	})

	if entry.Details == "" && len(details) > 0 {
		// 优先用扁平 key=value 拼接，便于前端表格展示
		parts := make([]string, 0, len(details))
		for k, v := range details {
			parts = append(parts, k+"="+v)
		}
		entry.Details = strings.Join(parts, " ")
	} else if len(details) > 0 {
		// 已经有 Details（component-style）时，附加 JSON 形式的剩余字段
		if buf, err := json.Marshal(details); err == nil {
			entry.Details = entry.Details + " " + string(buf)
		}
	}
	if entry.Category == "" {
		entry.Category = inferCategory(entry.Source, entry.Message, entry.Details)
	}

	// 检查 category 过滤器：若该分类被关闭，跳过 ring buffer 写入
	// （stderr 和文件日志已在 fanoutHandler 的其他 sink 中独立写入，不受此影响）
	if !h.catFilter.isEnabled(entry.Category) {
		return nil
	}

	h.ring.push(entry)
	return nil
}

func inferCategory(source string, message string, details string) string {
	haystack := strings.ToLower(source + " " + message + " " + details)
	// 通信方向优先判定：发送（send）与接收（response/recv）需分开匹配，
	// 避免把"command"单独关键字误归到 hardware-recv（与前端 CATEGORY_LABELS 口径相反）。
	// 仅在 entry 未显式指定 category 时作为兜底，所以保守匹配即可。
	switch {
	case strings.Contains(haystack, "hardware-send"), strings.Contains(haystack, "command send"), strings.Contains(haystack, "bytes written"):
		return "hardware-send"
	case strings.Contains(haystack, "hardware-recv"), strings.Contains(haystack, "command response"), strings.Contains(haystack, "tcp connected"), strings.Contains(haystack, "tcp disconnected"):
		return "hardware-recv"
	case strings.Contains(haystack, "acquisition"), strings.Contains(haystack, "采集"):
		return "acquisition"
	case strings.Contains(haystack, "calibration"), strings.Contains(haystack, "traversal"), strings.Contains(haystack, "storage"), strings.Contains(haystack, "motion"):
		return "business"
	default:
		return "system"
	}
}

// applyAttrToEntry 把一个 slog.Attr 写入 entry / details，
// category / component / device_id / device 走结构化字段，其他归到 details。
func applyAttrToEntry(a slog.Attr, groupPrefix string, entry *RingEntry, details map[string]string) {
	if a.Equal(slog.Attr{}) {
		return
	}
	key := groupPrefix + a.Key
	val := a.Value.String()
	switch a.Key {
	case "category":
		entry.Category = val
	case "component":
		entry.Source = val
	case "device_id":
		// 规范键：直接采用，覆盖任何先前由 "device" 别名写入的值。
		entry.DeviceID = val
	case "device":
		// 兼容别名：仅在规范键 device_id 未设置时填充，避免两者并存时静默覆盖。
		if entry.DeviceID == "" {
			entry.DeviceID = val
		}
	case "session_id", "task_id":
		if entry.Details == "" {
			entry.Details = a.Key + "=" + val
		} else {
			entry.Details += " " + a.Key + "=" + val
		}
	default:
		details[key] = val
	}
}

// WithAttrs 返回一个保留了额外 attrs 的 RingHandler 副本，符合 slog.Handler 语义。
func (h *RingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	merged := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	merged = append(merged, h.attrs...)
	merged = append(merged, attrs...)
	return &RingHandler{
		ring:      h.ring,
		level:     h.level,
		catFilter: h.catFilter,
		attrs:     merged,
		groups:    h.groups,
	}
}

// WithGroup 返回一个追加分组名的 RingHandler 副本。
func (h *RingHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	groups := make([]string, 0, len(h.groups)+1)
	groups = append(groups, h.groups...)
	groups = append(groups, name)
	return &RingHandler{
		ring:      h.ring,
		level:     h.level,
		catFilter: h.catFilter,
		attrs:     h.attrs,
		groups:    groups,
	}
}
