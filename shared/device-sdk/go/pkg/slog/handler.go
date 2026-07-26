package slog

import (
	"context"
	"io"
	"strconv"
	"sync"
)

// Handler 是日志处理器接口。
// 实现者：TextHandler、wind-daq 的 RingHandler 和 fanoutHandler。
type Handler interface {
	// Enabled 判断给定级别是否会被输出。返回 false 时 Logger 跳过构造 Record。
	Enabled(ctx context.Context, level Level) bool
	// Handle 处理一条 Record。
	Handle(ctx context.Context, r Record) error
	// WithAttrs 返回带额外 attrs 的 Handler 副本。
	WithAttrs(attrs []Attr) Handler
	// WithGroup 返回追加 group name 的 Handler 副本。
	WithGroup(name string) Handler
}

// HandlerOptions 控制 TextHandler 的输出选项。
type HandlerOptions struct {
	// Level 级别过滤器，nil 表示 INFO。通常传入 *LevelVar 实现运行时级别调整。
	Level Leveler
	// AddSource 是否附带源码位置（业务侧未使用，本实现忽略）。
	AddSource bool
	// ReplaceAttr 属性重写钩子（业务侧未使用，本实现忽略）。
	ReplaceAttr func(groups []string, a Attr) Attr
}

// effectiveLevel 返回 HandlerOptions.Level 的有效级别，nil 时默认 INFO。
func (h *HandlerOptions) effectiveLevel() Level {
	if h == nil || h.Level == nil {
		return LevelInfo
	}
	return h.Level.Level()
}

// TextHandler 把日志以 key=val 文本形式写入 io.Writer。
// 与标准库 slog.TextHandler 输出格式一致：
//   time=2006-01-02T15:04:05.000-07:00 level=INFO msg="..." key=val key=val
type TextHandler struct {
	w      io.Writer
	opts   *HandlerOptions
	attrs  []Attr
	groups []string
	mu     sync.Mutex // 串行化 Write，避免多 goroutine 输出交错
}

// NewTextHandler 构造一个 TextHandler。opts 为 nil 时使用默认值。
func NewTextHandler(w io.Writer, opts *HandlerOptions) *TextHandler {
	if opts == nil {
		opts = &HandlerOptions{}
	}
	return &TextHandler{w: w, opts: opts}
}

// Enabled 实现 Handler 接口。
func (h *TextHandler) Enabled(_ context.Context, level Level) bool {
	return level >= h.opts.effectiveLevel()
}

// Handle 实现 Handler 接口。把 Record 格式化为文本后写入 io.Writer。
func (h *TextHandler) Handle(_ context.Context, r Record) error {
	var b []byte
	b = append(b, "time="...)
	b = r.Time.AppendFormat(b, "2006-01-02T15:04:05.000Z07:00")
	b = append(b, " level="...)
	b = append(b, r.Level.String()...)
	b = append(b, " msg="...)
	b = append(b, quoteString(r.Message)...)

	// 先输出 WithAttrs 链路上的累积属性，再输出本次 Record 的属性
	for _, a := range h.attrs {
		b = appendAttrText(b, h.groups, a)
	}
	r.Attrs(func(a Attr) bool {
		b = appendAttrText(b, h.groups, a)
		return true
	})

	b = append(b, '\n')

	h.mu.Lock()
	_, err := h.w.Write(b)
	h.mu.Unlock()
	return err
}

// WithAttrs 实现 Handler 接口。
func (h *TextHandler) WithAttrs(attrs []Attr) Handler {
	if len(attrs) == 0 {
		return h
	}
	merged := make([]Attr, 0, len(h.attrs)+len(attrs))
	merged = append(merged, h.attrs...)
	merged = append(merged, attrs...)
	return &TextHandler{
		w:      h.w,
		opts:   h.opts,
		attrs:  merged,
		groups: h.groups,
	}
}

// WithGroup 实现 Handler 接口。
func (h *TextHandler) WithGroup(name string) Handler {
	if name == "" {
		return h
	}
	groups := make([]string, 0, len(h.groups)+1)
	groups = append(groups, h.groups...)
	groups = append(groups, name)
	return &TextHandler{
		w:      h.w,
		opts:   h.opts,
		attrs:  h.attrs,
		groups: groups,
	}
}

// appendAttrText 把 Attr 追加到字节缓冲，格式 " key=val"。
// 处理 Group Attr：递归展开为 "group.key=val"。
func appendAttrText(b []byte, groups []string, a Attr) []byte {
	// Group Attr 展开
	if a.Value.Kind() == KindGroup {
		groupAttrs := a.Value.GroupValue()
		if len(groupAttrs) == 0 {
			return b
		}
		newGroups := make([]string, 0, len(groups)+1)
		newGroups = append(newGroups, groups...)
		newGroups = append(newGroups, a.Key)
		for _, sub := range groupAttrs {
			b = appendAttrText(b, newGroups, sub)
		}
		return b
	}

	b = append(b, ' ')
	for _, g := range groups {
		b = append(b, g...)
		b = append(b, '.')
	}
	b = append(b, a.Key...)
	b = append(b, '=')
	b = append(b, quoteString(a.Value.String())...)
	return b
}

// quoteString 对包含空格/特殊字符的字符串加引号。
// 与标准库 slog 行为一致：纯字母数字下划线连字符不加引号，否则用 strconv.Quote。
func quoteString(s string) string {
	if s == "" {
		return "\"\""
	}
	needQuote := false
	for _, r := range s {
		if r <= ' ' || r == '"' || r == '=' || r == '\\' {
			needQuote = true
			break
		}
	}
	if !needQuote {
		return s
	}
	return strconv.Quote(s)
}