package backend

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// dialog.go 抽取 5 / 3 / 7 孔"打开 PRB 文件选择对话框"的公共逻辑。
//
// 3 个探针 service 原本各自实现了 openXxxFileDialog(title) 方法，函数体完全一致：
//   - 取 a.app（fallback 到 application.Get()）
//   - 链式 SetTitle + AddFilter("PRB Files") + AddFilter("All Files")
//
// 抽到此处后 service 仅调用 a.openPrbFileDialog(title) 即可。
// 调用方仍可链式追加 CSV/TXT/DAT 等过滤器（5 孔仅 .prb，3/7 孔在 CSV 导入时追加）。

// openPrbFileDialog 创建文件选择对话框，预设 .prb 过滤器。
// 调用方可链式 AddFilter 覆盖或追加过滤规则（例如 CSV 导入时追加 .csv/.txt/.dat）。
//
// 注意：方法名首字母小写，不会被 Wails binding 暴露到前端。
func (a *App) openPrbFileDialog(title string) *application.OpenFileDialogStruct {
	app := a.app
	if app == nil {
		app = application.Get()
	}
	return app.Dialog.OpenFile().
		SetTitle(title).
		AddFilter("PRB Files (*.prb)", "*.prb").
		AddFilter("All Files (*.*)", "*.*")
}
