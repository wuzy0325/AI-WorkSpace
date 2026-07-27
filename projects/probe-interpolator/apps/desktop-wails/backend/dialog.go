package backend

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// dialog.go 抽取 5 / 3 / 7 孔"打开文件选择对话框"的公共逻辑。
//
// 3 个探针 service 原本各自实现了 openXxxFileDialog(title) 方法，函数体完全一致：
//   - 取 application 单例（application.Get()）
//   - 链式 SetTitle + AddFilter("PRB Files") + AddFilter("All Files")
//
// 抽到此处后 service 仅调用 a.openFileDialog(title) 即可。
// 调用方仍可链式追加 CSV/TXT/DAT 等过滤器（5 孔仅 .prb，3/7 孔在 CSV 导入时追加）。
//
// 方法名原为 openPrbFileDialog，v0.2.0 改名为 openFileDialog——对话框不再限于 PRB，
// 7 孔 CSV 导入（PickSevenHoleFiles / ImportSevenHoleCsvData）也复用此入口。

// openFileDialog 创建文件选择对话框，预设 .prb 过滤器。
// 调用方可链式 AddFilter 覆盖或追加过滤规则（例如 CSV 导入时追加 .csv/.txt/.dat）。
//
// 注意：方法名首字母小写，不会被 Wails binding 暴露到前端。
//
// 实现说明：直接调用 application.Get() 取全局单例。Wails 启动时已通过 application.New()
// 初始化全局实例，调用本方法时单例一定存在，故无需 a.app 字段做 fallback
// （参见 app.go ServiceStartup 注释解释了为何移除 a.app 字段）。
func (a *App) openFileDialog(title string) *application.OpenFileDialogStruct {
	return application.Get().Dialog.OpenFile().
		SetTitle(title).
		AddFilter("PRB Files (*.prb)", "*.prb").
		AddFilter("All Files (*.*)", "*.*")
}
