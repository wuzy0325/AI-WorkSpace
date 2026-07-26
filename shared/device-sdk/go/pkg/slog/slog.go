// Package slog 提供 slog 标准库 API 的最小化实现，
// 用于 Go 1.20 项目（标准库 log/slog 在 Go 1.21+ 才有）。
//
// 本文件提供顶级函数 Info/Debug/Warn/Error/Log/LogAttrs 与 SetLevel/CurrentLevel。
// 类型与构造器分别在 level.go / attr.go / record.go / handler.go / logger.go 中实现。
//
// 顶级函数委托给 defaultLogger（见 logger.go 的 Default() / SetDefault()）。
// SetLevel 直接调整 defaultLevelVar，影响 defaultLogger 的 TextHandler 级别过滤。
package slog

import (
	"context"
)

// SetLevel 调整默认 Logger 的级别。
// 通过修改 defaultLevelVar 实现，defaultLogger 的 TextHandler 引用同一个 LevelVar，
// 因此 SetLevel 后立即生效（无需重建 Logger）。
//
// 向后兼容 daq-t1603 的早期 SetLevel 用法。
func SetLevel(level Level) {
	defaultLevelVar.Set(level)
}

// CurrentLevel 返回默认 Logger 当前生效的级别。
// 与 SetLevel 配对使用，便于业务侧持久化/恢复日志配置。
func CurrentLevel() Level {
	return defaultLevelVar.Level()
}

// Log 提交一条日志到默认 Logger。
// args 按 key-value 对解析：奇数位置为 key（string），偶数位置为 value。
// 与标准库 slog.Log 行为一致。
func Log(ctx context.Context, level Level, msg string, args ...any) {
	Default().Log(ctx, level, msg, args...)
}

// LogAttrs 提交一条日志到默认 Logger（attrs 形式）。
// 与标准库 slog.LogAttrs 行为一致：避免 args 解析开销，适合高频日志路径。
func LogAttrs(ctx context.Context, level Level, msg string, attrs ...Attr) {
	Default().LogAttrs(ctx, level, msg, attrs...)
}

// Info Info 级别日志。
// args 按 key-value 对解析（如 "device", "P1603", "channel", 5）。
func Info(msg string, args ...any) {
	Default().Log(context.Background(), LevelInfo, msg, args...)
}

// Debug Debug 级别日志。
func Debug(msg string, args ...any) {
	Default().Log(context.Background(), LevelDebug, msg, args...)
}

// Warn Warn 级别日志。
func Warn(msg string, args ...any) {
	Default().Log(context.Background(), LevelWarn, msg, args...)
}

// Error Error 级别日志。
func Error(msg string, args ...any) {
	Default().Log(context.Background(), LevelError, msg, args...)
}

// InfoCtx 带 ctx 的 Info 日志。
func InfoCtx(ctx context.Context, msg string, args ...any) {
	Default().Log(ctx, LevelInfo, msg, args...)
}

// WarnCtx 带 ctx 的 Warn 日志。
func WarnCtx(ctx context.Context, msg string, args ...any) {
	Default().Log(ctx, LevelWarn, msg, args...)
}

// ErrorCtx 带 ctx 的 Error 日志。
func ErrorCtx(ctx context.Context, msg string, args ...any) {
	Default().Log(ctx, LevelError, msg, args...)
}

// DebugCtx 带 ctx 的 Debug 日志。
func DebugCtx(ctx context.Context, msg string, args ...any) {
	Default().Log(ctx, LevelDebug, msg, args...)
}