//go:build windows

package backend

import (
	"syscall"
)

// processIsAlive 探测指定 PID 的进程是否仍在运行（Windows 实现）。
// 与 POSIX 不同，Windows 下 os.FindProcess 会调用 OpenProcess，PID 不存在或
// 已退出时直接返回 error。但更可靠的做法是直接调 OpenProcess + GetExitCodeProcess：
//   - OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, ...) 失败 → 进程已不存在。
//   - GetExitCodeProcess 返回 STILL_ACTIVE (259) → 仍在运行。
//   - 其他 → 已退出。
// 这样能避开 os.FindProcess 在某些场景拿到僵尸句柄的问题。
func processIsAlive(pid int) bool {
	const (
		processQueryLimitedInformation = 0x1000
		stillActive                    = 259
	)
	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		// 常见是 ERROR_INVALID_PARAMETER（PID 无效）或 ERROR_ACCESS_DENIED
		// 后者出现在跨权限/Session 时；本场景父子同源进程，遇到 access denied 视为存活更保守。
		if err == syscall.ERROR_ACCESS_DENIED {
			return true
		}
		return false
	}
	defer syscall.CloseHandle(handle)

	var code uint32
	if err := syscall.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code == stillActive
}
