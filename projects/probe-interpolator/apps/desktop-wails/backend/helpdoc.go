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
//
// 查找顺序（依次尝试）：
//  1. exe 同级 docs/（NSIS 安装后的标准结构：$INSTDIR/docs/<fileName>）
//  2. exe 上 1/2/3/4 级 docs/（开发模式：build/bin/probe-interpolator.exe
//     向上 2 级到 apps/desktop-wails/docs/；wails dev 临时目录可能更深）
//  3. 当前工作目录及上 1/2 级 docs/（Windows 开发模式 wails dev 兜底）
//
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
		filepath.Join(exeDir, "..", "..", "..", "docs", fileName),
		filepath.Join(exeDir, "..", "..", "..", "..", "docs", fileName),
	}

	// Windows 开发模式下 wails dev 可能使用临时目录作为 exe 路径，
	// 此时工作目录才是项目根，补充从 cwd 查找的兜底。
	if runtime.GOOS == "windows" {
		if cwd, err := os.Getwd(); err == nil {
			possiblePaths = append(possiblePaths,
				filepath.Join(cwd, "docs", fileName),
				filepath.Join(cwd, "..", "docs", fileName),
				filepath.Join(cwd, "..", "..", "docs", fileName),
			)
		}
	}

	for _, p := range possiblePaths {
		// 用 Clean 规范化路径（解析 .. 等），并校验命中是文件而非目录。
		cleanPath := filepath.Clean(p)
		if info, err := os.Stat(cleanPath); err == nil && !info.IsDir() {
			return cleanPath
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
