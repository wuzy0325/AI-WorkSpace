package backend

import "github.com/wailsapp/wails/v3/pkg/application"

// pickDirectory 打开系统目录选择对话框，返回用户选定的绝对路径。
// 被 LogService 和 RecordingService 共用，避免重复实现。
func pickDirectory(app *application.App) (string, error) {
	if app == nil {
		app = application.Get()
	}
	return app.Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true).
		SetTitle("选择保存目录").
		PromptForSingleSelection()
}
