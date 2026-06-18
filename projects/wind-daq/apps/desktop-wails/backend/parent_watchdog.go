// parent_watchdog.go — 父进程看护协程，仅 ModeMotion 子进程使用。
//
// 背景：OpenMotionWindow 用 exec.Command 拉起一个独立 GUI 进程作为运动控制器
// 单独窗口。父进程优雅退出时（Wails Shutdown）会主动 Kill 子进程，但任务管理器
// 强杀父进程不会触发 Shutdown，子进程会成为孤儿。本 watchdog 周期性探测父进程
// 是否仍存活，发现已退出则自动结束本进程。
//
// 跨平台实现：os.FindProcess 在 Windows 下会真正 OpenProcess，PID 不存在时直接
// 返回 error；在 Linux/macOS 下永远成功，需要再用 Signal(0) 探测。封装在
// processIsAlive 中。
package backend

import (
	"log"
	"os"
	"time"
)

// parentWatchdogInterval 父进程探测周期。1 秒在感知与开销间权衡：
// 任务管理器强杀后最多 1 秒内本进程退出。
const parentWatchdogInterval = 1 * time.Second

// startParentWatchdog 启动父进程看护协程。
// 仅 ModeMotion 子进程在 Startup 时调用，且要求 a.parentPID > 0。
func (a *App) startParentWatchdog() {
	pid := a.parentPID
	if pid <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(parentWatchdogInterval)
		defer ticker.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				if !processIsAlive(pid) {
					log.Printf("父进程 (pid=%d) 已退出，运动控制器独立窗口随之关闭", pid)
					// 直接 os.Exit，绕过 Wails Shutdown：父进程已不存在，
					// 任何持久化或事件回调也没有接收方，快速退出最干净。
					os.Exit(0)
				}
			}
		}
	}()
}
