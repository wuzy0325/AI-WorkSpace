package logging

import (
	"context"
	"encoding/json"
	"shared.local/device-sdk/go/pkg/slog"
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
// stderr 和文件日志不受 catFilter 影响（它们在 fanoutHandler 的其他 sink 中独立处理）。
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

func (h *RingHandler) Enabled(_ context.Context, level slog.Level) bool {
	if h.level == nil {
		return level >= slog.LevelInfo
	}
	return level >= h.level.Level()
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
