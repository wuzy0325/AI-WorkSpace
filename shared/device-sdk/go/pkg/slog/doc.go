// Package slog 提供 Go 1.20 项目用的 slog 标准库 API 兼容实现。
//
// 标准库 log/slog 在 Go 1.21 才引入，本包把它的核心 API 移植到 Go 1.20，
// 让 wind-daq / daq-t1603 等 Win7 LTS 工作线项目可以零改动复用主线代码。
//
// 已实现的 API 表面（与标准库 slog 对齐）：
//   - 顶级函数：Info / Debug / Warn / Error / Log / LogAttrs / Default / SetDefault
//   - 顶级构造函数：String / Int / Int64 / Bool / Float64 / Duration / Time / Any / Group
//   - 类型：Logger / Handler / Attr / Value / Record / LevelVar / Leveler / HandlerOptions
//   - Logger 方法：Info / Warn / Error / Debug / Log / LogAttrs / With / WithGroup / Enabled / Handler
//   - Handler 实现：NewTextHandler（key=val 文本输出，与标准库 slog.TextHandler 一致）
//
// 未实现（业务侧未使用，避免过度实现）：
//   - JSONHandler / CommonHandler
//   - Source / LogValuer 接口
//   - PC / AddSource 源码位置追踪（业务侧未使用，简化为占位字段）
//
// 与标准库的差异（必须注意）：
//   1. 顶级 SetLevel/CurrentLevel 是本包独有的兼容 API，操作 default Logger 的 LevelVar；
//      调用 SetDefault 装载新 Logger 后，本函数不影响新 Logger 的级别。
//   2. 默认 TextHandler 输出格式为 time=... level=INFO msg=... key=val，
//      与标准库 slog 一致；与原 daq-t1603 用的 [INFO] msg key=val 格式不同
//      （daq-t1603 代码未对日志格式做断言，升级安全）。
//   3. LevelVar.Set / Level 基于 atomic int32，兼容 Go 1.19+。
package slog