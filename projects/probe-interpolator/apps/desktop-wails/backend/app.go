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

// ServiceStartup 是 Wails 服务生命周期钩子。
//
// 历史上这里曾保存 ctx 与 app 引用，但后续 service 实际通过 application.Get()
// 在调用点取单例（dialog.go / seven_hole_service.go），未读取过字段，故移除字段
// 以消除"实例字段 vs 全局单例"双源回退带来的初始化时序歧义
// （参见 project_memory：双源获取让测试与初始化时序变得不可预测）。
// Wails 启动时已调用 application.New()，application.Get() 在 ServiceStartup 之后
// 任意调用点都返回同一单例。
func (a *App) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	_ = ctx
	_ = options
	return nil
}
