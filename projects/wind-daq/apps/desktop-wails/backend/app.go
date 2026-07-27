package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"golang.org/x/sys/windows/registry"
	"wind-daq/services/api-go/api"
	"wind-daq/services/api-go/pkg/appcontext"
	"wind-daq/services/api-go/pkg/logging"
	"wind-daq/services/api-go/pkg/types"
)

// 启动模式常量
const (
	// ModeNormal 主窗口模式，加载完整仪表盘及所有后台服务
	ModeNormal = "normal"
	// ModeMotion 运动控制器独立窗口模式，仅启动运动相关服务
	ModeMotion = "motion"
)

// VersionInfo 版本信息
type VersionInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// GenericResponse 通用响应结构
//
// Data 字段用于需要返回数据的 binding（如 CalibrationPreviewSevenHole 返回点位预览结果）。
// 简单的成功/失败响应（如 CalibrationStart/Pause/Stop）不填 Data，序列化时 omitempty 省略。
type GenericResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Data    any    `json:"data,omitempty"`
}

type FileResponse struct {
	Success  bool   `json:"success"`
	Filepath string `json:"filepath,omitempty"`
	Error    string `json:"error,omitempty"`
}

// App 是 Wails 应用的主结构体
type App struct {
	ctx        context.Context
	cancel     context.CancelFunc
	appContext *appcontext.AppContext
	apiServer  *http.Server
	relayStop  func()
	logMgr     *logging.Manager
	// mode 启动模式：normal 或 motion，决定 Startup 时加载哪些后台服务
	mode string
	// motionWindowMu 保护 motionWindowCmd，避免重复启动独立窗口进程
	motionWindowMu sync.Mutex
	// motionWindowCmd 已启动的运动控制器独立窗口进程句柄（可能已 Release）
	motionWindowCmd *exec.Cmd
	// shuttingDown 防止窗口销毁后后台协程继续通过 Wails ExecJS 推送事件。
	shuttingDown atomic.Bool
	// parentPID 仅 ModeMotion 子进程使用：父进程消失时本进程自杀，避免成为孤儿
	parentPID int
	// mainWindow 主窗口引用，注册 WindowClosing hook 用以拦截 X 按钮关闭
	mainWindow *application.WebviewWindow
	// userConfirmedExit 用户已确认退出的标志位
	// RegisterExitConfirmationHook 内 hook 检查该标志：
	//   - false → event.Cancel() 阻止默认关闭 listener，并 EmitEvent 通知前端弹确认对话框
	//   - true  → 放行，默认 listener 真正关闭窗口
	// 由 RequestExit binding 在用户确认后置 true，避免 hook 二次触发时再次 Cancel 导致死循环
	userConfirmedExit atomic.Bool
}

// NewApp 创建新的 App 实例
// mode 为启动模式："normal"（主窗口）或 "motion"（运动控制器独立窗口）
func NewApp(mode string) *App {
	if mode != ModeMotion {
		mode = ModeNormal
	}
	return &App{mode: mode}
}

// SetParentPID 仅 ModeMotion 子进程使用：在 Wails Run 之前把父进程 PID 注入。
// Startup 时启动 watchdog 协程，发现父进程不存在则触发自杀，
// 解决任务管理器强杀父进程导致子进程成为孤儿的问题。
func (a *App) SetParentPID(pid int) {
	a.parentPID = pid
}

// RegisterExitConfirmationHook 注册主窗口的 WindowClosing hook，
// 拦截 X 按钮关闭流程：未确认时取消默认关闭并向前端推送确认请求事件。
//
// 设计要点（基于 Wails v3 alpha2.106 事件分发机制）：
//   - 点 X → WM_CLOSE → emit events.Common.WindowClosing → hook 同步先跑 → 默认 listener 后跑
//     （默认 listener 会置 unconditionallyClose=1 并调用 impl.close() 真正销毁窗口）
//   - hook 内必须调用 event.Cancel() 才能阻止默认 listener 执行，否则窗口立刻关闭
//   - hook 内禁止阻塞弹模态对话框（会卡住事件分发），故通过 EmitEvent 异步通知前端
//   - 前端 confirm 后调 RequestExit binding，置 userConfirmedExit=true 并触发 application.Quit()
//     → cleanup() → window.Close() → hook 再次触发，此时 userConfirmedExit=true 放行
//
// 必须在 main.go 中 WailsApp.Run 之前调用，确保 hook 注册先于任何 WindowClosing 事件。
func (a *App) RegisterExitConfirmationHook(win *application.WebviewWindow) {
	a.mainWindow = win
	win.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		// 用户已确认退出 → 放行默认关闭 listener，让窗口真正销毁
		if a.userConfirmedExit.Load() {
			return
		}
		// 应用正在关闭中（如 ServiceShutdown 已触发）→ 放行避免阻塞退出流程
		if a.shuttingDown.Load() {
			return
		}
		// 阻止默认关闭 listener，窗口保持打开
		event.Cancel()
		// 异步推送事件给前端，避免阻塞事件分发 goroutine
		go func() {
			app := application.Get()
			if app == nil {
				return
			}
			app.Event.Emit("app:exit-requested")
		}()
	})
}

// RequestExit 由前端在用户确认退出对话框后调用。
// 置 userConfirmedExit=true 让后续 hook 放行，然后调用 application.Quit() 触发完整退出流程
// （cleanup → ServiceShutdown → 关闭所有窗口 → 最后一个窗口关闭后 PostQuitMessage 退出）。
func (a *App) RequestExit() GenericResponse {
	a.userConfirmedExit.Store(true)
	go func() {
		app := application.Get()
		if app == nil {
			return
		}
		app.Quit()
	}()
	return GenericResponse{Success: true}
}

// ServiceStartup is called by Wails v3 when the bound service starts.
func (a *App) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.shuttingDown.Store(false)

	// 初始化统一日志系统：stderr + 文件 data/logs/wind-daq-YYYYMMDD.log + 内存 ring buffer
	// 必须在所有其他后台服务启动之前调用，确保其后的 slog.Info/Warn/Error 全部被捕获。
	// 日志目录通过 ResolvePath 解析到用户可写目录（%APPDATA%/wind-daq/data/logs），
	// 避免安装目录不可写导致日志丢失。
	rawLogDir := filepath.Join("data", "logs")
	resolvedLogDir, resolveErr := a.ResolvePath(rawLogDir)
	if resolveErr != nil {
		// ResolvePath 失败时回退到原始相对路径，至少保证日志能写
		resolvedLogDir = rawLogDir
		slog.Warn("[app] 日志目录解析失败，使用相对路径", "component", "app", "raw", rawLogDir, "error", resolveErr)
	}
	opts := logging.Default(resolvedLogDir)
	opts.AddSource = false
	mgr, err := logging.Init(opts)
	if err == nil {
		a.logMgr = mgr
		slog.Info("[app] 日志系统已初始化", "component", "app", "logDir", resolvedLogDir, "level", opts.Level.String())
	} else {
		// 此时 slog 尚未挂上文件/ring sink，但 stderr 兜底仍能看到错误
		slog.Error("[app] 日志系统初始化失败", "component", "app", "error", err)
	}

	var initErr error
	a.appContext, initErr = appcontext.NewAppContext("")
	if initErr != nil {
		slog.Error("[app] 服务初始化错误", "component", "app", "error", initErr)
		if app := application.Get(); app != nil {
			app.Dialog.Error().
				SetTitle("初始化错误").
				SetMessage(fmt.Sprintf("服务初始化失败: %v", initErr)).
				Show()
		}
		return nil
	}

	// 运动控制器独立窗口进程：仅启动运动状态轮询，避免与主窗口进程冲突
	// （API 服务器端口、数据中继、硬件采集由主窗口进程负责）
	if a.mode == ModeMotion {
		// Motion-only 窗口只是主进程 MotionManager 的操作面板；
		// 所有运动请求经本地 HTTP 转发到主进程，避免子进程重复连接真实控制器。
		// 启动父进程看护：父进程消失时本进程自杀，避免任务管理器强杀父进程后留下孤儿。
		if a.parentPID > 0 {
			a.startParentWatchdog()
		}
		slog.Info("[app] 运动控制器独立窗口已初始化（仅运动服务）", "component", "app")
		return nil
	}

	// 主窗口进程：启动全部后台服务
	// 注意：不再启动后端 MotionStatusPoller。
	// 该 poller 每 100ms 调一次 StatusAll 但输出被直接丢弃，纯属冗余开销——
	// MotionManager 并不缓存状态（StatusAll 每次直查硬件），遍历/校准用例也是
	// 直接调 StatusAll，不依赖该 poller。与 motion-controller 项目保持一致：
	// 前端 HTTP 轮询是唯一的状态消费者，避免与前端争抢同一把硬件连接锁。
	a.startDataRelay()
	a.startLocalAPIServer()
	// 主进程启动后，后台异步连接所有标记为 AutoConnect 的位移机构，
	// 避免用户必须先打开运动控制面板才能触发连接。
	a.startMotionAutoConnect()
	// 后台异步连接所有标记为 AutoConnect 的 DAQ 设备（含模拟设备）。
	// 采集必须由用户显式点击"开始采集"触发。
	a.startDeviceAutoConnect()

	slog.Info("[app] Wind-DAQ 应用已成功初始化", "component", "app")
	return nil
}

func (a *App) startLocalAPIServer() {
	if a.appContext == nil {
		return
	}
	ring := func() *logging.RingBuffer {
		if a.logMgr != nil {
			return a.logMgr.Ring()
		}
		return nil
	}()
	a.apiServer = &http.Server{
		Addr: "127.0.0.1:8900",
		Handler: api.NewRouter(api.Deps{
			DeviceManager:      a.appContext.DeviceManager,
			AcquisitionHub:     a.appContext.AcquisitionHub,
			ReportManager:      a.appContext.ReportManager,
			MotionManager:      a.appContext.MotionManager,
			MotionService:      a.appContext.MotionManagerRaw,
			CalibrationManager: a.appContext.CalibrationMgr,
			TraversalManager:   a.appContext.TraversalMgr,
			StorageRecorder:    a.appContext.StorageRecorder,
			ConfigManager:      a.appContext.ConfigManager,
			LogRing:            ring,
			LogManager:         a.logMgr,
		}),
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}
	go func() {
		slog.Info("[app] local API 服务器启动", "component", "app", "addr", "http://127.0.0.1:8900")
		if err := a.apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("[app] local API 服务器异常退出", "component", "app", "error", err)
		}
	}()
}

// startDataRelay 启动采集数据中继。
//
// 设计说明（2026-06-29 修复）：
//   - 历史实现通过 app.Event.Emit("daq:payload", payload) 把采集数据推送到前端，
//     但 Wails v3 的 Emit 内部会通过 InvokeSync 在 GUI 主线程同步执行 WebView2
//     ExecuteScript。AcquisitionHub 默认 20Hz 节流，每秒 20 次同步主线程 JS 调用
//     会让 WebView2 返回 EINVAL ("[WebView2] Eval failed: invalid argument")，
//     同时阻塞 GUI 主线程，导致 startAcquisition / DeviceSubscribeStream 等
//     Wails binding 调用延迟甚至失败，前端表现为"开始采集后 UI 无数据更新"。
//   - 修复策略：前端改为按全局刷新频率 HTTP 轮询 /api/daq/latest/{id} 拿最新数据。
//     轮询间隔由前端 deviceApi.setPublishRate/getPublishRate 同步的 Hz 决定；
//     AcquisitionHub.OnData 始终更新 latestByDevice，不受 publishHz 节流影响。
//     后端这里仅 drain relay.Payloads() 通道，避免 relay goroutine 因通道满而
//     反压 AcquisitionHub；不再调用 app.Event.Emit，彻底消除主线程同步 JS 调用。
//   - 保留 DataStreamRelay 与 DeviceSubscribeStream binding 是为了不破坏前端
//     subscribeStream 调用契约（前端仍会调一次 subscribe 来标记订阅意图，
//     后端可据此做未来扩展，例如 SSE 推送到 Web 客户端）。
func (a *App) startDataRelay() {
	relay := a.appContext.DataStreamRelay
	if relay == nil {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.relayStop = cancel
	go func() {
		defer relay.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-relay.Payloads():
				if !ok {
					return
				}
				if a.shuttingDown.Load() {
					return
				}
				// 仅 drain payload，不再 Emit；前端通过 HTTP 轮询拿数据。
			}
		}
	}()
}

// startMotionAutoConnect 在后台异步连接所有 AutoConnect=true 的位移机构。
//
// 设计要点：
//   - 必须异步执行：底层 TCP 连接（B140 等真实硬件）可能耗时数百毫秒至几秒，
//     若同步阻塞 Startup，Wails GUI 主线程会卡住导致窗口长时间不出现。
//   - 单次失败不影响其他控制器：用 Promise.allSettled 风格的容错策略，
//     某个控制器拨号失败时仅记录日志，不阻塞其它控制器的连接尝试。
//   - 即使连接失败，控制器仍会出现在 StatusAll 列表中（Connected=false + LastError），
//     这是 MotionManager.Connect → getController 的既有行为，不需要额外处理。
//   - 调用前后均通过 ctx 检查关停信号，避免在应用即将退出时仍发起新连接。
func (a *App) startMotionAutoConnect() {
	if a.appContext == nil || a.appContext.MotionManagerRaw == nil {
		return
	}
	mgr := a.appContext.MotionManagerRaw

	go func() {
		// 让出当前调度并稍等 profileStore 完成首次加载（保险，
		// LoadProfiles 内部已自带同步，但延后 100ms 也能避免与 Startup 其它 goroutine 抢锁）
		select {
		case <-a.ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}

		profiles, err := mgr.LoadProfiles()
		if err != nil {
			slog.Warn("[app] 加载运动控制器配置失败", "component", "app", "error", err)
			return
		}

		// 仅挑选标记为 AutoConnect 的 profile，逐个并发连接
		var wg sync.WaitGroup
		for _, p := range profiles {
			if !p.AutoConnect {
				continue
			}
			wg.Add(1)
			go func(id, name string) {
				defer wg.Done()
				if a.ctx.Err() != nil {
					return
				}
				slog.Info("正在连接位移机构", "component", "motion-auto", "id", id, "name", name)
				if err := mgr.Connect(a.ctx, id); err != nil {
					// 连接失败不影响其它控制器；前端通过 StatusAll 的 LastError 字段感知
					slog.Warn("位移机构自动连接失败", "component", "motion-auto", "id", id, "name", name, "error", err)
					return
				}
				slog.Info("位移机构已成功连接", "component", "motion-auto", "id", id, "name", name)
			}(p.ID, p.Name)
		}
		wg.Wait()
	}()
}

// startDeviceAutoConnect 启动后自动连接所有标记为 AutoConnect 的 DAQ 设备。
// 该流程只负责连接设备，不自动开始采集。
func (a *App) startDeviceAutoConnect() {
	if a.appContext == nil || a.appContext.DeviceManager == nil {
		return
	}
	mgr := a.appContext.DeviceManager
	hub := a.appContext.AcquisitionHub

	go func() {
		select {
		case <-a.ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}

		profiles := mgr.GetProfiles()
		var wg sync.WaitGroup
		for _, p := range profiles {
			if !p.AutoConnect {
				continue
			}
			wg.Add(1)
			go func(id, name string) {
				defer wg.Done()
				if a.ctx.Err() != nil {
					return
				}
				slog.Info("正在连接采集设备", "component", "device-auto", "id", id, "name", name)
				if err := mgr.Connect(id); err != nil {
					slog.Warn("采集设备自动连接失败", "component", "device-auto", "id", id, "name", name, "error", err)
					return
				}
				slog.Info("采集设备已成功连接", "component", "device-auto", "id", id, "name", name)
			}(p.ID, p.Name)
		}
		wg.Wait()

		// 连接成功后稍等数据流入 AcquisitionHub，再尝试恢复之前已订阅的数据流
		if a.ctx.Err() != nil {
			return
		}
		select {
		case <-a.ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
		if hub != nil {
			for _, p := range profiles {
				if p.AutoConnect && p.Type == types.DeviceTypeSimulated {
					slog.Info("设备已连接，等待用户开始采集", "component", "device-auto", "name", p.Name)
				}
			}
		}
	}()
}

// DeviceSubscribeStream 前端调用此方法来订阅/取消订阅某个设备的采集数据流
func (a *App) DeviceSubscribeStream(deviceID string, subscribe bool) GenericResponse {
	if a.appContext == nil {
		return GenericResponse{Success: false, Error: "数据流中继未初始化"}
	}
	relay := a.appContext.DataStreamRelay
	if relay == nil {
		return GenericResponse{Success: false, Error: "数据流中继未初始化"}
	}
	if subscribe {
		relay.Subscribe(deviceID)
	} else {
		relay.Unsubscribe(deviceID)
	}
	return GenericResponse{Success: true}
}

// ServiceShutdown is called by Wails v3 when the bound service shuts down.
func (a *App) ServiceShutdown() error {
	a.shuttingDown.Store(true)

	if a.relayStop != nil {
		a.relayStop()
	}

	// 同步停止 DataStreamRelay 并等待所有转发 worker 完成 hub 退订（O-3），
	// 避免进程退出时 relay goroutine 仍持有 hub 订阅。
	// Stop 幂等且并发安全：startDataRelay 的 drain goroutine 退出时
	// defer relay.Stop() 会共享同一次终止，不会重复 cancel。
	if a.appContext != nil && a.appContext.DataStreamRelay != nil {
		a.appContext.DataStreamRelay.Stop()
	}

	// 先关闭子窗口进程：保证父进程退出时不会留下孤儿运动控制器窗口。
	// 顺序放在最前是因为子进程仍可能向父进程的日志/HTTP 写数据，先停子再停父更稳。
	a.terminateMotionWindow()

	// 停止校准任务并有界等待其 session 退出（writer flush/结果保存/运动归零完成），
	// 避免进程退出时校准 worker 仍在写文件或驱动运动轴。
	// 必须在 logMgr.Close 之前完成：校准 finalize 期间的日志需要能落盘。
	if a.appContext != nil && a.appContext.CalibrationMgr != nil {
		if err := a.appContext.CalibrationMgr.Shutdown(); err != nil {
			slog.Warn("[app] 校准任务停止等待超时，后台仍将继续收尾", "component", "app", "error", err)
		}
	}

	if a.cancel != nil {
		a.cancel()
	}
	if a.apiServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = a.apiServer.Shutdown(shutdownCtx)
	}
	// 在 logMgr.Close 之前打这条收尾日志，确保它能写入文件/ring sink。
	slog.Info("Wind-DAQ 应用已关闭", "component", "app")
	if a.logMgr != nil {
		_ = a.logMgr.Close()
	}
	return nil
}

// terminateMotionWindow 关闭运动控制器独立窗口子进程（如已启动）。
// 子进程是 Wails GUI 进程，Windows 下没有可用的 SIGTERM 软关停信号，
// 因此直接 Kill；子进程内部的 wails Shutdown 会被中断，但运动控制器
// 没有需要持久化的中途状态，可以接受。
//
// 调用方：仅父进程的 Shutdown。子进程自身退出时，由 OpenMotionWindow
// 内部的 Wait goroutine 清理 motionWindowCmd 引用，无需在此处理。
func (a *App) terminateMotionWindow() {
	a.motionWindowMu.Lock()
	cmd := a.motionWindowCmd
	a.motionWindowMu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return
	}
	// 已退出则跳过
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		return
	}

	pid := cmd.Process.Pid
	if err := cmd.Process.Kill(); err != nil {
		// 子进程可能在我们读句柄到 Kill 之间自然退出，此时 Kill 会报错；非致命。
		slog.Warn("kill motion window failed", "component", "motion-window", "pid", pid, "error", err)
		return
	}
	slog.Info("已关闭运动控制器独立窗口子进程", "component", "motion-window", "pid", pid)
}

// GetVersion 获取版本信息
func (a *App) GetVersion() VersionInfo {
	return VersionInfo{
		Name:    "Wind-DAQ",
		Version: "1.0.0",
	}
}

// GetStartupMode 获取当前应用启动模式
// 返回 "normal"（主窗口）或 "motion"（运动控制器独立窗口）
func (a *App) GetStartupMode() string {
	return a.mode
}

// GetInstallerLanguage 读取安装程序写入的语言偏好
// 安装时在 NSIS 中写入 HKCU\Software\<Company>\<Product>\InstallerLanguage
// 返回 "zh"、"en" 或空字符串（未由安装程序设置时）
func (a *App) GetInstallerLanguage() string {
	keyPath := fmt.Sprintf(`Software\%s\%s`, "wind-daq", "wind-daq")
	k, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	val, _, err := k.GetStringValue("InstallerLanguage")
	if err != nil {
		return ""
	}
	return val
}

// OpenMotionWindow 启动运动控制器独立窗口（独立进程）
// 通过重新启动当前可执行文件并传入 --motion-only 参数实现真正的独立窗口
// 使用互斥锁防止重复启动
func (a *App) OpenMotionWindow() GenericResponse {
	a.motionWindowMu.Lock()
	defer a.motionWindowMu.Unlock()

	// 检查已有进程是否仍在运行（非阻塞探测）
	if a.motionWindowCmd != nil && a.motionWindowCmd.Process != nil {
		// 若进程已退出则清理句柄，否则提示用户窗口已打开
		if a.motionWindowCmd.ProcessState != nil && a.motionWindowCmd.ProcessState.Exited() {
			a.motionWindowCmd = nil
		} else {
			return GenericResponse{
				Success: false,
				Error:   "运动控制器独立窗口已打开，请勿重复启动",
			}
		}
	}

	exePath, err := os.Executable()
	if err != nil {
		slog.Error("获取可执行文件路径失败", "component", "motion-window", "error", err)
		return GenericResponse{Success: false, Error: fmt.Sprintf("获取可执行文件路径失败: %v", err)}
	}

	// 启动独立进程，通过环境变量传递 motion-only 模式；同时把父进程 PID 传过去，
	// 子进程的 watchdog 在父进程消失时触发自杀，避免任务管理器强杀父进程留下孤儿。
	//
	// 注意：这里禁止注入 Wails 开发服务器相关环境变量 (devserver / frontenddevserverurl)。
	// 历史实现曾尝试为子进程注入这些变量，但生产构建的可执行文件并未启动 15173 dev server，
	// 子进程被误导成"开发模式"后会去连不存在的 dev server，结果直接白屏或启动失败。
	// 子进程应当沿用与父进程一致的资源加载方式（生产嵌入式资源 / 父进程 dev server），
	// 由 Wails 内部根据自身构建模式自动决定。
	childEnv := append(os.Environ(), "WIND_DAQ_MOTION_ONLY=1", fmt.Sprintf("WIND_DAQ_PARENT_PID=%d", os.Getpid()))
	cmd := exec.Command(exePath)
	cmd.Env = childEnv
	// 独立进程继承标准输出和错误，便于调试
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// 不等待子进程，解耦父子进程生命周期
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		slog.Error("启动运动控制器独立窗口失败", "component", "motion-window", "error", err)
		return GenericResponse{Success: false, Error: fmt.Sprintf("启动独立窗口失败: %v", err)}
	}

	slog.Info("运动控制器独立窗口已启动", "component", "motion-window", "pid", cmd.Process.Pid)
	a.motionWindowCmd = cmd

	// 后台监控子进程退出，退出后清理句柄以允许再次启动
	go func(c *exec.Cmd) {
		_ = c.Wait()
		a.motionWindowMu.Lock()
		if a.motionWindowCmd == c {
			a.motionWindowCmd = nil
		}
		a.motionWindowMu.Unlock()
		slog.Info("运动控制器独立窗口进程已退出", "component", "motion-window")
	}(cmd)

	return GenericResponse{Success: true}
}

// ResolvePath 将相对路径解析到用户可写的应用目录，避免安装目录不可写。
func (a *App) ResolvePath(p string) (string, error) {
	if p == "" {
		return "", nil
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p), nil
	}
	baseDir, err := writableUserConfigDir()
	if err == nil {
		return filepath.Join(baseDir, p), nil
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p, nil
	}
	return abs, nil
}

// writableUserConfigDir 返回用户可写的应用配置目录（Windows 下为 %APPDATA%\wind-daq）。
// 命名对齐 os.UserConfigDir() 语义：既是配置目录也是数据目录基础。
func writableUserConfigDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "wind-daq"), nil
}

// PickDirectory 选择目录对话框
func (a *App) PickDirectory() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("应用上下文未初始化")
	}
	app := application.Get()
	if app == nil {
		return "", fmt.Errorf("Wails 应用未初始化")
	}
	opts := application.OpenFileDialogOptions{
		Title:                "选择保存目录",
		CanCreateDirectories: true,
		CanChooseDirectories: true,
		CanChooseFiles:       false,
	}
	return app.Dialog.OpenFileWithOptions(&opts).PromptForSingleSelection()
}

// PickSaveFile 保存文件对话框，返回用户选择的完整文件路径。
// title 为对话框标题，defaultFilename 为默认文件名，filters 为文件扩展名过滤。
func (a *App) PickSaveFile(title string, defaultFilename string, filters []application.FileFilter) (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("应用上下文未初始化")
	}
	app := application.Get()
	if app == nil {
		return "", fmt.Errorf("Wails 应用未初始化")
	}
	opts := application.SaveFileDialogOptions{
		Title:    title,
		Filename: defaultFilename,
		Filters:  filters,
	}
	return app.Dialog.SaveFileWithOptions(&opts).PromptForSingleSelection()
}

func (a *App) PickFile(title string, filters []application.FileFilter) (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("应用上下文未初始化")
	}
	app := application.Get()
	if app == nil {
		return "", fmt.Errorf("Wails 应用未初始化")
	}
	return app.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{Title: title, Filters: filters}).PromptForSingleSelection()
}

func (a *App) PickFiles(title string, filters []application.FileFilter) ([]string, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("应用上下文未初始化")
	}
	app := application.Get()
	if app == nil {
		return nil, fmt.Errorf("Wails 应用未初始化")
	}
	return app.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{Title: title, Filters: filters}).PromptForMultipleSelection()
}

// FileExists 检查指定路径的文件是否存在。
// 用于校准 Start 前检测 CSV 文件是否已存在，提示用户决定是否覆盖。
// 路径不存在或指向目录时返回 false，不报错。
func (a *App) FileExists(path string) (bool, error) {
	if path == "" {
		return false, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return !info.IsDir(), nil
}

// RemoveFile 删除指定路径的文件。
// 用于校准 Start 前用户选择"覆盖"时清理旧 CSV 文件，
// 让后续追加模式 writer 当作新文件写入（BOM + 表头）。
// 路径不存在视为已删除，不报错。
func (a *App) RemoveFile(path string) (bool, error) {
	if path == "" {
		return false, nil
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}
	return true, nil
}

// callMgr 通用 manager 方法调用辅助
func (a *App) callMgr(mgr any, name string, fn func() error) GenericResponse {
	if a == nil || a.appContext == nil || mgr == nil {
		slog.Warn("callMgr: manager 未初始化", "component", "app", "manager", name)
		return GenericResponse{Success: false, Error: name + "未初始化"}
	}
	if err := fn(); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

func (a *App) deviceManager() any {
	if a == nil || a.appContext == nil {
		return nil
	}
	return a.appContext.DeviceManager
}

func (a *App) acquisitionHub() any {
	if a == nil || a.appContext == nil {
		return nil
	}
	return a.appContext.AcquisitionHub
}

func (a *App) motionManager() any {
	if a == nil || a.appContext == nil {
		return nil
	}
	return a.appContext.MotionManager
}

func (a *App) calibrationManager() any {
	if a == nil || a.appContext == nil {
		return nil
	}
	return a.appContext.CalibrationMgr
}

func (a *App) storageRecorder() any {
	if a == nil || a.appContext == nil {
		return nil
	}
	return a.appContext.StorageRecorder
}

func (a *App) configManager() any {
	if a == nil || a.appContext == nil {
		return nil
	}
	return a.appContext.ConfigManager
}

// ==================== 设备管理 API ====================

func (a *App) DeviceGetProfiles() []types.DeviceProfile {
	if a == nil || a.appContext == nil || a.appContext.DeviceManager == nil {
		return nil
	}
	return a.appContext.DeviceManager.GetProfiles()
}

func (a *App) DeviceUpsertProfile(profile types.DeviceProfile) GenericResponse {
	return a.callMgr(a.deviceManager(), "设备管理器", func() error {
		return a.appContext.DeviceManager.UpsertProfile(profile)
	})
}

func (a *App) DeviceDeleteProfile(id string) GenericResponse {
	return a.callMgr(a.deviceManager(), "设备管理器", func() error {
		return a.appContext.DeviceManager.DeleteProfile(id)
	})
}

func (a *App) DeviceScanDevices() ([]types.DeviceScanResult, error) {
	if a == nil || a.appContext == nil || a.appContext.DeviceManager == nil {
		return nil, fmt.Errorf("设备管理器未初始化")
	}
	return a.appContext.DeviceManager.ScanDevices()
}

func (a *App) DeviceConnect(id string) GenericResponse {
	return a.callMgr(a.deviceManager(), "设备管理器", func() error {
		return a.appContext.DeviceManager.Connect(id)
	})
}

func (a *App) DeviceDisconnect(id string) GenericResponse {
	return a.callMgr(a.deviceManager(), "设备管理器", func() error {
		return a.appContext.DeviceManager.Disconnect(id)
	})
}

// DeviceStartAcquisition 启动指定设备的采集。
// 采集启动成功后异步触发 autoStart 检查：若 storage-settings 中 autoStartOnAcquisition=true
// 且当前未在录制，则自动开始录制。失败仅记录日志，不阻塞采集响应。
func (a *App) DeviceStartAcquisition(id string) GenericResponse {
	resp := a.callMgr(a.deviceManager(), "设备管理器", func() error {
		return a.appContext.DeviceManager.StartAcquisition(id)
	})
	if resp.Success {
		go a.tryAutoStartRecording()
	}
	return resp
}

// storageSettingsSchema 镜像前端 StorageSettings 的 JSON schema，
// 用于从 storage-settings 配置键读取业务级录制配置。
// 注意：BaseDirectory 对应 RecordingConfig.OutputDir；
// WaveformBufferSize 是 UI-only 字段，后端忽略。
type storageSettingsSchema struct {
	BaseDirectory          string `json:"baseDirectory"`
	FilePrefix             string `json:"filePrefix"`
	AutoStartOnAcquisition bool   `json:"autoStartOnAcquisition"`
	StopConditions         struct {
		MaxDurationMs    int64 `json:"maxDurationMs,omitempty"`
		MaxFileSizeBytes int64 `json:"maxFileSizeBytes,omitempty"`
		MaxRecordCount   int64 `json:"maxRecordCount,omitempty"`
	} `json:"stopConditions"`
	FileRotation struct {
		Enabled          bool  `json:"enabled"`
		MaxFileSizeBytes int64 `json:"maxFileSizeBytes"`
		MaxDurationMs    int64 `json:"maxDurationMs"`
	} `json:"fileRotation"`
}

// tryAutoStartRecording 读取 storage-settings 配置，
// 若 autoStartOnAcquisition=true 且当前未在录制，则自动开始录制。
// 失败仅记录日志，不阻塞采集流程；异步调用，不阻塞 DeviceStartAcquisition 响应。
func (a *App) tryAutoStartRecording() {
	if a == nil || a.appContext == nil ||
		a.appContext.ConfigManager == nil || a.appContext.StorageRecorder == nil {
		return
	}
	// 已经在录制则跳过（如手动已开始录制）
	if a.appContext.StorageRecorder.Status().Recording {
		return
	}
	data, err := a.appContext.ConfigManager.LoadConfig("storage-settings")
	if err != nil {
		slog.Warn("[app] autoStart: 读取 storage-settings 失败",
			"component", "app", "error", err)
		return
	}
	if len(data) == 0 {
		return
	}
	var settings storageSettingsSchema
	if err := json.Unmarshal(data, &settings); err != nil {
		slog.Warn("[app] autoStart: 解析 storage-settings 失败",
			"component", "app", "error", err)
		return
	}
	if !settings.AutoStartOnAcquisition {
		return
	}
	if settings.BaseDirectory == "" || settings.FilePrefix == "" {
		slog.Warn("[app] autoStart: baseDirectory 或 filePrefix 为空，跳过自动录制",
			"component", "app")
		return
	}
	// 解析路径（与 StorageStartRecording 保持一致，统一在后端解析）
	resolved, err := a.ResolvePath(settings.BaseDirectory)
	if err != nil {
		slog.Warn("[app] autoStart: 解析路径失败",
			"component", "app", "path", settings.BaseDirectory, "error", err)
		return
	}
	// 构造 RecordingConfig：字段类型通过别名间接访问 storage.StopConditions/FileRotation
	config := types.StorageRecordingConfig{
		OutputDir:              resolved,
		FilePrefix:             settings.FilePrefix,
		AutoStartOnAcquisition: true,
	}
	config.StopConditions.MaxDurationMs = settings.StopConditions.MaxDurationMs
	config.StopConditions.MaxFileSizeBytes = settings.StopConditions.MaxFileSizeBytes
	config.StopConditions.MaxRecordCount = settings.StopConditions.MaxRecordCount
	config.FileRotation.Enabled = settings.FileRotation.Enabled
	config.FileRotation.MaxFileSizeBytes = settings.FileRotation.MaxFileSizeBytes
	config.FileRotation.MaxDurationMs = settings.FileRotation.MaxDurationMs

	if err := a.appContext.StorageRecorder.Start(config); err != nil {
		slog.Warn("[app] autoStart: 启动录制失败",
			"component", "app", "error", err)
	} else {
		slog.Info("[app] autoStart: 已自动开始录制",
			"component", "app", "outputDir", resolved, "filePrefix", settings.FilePrefix)
	}
}

func (a *App) DeviceStopAcquisition(id string) GenericResponse {
	return a.callMgr(a.deviceManager(), "设备管理器", func() error {
		return a.appContext.DeviceManager.StopAcquisition(id)
	})
}

func (a *App) DeviceGetStatus(id string) (types.DeviceStatus, bool) {
	if a == nil || a.appContext == nil || a.appContext.DeviceManager == nil {
		return types.DeviceStatus{}, false
	}
	return a.appContext.DeviceManager.GetStatus(id)
}

func (a *App) DeviceGetLatestData(deviceID string) (types.DeviceDataPayload, bool) {
	if a == nil || a.appContext == nil || a.appContext.AcquisitionHub == nil {
		return types.DeviceDataPayload{}, false
	}
	return a.appContext.AcquisitionHub.GetLatestData(deviceID)
}

func (a *App) DeviceSetPublishRate(hz float64) GenericResponse {
	return a.callMgr(a.acquisitionHub(), "采集中心", func() error {
		return a.appContext.AcquisitionHub.SetPublishRate(hz)
	})
}

func (a *App) DeviceGetPublishRate() float64 {
	if a == nil || a.appContext == nil || a.appContext.AcquisitionHub == nil {
		return 0
	}
	return a.appContext.AcquisitionHub.PublishRate()
}

// ==================== 运动控制 API ====================

func (a *App) MotionGetProfiles() string {
	if a == nil || a.appContext == nil || a.appContext.MotionManager == nil {
		return "[]"
	}
	profiles, err := a.appContext.MotionManager.LoadProfiles()
	if err != nil {
		return "[]"
	}
	data, _ := json.Marshal(profiles)
	return string(data)
}

func (a *App) MotionUpsertProfile(profile types.MotionControllerProfile) GenericResponse {
	return a.callMgr(a.motionManager(), "运动管理器", func() error {
		return a.appContext.MotionManager.UpsertProfile(profile)
	})
}

func (a *App) MotionDeleteProfile(id string) GenericResponse {
	return a.callMgr(a.motionManager(), "运动管理器", func() error {
		return a.appContext.MotionManager.DeleteProfile(id)
	})
}

func (a *App) MotionGetStatus() string {
	if a == nil || a.appContext == nil || a.appContext.MotionManager == nil {
		return "[]"
	}
	statuses := a.appContext.MotionManager.StatusAll(a.ctx)
	data, _ := json.Marshal(statuses)
	return string(data)
}

func (a *App) MotionConnect(id string) GenericResponse {
	return a.callMgr(a.motionManager(), "运动管理器", func() error {
		return a.appContext.MotionManager.Connect(a.ctx, id)
	})
}

func (a *App) MotionDisconnect(id string) GenericResponse {
	return a.callMgr(a.motionManager(), "运动管理器", func() error {
		return a.appContext.MotionManager.Disconnect(a.ctx, id)
	})
}

func (a *App) MotionHome(id string, axis string) GenericResponse {
	return a.callMgr(a.motionManager(), "运动管理器", func() error {
		return a.appContext.MotionManager.Home(a.ctx, id, types.MotionAxisName(axis))
	})
}

func (a *App) MotionStop(id string, axis string) GenericResponse {
	return a.callMgr(a.motionManager(), "运动管理器", func() error {
		var axisName types.MotionAxisName
		if axis != "" {
			axisName = types.MotionAxisName(axis)
		}
		return a.appContext.MotionManager.Stop(a.ctx, id, axisName)
	})
}

func (a *App) MotionEmergencyStop(id string) GenericResponse {
	return a.callMgr(a.motionManager(), "运动管理器", func() error {
		return a.appContext.MotionManager.EmergencyStop(a.ctx, id)
	})
}

func (a *App) MotionMoveTo(id string, axis string, position float64) GenericResponse {
	return a.callMgr(a.motionManager(), "运动管理器", func() error {
		return a.appContext.MotionManager.MoveTo(a.ctx, id, types.MotionAxisName(axis), position)
	})
}

func (a *App) MotionMoveBy(id string, axis string, delta float64) GenericResponse {
	return a.callMgr(a.motionManager(), "运动管理器", func() error {
		return a.appContext.MotionManager.MoveBy(a.ctx, id, types.MotionAxisName(axis), delta)
	})
}

func (a *App) MotionJog(id string, axis string, velocity float64) GenericResponse {
	return a.callMgr(a.motionManager(), "运动管理器", func() error {
		return a.appContext.MotionManager.Jog(a.ctx, id, types.MotionAxisName(axis), velocity)
	})
}

func (a *App) MotionDefinePosition(id string, axis string, position float64) GenericResponse {
	return a.callMgr(a.motionManager(), "运动管理器", func() error {
		return a.appContext.MotionManager.DefinePosition(a.ctx, id, types.MotionAxisName(axis), position)
	})
}

func (a *App) MotionResetEmergencyStop(id string) GenericResponse {
	return a.callMgr(a.motionManager(), "运动管理器", func() error {
		return a.appContext.MotionManager.ResetEmergencyStop(a.ctx, id)
	})
}

// ==================== 校准 API ====================

// CalibrationStart 启动校准任务。
//
// 参数使用 pkg/types 暴露的 CalibrationConfigDTO（adapters/config 层 DTO 的公共别名），
// 而非直接用 core 的 calibration.Config。原因：Wails v3 运行时用 encoding/json 把前端
// JS 对象反序列化进方法参数，而前端发送的探针通道是嵌套 channel 格式，core 层禁止自带
// UnmarshalJSON（零容忍约束）。DTO 用普通 struct tag 同时接收扁平与嵌套两种 shape，
// ToCore 再转换为 core 层的 calibration.Config。
// backend 是独立 Go module，不能直接 import internal/adapters/config（Go internal 规则），
// 故通过 pkg/types 的类型别名 facade 访问 DTO。
func (a *App) CalibrationStart(dto types.CalibrationConfigDTO) GenericResponse {
	config := dto.ToCore()
	return a.callMgr(a.calibrationManager(), "校准管理器", func() error {
		// 路径归一：相对 savePath 解析到 %APPDATA%\wind-daq\<相对>，
		// 与 StorageStartRecording 范式一致，避免依赖工作目录。
		if config.SavePath != "" {
			resolved, err := a.ResolvePath(config.SavePath)
			if err != nil {
				return err
			}
			config.SavePath = resolved
		}
		return a.appContext.CalibrationMgr.Start(config)
	})
}

// CalibrationPreviewSevenHole 七孔点位预览（spec Task 13）
//
// 接收前端"配置向导"提交的 SevenHoleConfigDTO（α/β/θ/φ 范围与步长），
// 调用 CalibrationManager.PreviewSevenHolePoints 生成完整点位列表 + 内/外区聚合统计，
// 返回 SevenHolePreviewResult 供前端实时显示总点数（如 673 点 = 169 内区 + 504 外区）。
//
// 与 CalibrationStart 的区别：
//   - 纯计算，不启动采集、不创建 CSV writer、不创建 runtime
//   - 不需要路径归一（无 SavePath）
//   - 不需要 ToCore 转换（SevenHoleConfigDTO 直接是 calibration.SevenHoleConfig 别名）
//
// 错误处理：配置非法（步长 ≤ 0、范围 min > max）返回 Success=false + Error 透传 GenerateSevenHolePoints 错误。
// 注意：无法复用 callMgr（它只返回成功/失败不带 Data），这里手写响应构造。
func (a *App) CalibrationPreviewSevenHole(dto types.SevenHoleConfigDTO) GenericResponse {
	if a == nil || a.appContext == nil || a.appContext.CalibrationMgr == nil {
		return GenericResponse{Success: false, Error: "校准管理器未初始化"}
	}
	result, err := a.appContext.CalibrationMgr.PreviewSevenHolePoints(dto)
	if err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true, Data: result}
}

// CalibrationPreviewFiveHole 五孔点位预览（spec Task 11）
//
// 接收前端"配置向导"提交的 FiveHolePointLayoutDTO（α/β 范围与步长 + serpentine 开关），
// 调用 CalibrationManager.PreviewFiveHolePoints 生成蛇形/raster 点位列表，
// 返回 []FiveHoleSnakePoint（bare array，与 HTTP /api/calibration/fivehole 契约一致）
// 经 GenericResponse.Data 包装供 Wails binding 透传。
//
// 与 CalibrationPreviewSevenHole 的区别：
//   - 五孔返回 bare array（Data 字段是 []FiveHoleSnakePoint），前端直接迭代
//   - 七孔返回包装对象（Data 字段是 SevenHolePreviewResult，含 totalCount/innerCount/outerCount）
//
// 与 CalibrationStart 的区别：
//   - 纯计算，不启动采集、不创建 CSV writer、不创建 runtime
//   - 不需要路径归一（无 SavePath）
//   - 不需要 ToCore 转换（FiveHolePointLayoutDTO 直接是 calibration.FiveHolePointLayout 别名）
//
// 错误处理：配置非法（步长 ≤ 0）返回 Success=false + Error 透传 GenerateFiveHoleSnakePoints 错误。
// spec Task 11 acceptance：HTTP/Wails 都调用同一 usecase，后端错误传到 UI，不静默 fallback。
func (a *App) CalibrationPreviewFiveHole(dto types.FiveHolePointLayoutDTO) GenericResponse {
	if a == nil || a.appContext == nil || a.appContext.CalibrationMgr == nil {
		return GenericResponse{Success: false, Error: "校准管理器未初始化"}
	}
	points, err := a.appContext.CalibrationMgr.PreviewFiveHolePoints(dto)
	if err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true, Data: points}
}

func (a *App) CalibrationStatus() types.CalibrationStatus {
	if a == nil || a.appContext == nil || a.appContext.CalibrationMgr == nil {
		return types.CalibrationStatus{}
	}
	return a.appContext.CalibrationMgr.Status()
}

func (a *App) CalibrationCollect() GenericResponse {
	return a.callMgr(a.calibrationManager(), "校准管理器", func() error {
		return a.appContext.CalibrationMgr.CollectCurrentPoint()
	})
}

func (a *App) CalibrationPause() GenericResponse {
	return a.callMgr(a.calibrationManager(), "校准管理器", func() error {
		return a.appContext.CalibrationMgr.Pause()
	})
}

func (a *App) CalibrationResume() GenericResponse {
	return a.callMgr(a.calibrationManager(), "校准管理器", func() error {
		return a.appContext.CalibrationMgr.Resume()
	})
}

func (a *App) CalibrationStop() GenericResponse {
	return a.callMgr(a.calibrationManager(), "校准管理器", func() error {
		return a.appContext.CalibrationMgr.Stop()
	})
}

func (a *App) CalibrationGetResult(taskID string) (types.CalibrationStatus, bool) {
	if a == nil || a.appContext == nil || a.appContext.CalibrationMgr == nil {
		return types.CalibrationStatus{}, false
	}
	return a.appContext.CalibrationMgr.GetResult(taskID)
}

func (a *App) CalibrationSaveCsv(taskID string, savePath string) FileResponse {
	if a == nil || a.appContext == nil || a.appContext.CalibrationMgr == nil {
		return FileResponse{Success: false, Error: "校准管理器未初始化"}
	}
	// 路径归一：相对 savePath 解析到 %APPDATA%\wind-daq\<相对>，
	// 与 StorageStartRecording / CalibrationStart 范式一致。
	if savePath != "" {
		resolved, err := a.ResolvePath(savePath)
		if err != nil {
			return FileResponse{Success: false, Error: err.Error()}
		}
		savePath = resolved
	}
	path, err := a.appContext.CalibrationMgr.SaveCsv(taskID, savePath)
	if err != nil {
		return FileResponse{Success: false, Error: err.Error()}
	}
	return FileResponse{Success: true, Filepath: path}
}

// ==================== 存储 API ====================

func (a *App) StorageGetStatus() types.StorageRecordingStatus {
	if a == nil || a.appContext == nil || a.appContext.StorageRecorder == nil {
		return types.StorageRecordingStatus{}
	}
	return a.appContext.StorageRecorder.Status()
}

// StorageStartRecording 启动数据录制。
// 接收完整 RecordingConfig（含 StopConditions/FileRotation/Format 等业务级字段），
// 路径解析统一在后端完成（前端不需要预 resolve），避免双轨配置与重复解析。
func (a *App) StorageStartRecording(config types.StorageRecordingConfig) GenericResponse {
	return a.callMgr(a.storageRecorder(), "存储记录器", func() error {
		resolvedOutputDir, err := a.ResolvePath(config.OutputDir)
		if err != nil {
			return err
		}
		config.OutputDir = resolvedOutputDir
		return a.appContext.StorageRecorder.Start(config)
	})
}

func (a *App) StorageStopRecording() GenericResponse {
	return a.callMgr(a.storageRecorder(), "存储记录器", func() error {
		return a.appContext.StorageRecorder.Stop()
	})
}

// ==================== 报告 API ====================

func (a *App) ReportGetStatus() types.ReportStatus {
	if a == nil || a.appContext == nil || a.appContext.ReportManager == nil {
		return types.ReportStatus{}
	}
	return a.appContext.ReportManager.Status()
}

// ==================== 配置 API ====================

func (a *App) ConfigLoad(key string) (map[string]any, error) {
	if a == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	if a.appContext == nil || a.appContext.ConfigManager == nil {
		return nil, fmt.Errorf("配置管理器未初始化")
	}
	data, err := a.appContext.ConfigManager.LoadConfig(key)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return map[string]any{"success": true, "data": nil}, nil
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, err
	}
	return map[string]any{"success": true, "data": decoded}, nil
}

func (a *App) ConfigSave(key string, configJSON string) GenericResponse {
	if a == nil {
		return GenericResponse{Success: false, Error: "应用服务未初始化"}
	}
	return a.callMgr(a.configManager(), "配置管理器", func() error {
		return a.appContext.ConfigManager.SaveConfig(key, json.RawMessage(configJSON))
	})
}
