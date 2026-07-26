package slog

import "sync/atomic"

// Level 日志级别。值与标准库 log/slog 对齐，方便未来迁移到标准库。
type Level int

const (
	LevelDebug Level = -4
	LevelInfo  Level = 0
	LevelWarn  Level = 4
	LevelError Level = 8
)

// Leveler 接口：能返回当前 Level 的对象（LevelVar 实现）。
// 用于 HandlerOptions.Level 字段，允许在运行时动态调整级别。
type Leveler interface {
	Level() Level
}

// LevelVar 是可变的 Level 容器，允许多个 goroutine 通过 Set/Level
// 在运行时调整日志级别（如根据 config 热更新）。
// 用 int32 atomic 兼容 Go 1.19 及更早版本（无 atomic.Int32 类型）。
type LevelVar struct {
	val int32
}

// Level 返回当前级别。nil receiver 视为 LevelInfo。
func (l *LevelVar) Level() Level {
	if l == nil {
		return LevelInfo
	}
	return Level(atomic.LoadInt32(&l.val))
}

// Set 设置当前级别。nil receiver 是 no-op。
func (l *LevelVar) Set(level Level) {
	if l == nil {
		return
	}
	atomic.StoreInt32(&l.val, int32(level))
}

// String 返回 Level 的字符串表示，用于 TextHandler 输出。
// 与标准库 slog.Level.String 略有差异：标准库对非标准级别返回 "INFO+2" 形式，
// 本实现只返回 DEBUG/INFO/WARN/ERROR 四档，因为业务侧未使用自定义级别。
func (l Level) String() string {
	switch {
	case l < LevelInfo:
		return "DEBUG"
	case l < LevelWarn:
		return "INFO"
	case l < LevelError:
		return "WARN"
	default:
		return "ERROR"
	}
}

// Level 让 Level 类型自身实现 Leveler 接口，与标准库 slog.Level 对齐。
// 这样 HandlerOptions{Level: slog.LevelInfo} 可直接使用，无需包装成 LevelVar。
func (l Level) Level() Level { return l }

// MarshalJSON 实现 json.Marshaler，便于级别作为字段输出。
// 与标准库 slog.Level.MarshalJSON 行为一致。
func (l Level) MarshalJSON() ([]byte, error) {
	return []byte("\"" + l.String() + "\""), nil
}