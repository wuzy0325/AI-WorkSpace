// Package slog 提供 slog 标准库 API 的最小化实现，
// 用于 Go 1.20 项目（标准库 log/slog 在 Go 1.21+ 才有）。
//
// 仅实现 daq-t1603 业务代码实际使用的 API：
//   - 顶级函数 Info/Debug/Warn/Error（接受 msg + key-value 对）
//   - Level 常量 LevelInfo/LevelDebug/LevelWarn/LevelError
//   - SetLevel 函数
//
// 输出格式：[LEVEL] msg key=val key=val
// 日志级别过滤通过 SetLevel 控制，默认 Info。
package slog

import (
	"fmt"
	"log"
	"strings"
	"sync/atomic"
)

// Level 日志级别。值与标准库 log/slog 对齐，方便未来迁移。
type Level int

const (
	LevelDebug Level = -4
	LevelInfo  Level = 0
	LevelWarn  Level = 4
	LevelError Level = 8
)

// globalLevel 全局日志级别。原子读写保证并发安全。
// 用 int32 而非 atomic.Int32，兼容 Go 1.19 及更早版本。
var globalLevel int32 = int32(LevelInfo)

// SetLevel 设置全局日志级别。低于此级别的日志会被丢弃。
func SetLevel(level Level) {
	atomic.StoreInt32(&globalLevel, int32(level))
}

// CurrentLevel 返回当前全局日志级别。
func CurrentLevel() Level {
	return Level(atomic.LoadInt32(&globalLevel))
}

// Info Info 级别日志。
// args 按 key-value 对解析（如 "device", "P1603", "channel", 5）。
// 奇数个 args 时最后一个视为缺少值，输出 "?"。
func Info(msg string, args ...any) {
	if LevelInfo >= CurrentLevel() {
		log.Printf("[INFO] %s %s", msg, formatArgs(args))
	}
}

// Debug Debug 级别日志。
func Debug(msg string, args ...any) {
	if LevelDebug >= CurrentLevel() {
		log.Printf("[DEBUG] %s %s", msg, formatArgs(args))
	}
}

// Warn Warn 级别日志。
func Warn(msg string, args ...any) {
	if LevelWarn >= CurrentLevel() {
		log.Printf("[WARN] %s %s", msg, formatArgs(args))
	}
}

// Error Error 级别日志。
func Error(msg string, args ...any) {
	if LevelError >= CurrentLevel() {
		log.Printf("[ERROR] %s %s", msg, formatArgs(args))
	}
}

// formatArgs 将 key-value 对格式化为 "key=val key=val" 字符串。
func formatArgs(args []any) string {
	if len(args) == 0 {
		return ""
	}
	var sb strings.Builder
	for i := 0; i < len(args); i += 2 {
		if i > 0 {
			sb.WriteString(" ")
		}
		if i+1 < len(args) {
			sb.WriteString(fmt.Sprintf("%v=%v", args[i], args[i+1]))
		} else {
			sb.WriteString(fmt.Sprintf("%v=?", args[i]))
		}
	}
	return sb.String()
}