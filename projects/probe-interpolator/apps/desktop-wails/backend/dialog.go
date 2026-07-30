package backend

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// dialog.go 抽取 5 / 3 / 7 孔"打开文件选择对话框"的公共入口。
//
// 3 个探针 service 原本各自实现了 openXxxFileDialog(title) 方法，函数体完全一致：
//   - 取 application 单例（application.Get()）
//   - 链式 SetTitle + AddFilter("PRB Files") + AddFilter("All Files")
//
// 抽到此处后 service 仅调用 a.openFileDialog(title) 即可。
//
// v0.1.1 起 openFileDialog 仅设置标题，不预设任何过滤器：
// 调用方必须按场景链式 AddFilter（PRB 加载加 .prb，CSV 导入加 .csv/.txt/.dat）
// 并按需追加 All Files 兜底，避免不同场景的过滤器互相污染
// （例如 CSV 导入对话框不应出现 PRB 文件类型选项）。
//
// 注意：方法名首字母小写，不会被 Wails binding 暴露到前端。
//
// 实现说明：直接调用 application.Get() 取全局单例。Wails 启动时已通过 application.New()
// 初始化全局实例，调用本方法时单例一定存在，故无需 a.app 字段做 fallback
// （参见 app.go ServiceStartup 注释解释了为何移除 a.app 字段）。
func (a *App) openFileDialog(title string) *application.OpenFileDialogStruct {
	return application.Get().Dialog.OpenFile().
		SetTitle(title)
}
