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

// parentWatchdogGrace 检测到父进程消失后给优雅关停留出的时间窗。
// 期间会调用 cancel() 让 ctx 派生的子协程退出、appContext 落盘任何挂起的
// 配置写。500ms 在感知（用户等待时长）和数据完整性之间折中。
const parentWatchdogGrace = 500 * time.Millisecond

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
					// 优雅关停：先 cancel app ctx 让派生协程退出（MotionStatusPoller 等），
					// 给可能正在执行的 fsync / 配置原子写一个 500ms 完成窗口，再 os.Exit。
					// 不调用 a.Shutdown：Wails Shutdown 假定 runtime 还在，但此处直接 os.Exit
					// 是为了避免 Wails GUI 主线程在已无父进程上下文下继续操作。
					if a.cancel != nil {
						a.cancel()
					}
					time.Sleep(parentWatchdogGrace)
					os.Exit(0)
				}
			}
		}
	}()
}
