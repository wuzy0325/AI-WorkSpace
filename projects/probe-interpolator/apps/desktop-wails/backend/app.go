// Package backend 是 probe-interpolator 桌面程序的 Wails 后端入口。
// v0.1.0 当前阶段提供：
//   - App struct（Wails 服务根对象，绑定到前端）
//   - probe_selector.go（启动选择页后端：GetAvailableProbes / SetActiveProbe / GetActiveProbe）
//
// 后续任务会按探针类型追加 five_hole_service / three_hole_service / seven_hole_service。
package backend

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// App 是 Wails 后端服务根对象，绑定到前端。
// 所有前端可调用的方法都挂在 App 上（Wails v3 的 binding 机制要求）。
type App struct {
	ctx context.Context
	app *application.App

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

// ServiceStartup 是 Wails 服务生命周期钩子，保存 ctx 与 app 引用以备后用。
// probe_selector 不需要 ctx/app，但后续 five_hole_service 等会用到（如打开文件对话框）。
func (a *App) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	a.ctx = ctx
	a.app = application.Get()
	return nil
}
