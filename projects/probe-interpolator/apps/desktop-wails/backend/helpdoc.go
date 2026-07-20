package backend

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// helpdoc.go 抽取 5 / 3 / 7 孔"打开用户说明书 HTML"的公共逻辑。
//
// 3 个探针 service 原本各自实现：
//   - getXxxHelpDocPath(fileName)：在 exe 附近若干级目录查找 docs/<fileName>
//   - OpenXxxHelpDoc()：跨平台调用系统默认程序打开 HTML
//
// 抽到此处后 service 仅保留各探针的文件名常量和方法签名（Wails binding 要求方法名唯一）。

// GetHelpDocPath 在可执行文件附近查找用户说明书 HTML。
// 查找顺序：exe 同级 docs/ → 上 1 级 docs/ → 上 2 级 docs/。
// 开发模式下 docs/ 通常在向上若干级的目录，依次尝试兼容 dev 与 release 布局。
// 找不到返回空字符串，调用方应给出友好错误。
func GetHelpDocPath(fileName string) string {
	ex, err := os.Executable()
	if err != nil {
		return ""
	}
	exeDir := filepath.Dir(ex)

	possiblePaths := []string{
		filepath.Join(exeDir, "docs", fileName),
		filepath.Join(exeDir, "..", "docs", fileName),
		filepath.Join(exeDir, "..", "..", "docs", fileName),
	}

	for _, p := range possiblePaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// OpenHelpDocByPath 用系统默认程序打开用户说明书 HTML 文件。
// 跨平台：Windows 用 cmd start，macOS 用 open，Linux 用 xdg-open。
//
// 用 Run 等待命令完成并捕获退出状态，避免 start 子进程失败但主进程已返回导致的静默错误。
func OpenHelpDocByPath(helpPath string) error {
	if helpPath == "" {
		return fmt.Errorf("未找到用户说明书文件")
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", helpPath)
	case "darwin":
		cmd = exec.Command("open", helpPath)
	default:
		cmd = exec.Command("xdg-open", helpPath)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("打开帮助文档失败: %w", err)
	}
	return nil
}
