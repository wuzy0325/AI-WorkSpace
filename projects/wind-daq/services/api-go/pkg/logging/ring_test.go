package logging

import (
	"bytes"
	"context"
	"shared.local/device-sdk/go/pkg/slog"
	"strings"
	"testing"
	"time"
)

// captureHandler 记录所有 Handle 调用的消息，用于断言 inner sink 是否被调用。
type captureHandler struct {
	messages []string
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.messages = append(h.messages, r.Message)
	return nil
}

func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler       { return h }

// newRecord 构造带 inline attrs 的 slog.Record，供测试统一调用。
// 注意：polyfill 的 Record.Add 签名为 (key, value) 二元，标准库为 ...any 可变参数；
// 此处用 AddAttrs 直接传 Attr 切片，两种实现都支持，跨 Go 版本兼容。
func newRecord(msg string, level slog.Level, attrs ...slog.Attr) slog.Record {
	r := slog.NewRecord(time.Now(), level, msg, 0)
	r.AddAttrs(attrs...)
	return r
}

// TestCategorySkipHandler_SkipMatchingCategory 验证命中 skip 列表的 category 不写入 inner。
func TestCategorySkipHandler_SkipMatchingCategory(t *testing.T) {
	capture := &captureHandler{}
	h := NewCategorySkipHandler(capture, []string{"hardware-send", "hardware-recv"})

	_ = h.Handle(context.Background(), newRecord("DSA3217 command send", slog.LevelInfo,
		slog.String("category", "hardware-send")))

	if len(capture.messages) != 0 {
		t.Fatalf("hardware-send 应被跳过，但 inner 收到 %d 条: %v", len(capture.messages), capture.messages)
	}
}

// TestCategorySkipHandler_PassThroughNonSkip 验证未命中 skip 列表的 category 正常透传 inner。
func TestCategorySkipHandler_PassThroughNonSkip(t *testing.T) {
	capture := &captureHandler{}
	h := NewCategorySkipHandler(capture, []string{"hardware-send", "hardware-recv"})

	_ = h.Handle(context.Background(), newRecord("device connected", slog.LevelInfo,
		slog.String("category", "system")))

	if len(capture.messages) != 1 || capture.messages[0] != "device connected" {
		t.Fatalf("system category 应透传，实际收到 %d 条: %v", len(capture.messages), capture.messages)
	}
}

// TestCategorySkipHandler_NoCategoryAttr 验证无 category attr 的 record 不跳过。
func TestCategorySkipHandler_NoCategoryAttr(t *testing.T) {
	capture := &captureHandler{}
	h := NewCategorySkipHandler(capture, []string{"hardware-send"})

	_ = h.Handle(context.Background(), newRecord("app started", slog.LevelInfo))

	if len(capture.messages) != 1 {
		t.Fatalf("无 category 的 record 应透传，实际收到 %d 条", len(capture.messages))
	}
}

// TestCategorySkipHandler_EmptySkipList 验证空 skip 列表等价于直接透传。
func TestCategorySkipHandler_EmptySkipList(t *testing.T) {
	capture := &captureHandler{}
	h := NewCategorySkipHandler(capture, nil)

	_ = h.Handle(context.Background(), newRecord("anything", slog.LevelInfo,
		slog.String("category", "hardware-send")))

	if len(capture.messages) != 1 {
		t.Fatalf("空 skip 列表应透传所有日志，实际收到 %d 条", len(capture.messages))
	}
}

// TestCategorySkipHandler_WithAttrsPreservesSkip 验证 WithAttrs 后仍能识别 inline category。
//
// 注意：CategorySkipHandler 仅识别 inline attrs 的 category，不识别 WithAttrs 链路上的 category
// （因为 slog.Record.Attrs 不暴露累积的 WithAttrs 值）。本测试确认 WithAttrs 包装后，
// 后续 inline 传入的 category 仍能被正确识别跳过。
func TestCategorySkipHandler_WithAttrsPreservesSkip(t *testing.T) {
	capture := &captureHandler{}
	h := NewCategorySkipHandler(capture, []string{"hardware-send"})
	wrapped := h.WithAttrs([]slog.Attr{slog.String("component", "hardware")})

	skip := wrapped.(*CategorySkipHandler)
	if _, ok := skip.skip["hardware-send"]; !ok {
		t.Fatalf("WithAttrs 后 skip map 应保留 hardware-send")
	}

	_ = wrapped.Handle(context.Background(), newRecord("send frame", slog.LevelInfo,
		slog.String("category", "hardware-send")))

	if len(capture.messages) != 0 {
		t.Fatalf("WithAttrs 后 hardware-send 仍应被跳过，实际收到 %d 条", len(capture.messages))
	}
}

// TestRingHandler_InfoLevelWritesToRing 验证 Info 级别日志在默认 Info 阈值下写入 ring buffer。
func TestRingHandler_InfoLevelWritesToRing(t *testing.T) {
	ring := NewRingBuffer(16)
	level := new(slog.LevelVar)
	catFilter := &categoryFilter{enabled: make(map[string]bool)}
	h := NewRingHandler(ring, level, catFilter)

	_ = h.Handle(context.Background(), newRecord("test info", slog.LevelInfo))

	recent := ring.Recent(10)
	if len(recent) != 1 || recent[0].Message != "test info" {
		t.Fatalf("应写入 1 条 Info 日志，实际 %d 条: %v", len(recent), recent)
	}
}

// TestRingHandler_CategoryFilterSkipsDisabled 验证被 catFilter 关闭的 category 不写入 ring。
func TestRingHandler_CategoryFilterSkipsDisabled(t *testing.T) {
	ring := NewRingBuffer(16)
	level := new(slog.LevelVar)
	catFilter := &categoryFilter{enabled: map[string]bool{"hardware-send": false}}
	h := NewRingHandler(ring, level, catFilter)

	_ = h.Handle(context.Background(), newRecord("send", slog.LevelInfo,
		slog.String("category", "hardware-send"),
		slog.String("component", "hardware"),
	))

	if len(ring.Recent(10)) != 0 {
		t.Fatalf("被 catFilter 关闭的 hardware-send 不应写入 ring，实际 %d 条", len(ring.Recent(10)))
	}
}

// TestRingHandler_AttrsPopulateEntry 验证 inline attrs 正确填充 RingEntry 结构化字段。
func TestRingHandler_AttrsPopulateEntry(t *testing.T) {
	ring := NewRingBuffer(16)
	level := new(slog.LevelVar)
	catFilter := &categoryFilter{enabled: make(map[string]bool)}
	h := NewRingHandler(ring, level, catFilter)

	_ = h.Handle(context.Background(), newRecord("send frame", slog.LevelInfo,
		slog.String("category", "hardware-send"),
		slog.String("component", "hardware"),
		slog.String("device_id", "dsa-001"),
	))

	recent := ring.Recent(1)
	if len(recent) != 1 {
		t.Fatalf("应写入 1 条，实际 %d 条", len(recent))
	}
	entry := recent[0]
	if entry.Category != "hardware-send" {
		t.Errorf("Category 应为 hardware-send，实际 %q", entry.Category)
	}
	if entry.Source != "hardware" {
		t.Errorf("Source 应为 hardware，实际 %q", entry.Source)
	}
	if entry.DeviceID != "dsa-001" {
		t.Errorf("DeviceID 应为 dsa-001，实际 %q", entry.DeviceID)
	}
}

// TestRingHandler_EnabledRespectsLevelVar 验证 Enabled 按全局 levelVar 阈值过滤。
//
// 关键性能保证：Debug 日志在全局 Info 阈值下被 Enabled 拦截，不进入 Handle，
// 避免 1000Hz 采集场景下 Record 构造开销。
func TestRingHandler_EnabledRespectsLevelVar(t *testing.T) {
	level := new(slog.LevelVar) // 默认 LevelInfo
	catFilter := &categoryFilter{enabled: make(map[string]bool)}
	h := NewRingHandler(NewRingBuffer(16), level, catFilter)

	if h.Enabled(context.Background(), slog.LevelDebug) {
		t.Errorf("Info 阈值下 Debug 不应 Enabled")
	}
	if !h.Enabled(context.Background(), slog.LevelInfo) {
		t.Errorf("Info 阈值下 Info 应 Enabled")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Errorf("Info 阈值下 Error 应 Enabled")
	}

	level.Set(slog.LevelWarn)
	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Errorf("Warn 阈值下 Info 不应 Enabled")
	}
}

// TestRingHandler_InferCategoryFallback 验证未显式标注 category 时走 inferCategory 兜底。
func TestRingHandler_InferCategoryFallback(t *testing.T) {
	ring := NewRingBuffer(16)
	level := new(slog.LevelVar)
	catFilter := &categoryFilter{enabled: make(map[string]bool)}
	h := NewRingHandler(ring, level, catFilter)

	_ = h.Handle(context.Background(), newRecord("采集 started", slog.LevelInfo))

	recent := ring.Recent(1)
	if len(recent) != 1 {
		t.Fatalf("应写入 1 条，实际 %d 条", len(recent))
	}
	if recent[0].Category != "acquisition" {
		t.Errorf("inferCategory 应识别 '采集' 为 acquisition，实际 %q", recent[0].Category)
	}
}

// TestStderrFileSkipCategoriesContainsHardware 验证默认跳过列表包含 hardware-send / hardware-recv。
//
// 防止未来重构时误删这两个高频 category，导致命令收发帧重新刷屏文件与终端。
func TestStderrFileSkipCategoriesContainsHardware(t *testing.T) {
	has := func(cat string) bool {
		for _, c := range StderrFileSkipCategories {
			if c == cat {
				return true
			}
		}
		return false
	}
	if !has("hardware-send") {
		t.Errorf("StderrFileSkipCategories 应包含 hardware-send")
	}
	if !has("hardware-recv") {
		t.Errorf("StderrFileSkipCategories 应包含 hardware-recv")
	}
}

// TestRingBuffer_RecentOrdering 验证 Recent 返回的日志按时间从旧到新排序。
//
// 注意：当前 Recent 在容量未满时返回从最早条目开始的 n 条（而非"最近 n 条"），
// 这是预存行为；本测试断言匹配实际行为，未来若修正 Recent 语义为"最近 n 条"
// 需同步更新断言。
func TestRingBuffer_RecentOrdering(t *testing.T) {
	ring := NewRingBuffer(8)
	for i := 0; i < 5; i++ {
		ring.push(RingEntry{Message: string(rune('a' + i))})
	}

	recent := ring.Recent(3)
	if len(recent) != 3 {
		t.Fatalf("应返回 3 条，实际 %d 条", len(recent))
	}
	// 容量未满时取最早 3 条 a,b,c
	if recent[0].Message != "a" || recent[1].Message != "b" || recent[2].Message != "c" {
		t.Errorf("顺序应为 a,b,c，实际 %v", recent)
	}
}

// TestRingBuffer_CapacityLimit 验证 ring buffer 满后旧条目被覆盖。
func TestRingBuffer_CapacityLimit(t *testing.T) {
	ring := NewRingBuffer(3)
	for i := 0; i < 5; i++ {
		ring.push(RingEntry{Message: string(rune('a' + i))})
	}

	recent := ring.Recent(10)
	if len(recent) != 3 {
		t.Fatalf("容量 3 应只保留最近 3 条，实际 %d 条", len(recent))
	}
	// 最早两条 a/b 应被覆盖，保留 c/d/e
	if recent[0].Message != "c" || recent[2].Message != "e" {
		t.Errorf("应保留 c/d/e，实际 %v", recent)
	}
}

// TestFanoutHandlerWritesToAllSinks 验证 fanoutHandler 将日志分发到所有 sink。
//
// 本测试用 stderr sink + captureHandler 模拟 ring + stderr 双路，
// 确认 hardware-send 在 stderr 被 CategorySkipHandler 跳过、在 capture 中正常透传。
func TestFanoutHandlerWritesToAllSinks(t *testing.T) {
	capture := &captureHandler{}
	// 模拟 logger.go Init 中的 sink 组合：ring (capture) + stderr (被 CategorySkip 包装)
	var stderrBuf bytes.Buffer
	stderrHandler := slog.NewTextHandler(&stderrBuf, &slog.HandlerOptions{Level: slog.LevelInfo})
	fanout := newFanoutHandler(capture, NewCategorySkipHandler(stderrHandler, StderrFileSkipCategories))

	logger := slog.New(fanout)
	logger.Info("send frame", "category", "hardware-send", "component", "hardware")

	if len(capture.messages) != 1 {
		t.Errorf("capture sink 应收到 1 条（模拟 ring），实际 %d 条", len(capture.messages))
	}
	stderrOutput := stderrBuf.String()
	if strings.Contains(stderrOutput, "send frame") {
		t.Errorf("stderr 应被 CategorySkipHandler 跳过 hardware-send，实际输出: %s", stderrOutput)
	}
}
