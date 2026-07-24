// Package backend 是 probe-interpolator 桌面程序的后端入口。
//
// Win7 分支与 trunk 主分支的差异：
//   - 移除 *application.App 依赖（Wails v3），App 仅持有 ctx
//   - ServiceStartup 签名从 (ctx, application.ServiceOptions) 改为 (ctx)
//   - 文件选择对话框改由前端 Electron IPC 处理，LoadPrb/ImportCsv 接收文件路径参数
//   - OpenHelpDoc 保留 exec.Command 调用系统默认程序打开 HTML
//
// v0.1.0 当前阶段提供：
//   - App struct（HTTP 后端服务根对象）
//   - probe_selector.go（启动选择页后端：GetAvailableProbes / SetActiveProbe / GetActiveProbe）
//   - five_hole_service.go / three_hole_service.go / seven_hole_service.go（5/3/7 孔插值）
package backend

import (
	"context"
)

// App 是后端服务根对象。
// 所有 HTTP handler 调用的方法都挂在 App 上。
type App struct {
	ctx context.Context

	// selector 管理"会话内探针类型固定"状态，自带 RWMutex，
	// 与后续各探针 service 的锁隔离，避免混用。
	selector probeSelector

	// fiveHole 是 5 孔探针插值的运行时状态，自带 RWMutex。
	fiveHole fiveHoleState

	// threeHole 是 3 孔探针插值的运行时状态，自带 RWMutex。
	threeHole threeHoleState

	// sevenHole 是 7 孔探针插值的运行时状态，自带 RWMutex。
	// 与 5 孔 / 3 孔 state 隔离，避免锁混用（SPEC § Boundaries 要求）。
	sevenHole sevenHoleState
}

// NewApp 构造 App 实例，selector 字段零值即"未选择"状态。
func NewApp() *App {
	return &App{}
}

// ServiceStartup 应用启动回调（Win7 分支：仅 ctx，无 application.ServiceOptions）。
// 保留 ctx 引用以备后用（如后续需要取消长时间运行的批量计算）。
func (a *App) ServiceStartup(ctx context.Context) error {
	a.ctx = ctx
	return nil
}

// ServiceShutdown 应用关闭回调。
// 当前无后台 goroutine 需要停止，保留方法以备后续扩展。
func (a *App) ServiceShutdown() error {
	return nil
}
