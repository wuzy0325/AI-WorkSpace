package slog

import (
	"context"
	"log"
	"sync"
	"time"
)

// Logger 是业务侧使用的日志器。通过 With/WithGroup 链式追加属性。
// 与标准库 slog.Logger 行为一致。
type Logger struct {
	handler Handler
	attrs   []Attr
	groups  []string
}

// New 构造一个 Logger。handler 为 nil 时使用默认 TextHandler。
func New(h Handler) *Logger {
	if h == nil {
		h = NewTextHandler(log.Default().Writer(), nil)
	}
	return &Logger{handler: h}
}

// Handler 返回 Logger 内部 handler。
func (l *Logger) Handler() Handler {
	if l == nil {
		return nil
	}
	return l.handler
}

// Enabled 判断给定级别是否会被输出。
// 业务侧可在构造复杂 Record 前调用以避免无效工作。
func (l *Logger) Enabled(ctx context.Context, level Level) bool {
	if l == nil || l.handler == nil {
		return false
	}
	return l.handler.Enabled(ctx, level)
}

// LogAttrs 提交一条日志（attrs 形式）。
// 与标准库 slog.Logger.LogAttrs 签名一致。
func (l *Logger) LogAttrs(ctx context.Context, level Level, msg string, attrs ...Attr) {
	if !l.Enabled(ctx, level) {
		return
	}
	r := NewRecord(time.Now(), level, msg, 0)
	// 先追加 Logger.With 链路上的属性，再追加本次调用传入的 attrs
	if len(l.attrs) > 0 {
		r.AddAttrs(l.attrs...)
	}
	if len(attrs) > 0 {
		r.AddAttrs(attrs...)
	}
	_ = l.handler.Handle(ctx, r)
}

// Log 提交一条日志（args 形式）。
// args 按 key-value 对解析：奇数位置为 key（string），偶数位置为 value。
func (l *Logger) Log(ctx context.Context, level Level, msg string, args ...any) {
	if !l.Enabled(ctx, level) {
		return
	}
	r := NewRecord(time.Now(), level, msg, 0)
	if len(l.attrs) > 0 {
		r.AddAttrs(l.attrs...)
	}
	r.AddAttrs(argsToAttrs(args)...)
	_ = l.handler.Handle(ctx, r)
}

// Info Info 级别日志。
func (l *Logger) Info(msg string, args ...any) {
	l.Log(context.Background(), LevelInfo, msg, args...)
}

// Warn Warn 级别日志。
func (l *Logger) Warn(msg string, args ...any) {
	l.Log(context.Background(), LevelWarn, msg, args...)
}

// Error Error 级别日志。
func (l *Logger) Error(msg string, args ...any) {
	l.Log(context.Background(), LevelError, msg, args...)
}

// Debug Debug 级别日志。
func (l *Logger) Debug(msg string, args ...any) {
	l.Log(context.Background(), LevelDebug, msg, args...)
}

// InfoCtx 带 ctx 的 Info 日志。
func (l *Logger) InfoCtx(ctx context.Context, msg string, args ...any) {
	l.Log(ctx, LevelInfo, msg, args...)
}

// WarnCtx 带 ctx 的 Warn 日志。
func (l *Logger) WarnCtx(ctx context.Context, msg string, args ...any) {
	l.Log(ctx, LevelWarn, msg, args...)
}

// ErrorCtx 带 ctx 的 Error 日志。
func (l *Logger) ErrorCtx(ctx context.Context, msg string, args ...any) {
	l.Log(ctx, LevelError, msg, args...)
}

// DebugCtx 带 ctx 的 Debug 日志。
func (l *Logger) DebugCtx(ctx context.Context, msg string, args ...any) {
	l.Log(ctx, LevelDebug, msg, args...)
}
// With 返回带额外 attrs 的 Logger 副本。
// args 按 key-value 对解析，与顶级 Info/Warn/Error/Debug 一致。
func (l *Logger) With(args ...any) *Logger {
	if l == nil {
		return nil
	}
	merged := make([]Attr, 0, len(l.attrs)+len(args)/2+1)
	merged = append(merged, l.attrs...)
	merged = append(merged, argsToAttrs(args)...)
	return &Logger{
		handler: l.handler,
		attrs:   merged,
		groups:  l.groups,
	}
}

// WithGroup 返回追加 group name 的 Logger 副本。
func (l *Logger) WithGroup(name string) *Logger {
	if l == nil {
		return nil
	}
	if name == "" {
		return l
	}
	groups := make([]string, 0, len(l.groups)+1)
	groups = append(groups, l.groups...)
	groups = append(groups, name)
	return &Logger{
		handler: l.handler,
		attrs:   l.attrs,
		groups:  groups,
	}
}

// argsToAttrs 把 key-value 对 args 转换为 Attr 切片。
// 奇数个 args 时最后一个视为 key="?" 兜底。
func argsToAttrs(args []any) []Attr {
	if len(args) == 0 {
		return nil
	}
	attrs := make([]Attr, 0, len(args)/2+1)
	for i := 0; i < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			continue
		}
		if i+1 < len(args) {
			attrs = append(attrs, Any(key, args[i+1]))
		} else {
			attrs = append(attrs, String(key, "?"))
		}
	}
	return attrs
}
// ----- Default Logger 管理 -----
//
// defaultLogger 是包级默认 Logger，由 init() 初始化为输出到 log.Default() 的 TextHandler。
// 业务侧通过 slog.Default() 获取并通过 slog.SetDefault(l) 替换（如 wind-daq logging 包
// 在 Manager 启动时把 defaultLogger 切换为 fanout logger）。
//
// defaultLevelVar 是与 defaultLogger 共享级别的 LevelVar，业务侧可通过 SetLevel 调整
// 默认 Logger 的级别（向后兼容 daq-t1603 的 SetLevel 用法）。

var (
	defaultMu       sync.Mutex
	defaultLogger   *Logger
	defaultLevelVar = &LevelVar{}
)

func init() {
	// 默认 INFO 级别。defaultLevelVar 是 *LevelVar（实现 Leveler），
	// 业务侧调用 SetLevel 时会同步影响 defaultLogger 的 TextHandler。
	defaultLevelVar.Set(LevelInfo)
	defaultLogger = New(NewTextHandler(log.Default().Writer(), &HandlerOptions{
		Level: defaultLevelVar,
	}))
}

// Default 返回包级默认 Logger。永不返回 nil。
// 业务侧应优先使用 Default() 而非自己 New 一个 Logger。
func Default() *Logger {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	return defaultLogger
}

// SetDefault 替换包级默认 Logger。l 为 nil 时忽略。
// 后续 Default() 调用返回新的 Logger；已持有旧 Logger 引用的代码不受影响。
func SetDefault(l *Logger) {
	if l == nil {
		return
	}
	defaultMu.Lock()
	defaultLogger = l
	defaultMu.Unlock()
}