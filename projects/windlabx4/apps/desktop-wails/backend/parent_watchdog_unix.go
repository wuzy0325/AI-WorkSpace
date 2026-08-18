//go:build !windows

package backend

import (
	"errors"
	"os"
	"syscall"
)

// processIsAlive 探测指定 PID 的进程是否仍在运行（POSIX 实现）。
// os.FindProcess 在 Unix 系统永远成功，需用信号 0 探测。
//   - syscall.Signal(0) 不发送信号，仅做权限和存活检查。
//   - 返回 nil 表示进程存在且本进程有权访问。
//   - 返回 ESRCH 表示进程不存在；其他错误（如 EPERM）保守视为存活。
func processIsAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// 用 errors.Is 而非 == 比较；某些 libc/runtime 会包成 *os.SyscallError。
	// 只有真正 ESRCH 才视为进程消失，其它（EPERM 等）保守视为存活。
	if errors.Is(err, syscall.ESRCH) {
		return false
	}
	return true
}
