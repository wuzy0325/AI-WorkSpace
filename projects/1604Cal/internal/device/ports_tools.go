package device

import (
	"context"
	"strings"
)

// ---------------------------------------------------------------------------
// 端口层工具函数
//
// 这些函数原本散落在 infrastructure/driver 包，但 application 层需要调用它们。
// 为遵守"usecase 不得导入 adapters"的依赖方向约束，将纯工具函数上移到 ports 层，
// driver 包保留同名导出函数作为转发，维持驱动内部调用不变。
// ---------------------------------------------------------------------------

// NormalizePressureUnit 将单位字符串规范化为标准大小写形式。
// 设备可能返回全小写（如 "kpa"、"mmhg"），前端和显示需要标准形式（如 "kPa"、"mmHg"）。
// 此函数为纯字符串映射，无 I/O，可安全放在 ports 层。
func NormalizePressureUnit(unit string) string {
	m := map[string]string{
		"pa": "Pa", "kpa": "kPa", "mpa": "MPa",
		"bar": "bar", "mbar": "mbar", "psi": "psi",
		"kgf/cm2": "kgf/cm2", "mmhg": "mmHg", "atm": "atm", "inhg": "inHg",
	}
	if normalized, ok := m[strings.ToLower(strings.TrimSpace(unit))]; ok {
		return normalized
	}
	return unit
}

// contextKeyPollType 是 context 中标记"轮询"的键类型。
// 轮询循环设置此标记后，driver 层在发布硬件事件时会附带 "poll": true，
// 前端可据此过滤轮询日志，避免高频轮询刷屏。
type contextKeyPollType struct{}

// ContextKeyPoll 是 context 中标记"轮询"的键。
// 导出此变量以便 driver 包（adapters 层）读取同一 key，保持 context 跨层传递一致性。
var ContextKeyPoll = contextKeyPollType{}

// WithPollContext 返回一个标记了"轮询"的新 context。
// application 层调用此函数为轮询操作打标，driver 层通过 IsPollContext 读取标记。
func WithPollContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, ContextKeyPoll, true)
}

// IsPollContext 检查 context 中是否标记了轮询操作。
// driver 层调用此函数决定是否在硬件事件中附带 "poll": true 标记。
func IsPollContext(ctx context.Context) bool {
	v := ctx.Value(ContextKeyPoll)
	b, _ := v.(bool)
	return b
}
