// Win7 版应用入口（WindLabX4）
//
// 与 trunk 主分支的差异：
//   - 移除 Wails v3 依赖（Wails v3 内部用 log/slog + maps + slices，需 Go 1.21+）
//   - 改为 net/http 启动 HTTP server，前端由 Electron 22.3.27 加载
//   - GUI 由 Electron 主进程承载；Go binary 只负责 HTTP API + 静态资源 serve
//   - motion-only 子进程也通过同一 main.go 入口启动，监听独立端口 8901（避免与主进程 8900 冲突）
//
// HTTP 路由由 api.NewRouter 提供（在 services/api-go/api 包），main.go 仅做装配：
//   - /api/* → api.NewRouter 返回的 handler（已包装 CORS/recover/metrics 中间件）
//   - /      → frontend/dist 静态资源（前端 SPA）

package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"shared.local/device-sdk/go/pkg/slog"

	"windlabx4/apps/desktop-wails/backend"
	"windlabx4/services/api-go/api"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// 早期 panic recovery：捕获所有 main goroutine 启动期 panic 并写入 crash log。
	// 必要性：windowsgui 子系统下 stderr 不可见，Wails 日志系统尚未初始化时
	// 任何 panic 都会让进程静默退出，用户看不到任何错误。crash log 是唯一的诊断手段。
	defer func() {
		if r := recover(); r != nil {
			writeCrashLog(fmt.Sprintf("启动 panic: %v", r))
			os.Exit(1)
		}
	}()

	// 解析命令行参数：
	//   --motion-only 启动运动控制器独立窗口（子进程，监听独立端口 8901）
	//   --parent-pid  仅 motion-only 子进程使用，传入父进程 PID，父进程消失时子进程自杀
	motionOnly := flag.Bool("motion-only", false, "以运动控制器独立窗口模式启动")
	parentPID := flag.Int("parent-pid", 0, "父进程 PID（仅 motion-only 模式下使用，父进程退出时子进程一并退出）")
	flag.Parse()
	motionOnlyFromEnv := os.Getenv("WINDLABX4_MOTION_ONLY") == "1"
	parentPIDValue := *parentPID
	if envParentPID := os.Getenv("WINDLABX4_PARENT_PID"); envParentPID != "" {
		if pid, err := strconv.Atoi(envParentPID); err == nil {
			parentPIDValue = pid
		}
	}
	isMotionOnly := *motionOnly || motionOnlyFromEnv

	mode := backend.ModeNormal
	if isMotionOnly {
		mode = backend.ModeMotion
	}

	// 实例化 App 并注入父进程 PID（仅 motion-only 子进程需要 watchdog）
	app := backend.NewApp(mode)
	if mode == backend.ModeMotion && parentPIDValue > 0 {
		app.SetParentPID(parentPIDValue)
	}

	// 应用级 ctx，所有后台 goroutine 派生自它，收到信号时统一取消
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	// 启动后台服务（日志、appContext、数据中继、自动连接等）
	// 注意：HTTP server 不在 Start 内启动，由 main.go 自己创建以挂载静态资源
	if err := app.Start(appCtx); err != nil {
		slog.Error("应用启动失败", "component", "main", "error", err)
		os.Exit(1)
	}

	// 监听端口：motion-only 子进程用 8901，避免与主进程 8900 冲突
	listenAddr := "127.0.0.1:8900"
	if isMotionOnly {
		listenAddr = "127.0.0.1:8901"
	}

	// 创建 mux：/api/* 走 API handler，/ 走静态资源
	apiHandler := api.NewRouter(app.NewDeps())
	mux := http.NewServeMux()
	mux.Handle("/api/", apiHandler)

	// 静态资源：frontend/dist 由 embed.FS 嵌入二进制
	// 失败说明构建时未执行 npm run build，直接退出避免运行时 panic
	frontendFS, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		slog.Error("frontend assets unavailable", "component", "main", "error", err)
		os.Exit(1)
	}
	mux.Handle("/", http.FileServer(http.FS(frontendFS)))

	// 端口被占用时终止启动（对齐 master 9eb77d0 修复意图）：
	// 旧版实例占用 127.0.0.1:8900 时，本进程 ListenAndServe 会失败退出，
	// 但 Electron 健康检查可能连到旧实例 API，形成前后端版本错配。
	// 启动前显式探测，被占则立即报错退出（非零退出码触发 Electron 弹窗）。
	if err := probeLocalPort(listenAddr); err != nil {
		slog.Error("本地 API 端口被占用，服务未启动", "component", "main", "addr", listenAddr, "error", err)
		os.Exit(2)
	}

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	// 启动 HTTP server；失败时取消 appCtx 触发优雅关闭
	go func() {
		slog.Info("HTTP server listening", "component", "main", "addr", "http://"+listenAddr, "mode", mode)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "component", "main", "error", err)
			appCancel()
		}
	}()

	// 优雅关闭：等待 SIGINT/SIGTERM 或 appCtx.Done
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sigCh:
		slog.Info("received shutdown signal", "component", "main")
	case <-appCtx.Done():
	}

	// 先 Shutdown HTTP server（拒绝新连接、等待在途请求完成），
	// 再调用 app.Stop 关闭后台服务（数据中继、运动窗口子进程、日志）
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "component", "main", "error", err)
	}
	_ = app.Stop()
}

// probeLocalPort 探测本地 TCP 端口是否已被占用。
// 成功返回 nil（端口可用）；被占用或探测失败返回错误。
// 探测失败（连接被拒绝）视为端口可用——本机无监听时 connect 会立即拒绝。
// 对齐 master 9eb77d0 的同名实现。
func probeLocalPort(addr string) error {
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return nil
	}
	_ = conn.Close()
	return fmt.Errorf("port %s already in use", addr)
}

// writeCrashLog 将启动期 panic 或致命错误写入 crash log 文件。
// 路径优先级：%APPDATA%\windlabx4\crash-YYYYMMDD-HHMMSS.log → exe 同目录 → 当前工作目录。
// 设计意图：windowsgui 子系统下 stderr 不可见，Wails 日志系统可能尚未初始化，
// crash log 是启动失败时唯一的诊断手段。
func writeCrashLog(message string) {
	// 获取调用栈
	buf := make([]byte, 64*1024)
	n := runtime.Stack(buf, false)
	stackTrace := string(buf[:n])

	// 构造 crash 信息
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	content := fmt.Sprintf(
		"WindLabX4 启动崩溃报告\n"+
			"时间: %s\n"+
			"错误: %s\n\n"+
			"调用栈:\n%s\n",
		timestamp, message, stackTrace)

	// 确定日志目录：优先 %APPDATA%\windlabx4，回退到 exe 同目录
	var crashDir string
	if configDir, err := os.UserConfigDir(); err == nil {
		crashDir = filepath.Join(configDir, "windlabx4")
	} else if exePath, err := os.Executable(); err == nil {
		crashDir = filepath.Dir(exePath)
	} else {
		crashDir = "."
	}
	_ = os.MkdirAll(crashDir, 0o755)

	// 写入 crash log（文件名含时间戳，避免覆盖历史 crash 记录）
	crashFile := filepath.Join(crashDir,
		fmt.Sprintf("crash-%s.log", time.Now().Format("20060102-150405")))
	if err := os.WriteFile(crashFile, []byte(content), 0o644); err != nil {
		// 写入失败时只能放弃，避免因日志写入失败再次 panic
		return
	}

	// stderr 兜底输出（windowsgui 下不可见，但 console 模式下可见）
	fmt.Fprintf(os.Stderr, "WindLabX4 启动崩溃: %s\n崩溃日志已写入: %s\n", message, crashFile)
}
