package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"shared.local/device-sdk/go/pkg/slog"

	"windlabx4/services/api-go/api"
	"windlabx4/services/api-go/pkg/appcontext"
	"windlabx4/services/api-go/pkg/logging"
	"windlabx4/services/api-go/pkg/types"
	wind_usecase "windlabx4/services/api-go/pkg/usecase"
)

// 启动模式常量
const (
	// ModeNormal 主窗口模式，加载完整仪表盘及所有后台服务
	ModeNormal = "normal"
	// ModeMotion 运动控制器独立窗口模式，仅启动运动相关服务
	ModeMotion = "motion"
)

// buildVersion 应用版本号，由构建期通过
// `-X windlabx4/apps/desktop-wails/backend.buildVersion=<version>` 注入
// （Taskfile build-go 从 VERSION 读取）；未注入时回退 "dev"。
// 不再硬编码版本字符串——历史硬编码 "1.0.0" 导致顶栏版本号与发布版本脱节
// （release-versioning 的 VERSION/package.json 已 bump，软件内却永远显示 v1.0.0）。
var buildVersion = "dev"

// GenericResponse 通用响应结构。
// Win7 LTS 版本移除了 Wails 绑定方法，绝大多数 API 通过 HTTP 暴露；
// 保留此类型仅用于 StorageStartRecording/ConfigSave 这两个仍由单元测试直接调用的方法。
// Data 字段用于需要返回数据的调用（如 CalibrationPreviewSevenHole 返回点位预览结果）。
// 简单的成功/失败响应（如 CalibrationStart/Pause/Stop）不填 Data，序列化时 omitempty 省略。
type GenericResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// App 是 windlabx4 桌面应用的主结构体（Win7 LTS 版本）。
//
// 与 trunk 主分支的差异：
//   - 移除 Wails v3 依赖（Wails v3 内部用 log/slog + maps + slices，需 Go 1.21+）
//   - Start/Stop 替代原 Wails ServiceStartup/ServiceShutdown
//   - 实现 api.AppHandler 接口，把 version/startup-mode/open-motion-window/resolve-path
//     通过 HTTP 路由暴露给前端
//   - 删除所有 Wails 绑定方法（Device*/Motion*/Calibration*/Report* 等），这些 API 已通过
//     api.NewRouter 的 HTTP 路由层完整覆盖
//   - tryAutoStartRecording 改为 OnAcquisitionStarted 回调，在 HTTP startAcquisition 路由中
//     异步触发，保留"采集启动后自动开始录制"业务策略
//   - HTTP server 生命周期由 main.go 控制；app.go 通过 NewDeps() 暴露 api.Deps
type App struct {
	ctx        context.Context
	cancel     context.CancelFunc
	appContext *appcontext.AppContext
	relayStop  func()
	// traversalShutdown 双探针 registry 关停入口（spec FR9）。
	// 默认为 appContext.TraversalRegistry.Shutdown；测试注入替身以覆盖 fatal 路径。
	// 补自 master shutdown 重构（lts/win7 273d704 merge 时漏合 app.go 此字段）。
	traversalShutdown func(ctx context.Context) error
	// hostExit fatal 路径的非零退出 seam（默认 os.Exit；测试注入以观测退出码，
	// 避免测试进程被真实退出）。registry shutdown 失败时跳过共享服务 Close 后调用。
	hostExit func(code int)
	// exitWatchdog 在用户确认退出时启动最终兜底；测试可注入同步替身。
	// 补自 master shutdown 重构（lts/win7 273d704 merge 时漏合 app.go 此字段）。
	exitWatchdog func()
	logMgr     *logging.Manager
	// mode 启动模式：normal 或 motion，决定 Start 时加载哪些后台服务
	mode string
	// motionWindowMu 保护 motionWindowCmd，避免重复启动独立窗口进程
	motionWindowMu sync.Mutex
	// motionWindowCmd 已启动的运动控制器独立窗口进程句柄（可能已 Release）
	motionWindowCmd *exec.Cmd
	// shuttingDown 防止应用关闭后后台协程继续往已关闭的 HTTP 客户端推送数据。
	shuttingDown atomic.Bool
	// parentPID 仅 ModeMotion 子进程使用：父进程消失时本进程自杀，避免成为孤儿
	parentPID int
}

// 编译期接口检查：App 必须实现 api.AppHandler。
// 若接口签名变更导致 App 不再满足，编译期即可发现，避免运行时 nil 调用。
var _ api.AppHandler = (*App)(nil)

// NewApp 创建新的 App 实例。
// mode 为启动模式："normal"（主窗口）或 "motion"（运动控制器独立窗口）。
func NewApp(mode string) *App {
	if mode != ModeMotion {
		mode = ModeNormal
	}
	app := &App{mode: mode}
	// exitWatchdog 在用户确认退出时启动最终兜底，避免优雅退出超时后进程残留。
	// 测试可注入同步替身。补自 master shutdown 重构（lts/win7 273d704 漏合）。
	app.exitWatchdog = func() {
		time.AfterFunc(30*time.Second, func() {
			slog.Error("[app] 优雅退出超时，强制结束残留进程",
				"component", "app", "timeout", "30s")
			if app.hostExit != nil {
				app.hostExit(0)
			}
		})
	}
	return app
}

// SetParentPID 仅 ModeMotion 子进程使用：在 Start 之前把父进程 PID 注入。
// Start 时启动 watchdog 协程，发现父进程不存在则触发自杀，
// 解决任务管理器强杀父进程导致子进程成为孤儿的问题。
func (a *App) SetParentPID(pid int) {
	a.parentPID = pid
}

// Start 启动应用后台服务（替代原 Wails ServiceStartup）。
// 由 main.go 在 HTTP server 启动前调用，传入应用级 ctx。
// 返回 error 而非吞掉错误，让 main.go 能在初始化失败时打印错误并退出。
//
// 注意：HTTP server 不在此处启动；main.go 通过 NewDeps() 拿到 api.Deps 后
// 自行创建 mux、挂载 API 路由和静态资源、启动 http.Server。
func (a *App) Start(ctx context.Context) error {
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.shuttingDown.Store(false)

	// 初始化统一日志系统：stderr + 文件 data/logs/windlabx4-YYYYMMDD.log + 内存 ring buffer
	// 必须在所有其他后台服务启动之前调用，确保其后的 slog.Info/Warn/Error 全部被捕获。
	// 日志目录通过 ResolvePath 解析到用户可写目录（%APPDATA%/windlabx4/data/logs），
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
		return initErr
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
	// 主进程启动后，后台异步连接所有标记为 AutoConnect 的位移机构，
	// 避免用户必须先打开运动控制面板才能触发连接。
	a.startMotionAutoConnect()
	// 后台异步连接所有标记为 AutoConnect 的 DAQ 设备（含模拟设备）。
	// 采集必须由用户显式点击"开始采集"触发。
	a.startDeviceAutoConnect()

	slog.Info("[app] WindLabX4 应用已成功初始化", "component", "app")
	return nil
}

// NewDeps 构造并返回 api.Deps，供 main.go 调用 api.NewRouter(deps) 创建 HTTP 路由。
//
// 此处统一注入：
//   - 所有业务 manager（device/acquisition/motion/calibration/traversal/storage/config/report）
//   - 日志 ring buffer 和 manager
//   - AppHandler=a，让 HTTP 路由层暴露 /api/app/* 端点
//   - OnAcquisitionStarted=a.tryAutoStartRecording，让 startAcquisition 路由异步触发"自动录制检查"
//
// 在 motion-only 模式下也会返回有效 Deps（appContext 已在 Start 中初始化），
// 让 motion-only 子进程也能通过 HTTP 暴露运动 API（监听不同端口避免与主进程冲突）。
func (a *App) NewDeps() api.Deps {
	ring := func() *logging.RingBuffer {
		if a.logMgr != nil {
			return a.logMgr.Ring()
		}
		return nil
	}()
	return api.Deps{
		DeviceManager:        a.appContext.DeviceManager,
		AcquisitionHub:       a.appContext.AcquisitionHub,
		ReportManager:        a.appContext.ReportManager,
		MotionManager:        a.appContext.MotionManager,
		MotionService:        a.appContext.MotionManagerRaw,
		CalibrationManager:  a.appContext.CalibrationMgr,
		TraversalManager:    a.appContext.TraversalMgr,
		TraversalRegistry:   a.appContext.TraversalRegistry,
		StorageRecorder:     a.appContext.StorageRecorder,
		ConfigManager:       a.appContext.ConfigManager,
		LogRing:             ring,
		LogManager:          a.logMgr,
		// AppHandler 由 App 实现，提供应用层 HTTP 端点
		AppHandler: a,
		// OnAcquisitionStarted 在采集启动成功后异步调用，
		// 实现"采集启动后自动开始录制"业务策略（读 storage-settings.autoStartOnAcquisition）
		OnAcquisitionStarted: a.tryAutoStartRecording,
	}
}

// startDataRelay 启动采集数据中继。
//
// 设计说明（与 trunk 主分支保持一致）：
//   - 前端按全局刷新频率 HTTP 轮询 /api/daq/latest/{id} 拿最新数据；
//     轮询间隔由前端 deviceApi.setPublishRate/getPublishRate 同步的 Hz 决定；
//     AcquisitionHub.OnData 始终更新 latestByDevice，不受 publishHz 节流影响。
//   - 后端这里仅 drain relay.Payloads() 通道，避免 relay goroutine 因通道满而
//     反压 AcquisitionHub。Win7 LTS 版本不再有 Wails Emit 调用，纯通道 drain。
//   - 保留 DataStreamRelay 与 HTTP /api/daq/subscribe 动作是为了不破坏前端
//     subscribeStream 调用契约（前端仍会调一次 subscribe 来标记订阅意图）。
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
				// 仅 drain payload；前端通过 HTTP 轮询拿数据。
			}
		}
	}()
}

// startMotionAutoConnect 在后台异步连接所有 AutoConnect=true 的位移机构。
//
// 设计要点：
//   - 必须异步执行：底层 TCP 连接（B140 等真实硬件）可能耗时数百毫秒至几秒，
//     若同步阻塞 Start，HTTP server 启动会被卡住导致前端长时间连不上。
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
		// LoadProfiles 内部已自带同步，但延后 100ms 也能避免与 Start 其它 goroutine 抢锁）
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

// Stop 停止应用后台服务（替代原 Wails ServiceShutdown）。
// 由 main.go 在收到 SIGINT/SIGTERM 或 ctx.Done 时调用，应在 HTTP server Shutdown 之后调用。
// 返回 error 用于未来扩展（当前关停流程不可逆，永远返回 nil）。
func (a *App) Stop() error {
	a.shuttingDown.Store(true)

	// 双探针 registry 先行关停（spec FR9）：失败时 fatal + 非零 exit seam，
	// 跳过后续共享服务 cleanup（relayStop/terminateMotionWindow 等）。
	// 测试可经 traversalShutdown/hostExit 字段注入替身覆盖 fatal 路径。
	// 补自 master shutdown 重构（lts/win7 273d704 merge 时漏合此逻辑）。
	if a.traversalShutdown != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := a.traversalShutdown(ctx); err != nil {
			slog.Error("fatal: traversal registry shutdown 失败，跳过共享服务关闭",
				"component", "app", "error", err)
			if a.hostExit != nil {
				a.hostExit(2)
			}
			return err
		}
	}

	if a.relayStop != nil {
		a.relayStop()
	}

	// 先关闭子窗口进程：保证父进程退出时不会留下孤儿运动控制器窗口。
	// 顺序放在最前是因为子进程仍可能向父进程的日志/HTTP 写数据，先停子再停父更稳。
	a.terminateMotionWindow()

	if a.cancel != nil {
		a.cancel()
	}
	// 在 logMgr.Close 之前打这条收尾日志，确保它能写入文件/ring sink。
	slog.Info("WindLabX4 应用已关闭", "component", "app")
	if a.logMgr != nil {
		_ = a.logMgr.Close()
	}
	return nil
}

// terminateMotionWindow 关闭运动控制器独立窗口子进程（如已启动）。
// 子进程是 GUI 进程，Windows 下没有可用的 SIGTERM 软关停信号，
// 因此直接 Kill；子进程内部没有需要持久化的中途状态，可以接受。
//
// 调用方：仅父进程的 Stop。子进程自身退出时，由 OpenMotionWindow
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

// Version 实现 api.AppHandler.Version。
// 返回应用版本信息（名称 + 版本号），供前端在关于对话框/标题栏显示。
func (a *App) Version() api.AppVersionInfo {
	return api.AppVersionInfo{
		Name: "WindLabX4",
		Version: buildVersion,
	}
}

// StartupMode 实现 api.AppHandler.StartupMode。
// 返回 "normal"（主窗口）或 "motion"（运动控制器独立窗口）。
// 前端据此决定加载完整仪表盘还是仅运动控制面板。
func (a *App) StartupMode() string {
	return a.mode
}

// OpenMotionWindow 实现 api.AppHandler.OpenMotionWindow。
// 启动运动控制器独立窗口子进程（独立进程），失败返回 error。
// 通过重新启动当前可执行文件并传入 --motion-only 参数实现真正的独立窗口。
// 使用互斥锁防止重复启动。
func (a *App) OpenMotionWindow() error {
	a.motionWindowMu.Lock()
	defer a.motionWindowMu.Unlock()

	// 检查已有进程是否仍在运行（非阻塞探测）
	if a.motionWindowCmd != nil && a.motionWindowCmd.Process != nil {
		// 若进程已退出则清理句柄，否则提示用户窗口已打开
		if a.motionWindowCmd.ProcessState != nil && a.motionWindowCmd.ProcessState.Exited() {
			a.motionWindowCmd = nil
		} else {
			return fmt.Errorf("运动控制器独立窗口已打开，请勿重复启动")
		}
	}

	exePath, err := os.Executable()
	if err != nil {
		slog.Error("获取可执行文件路径失败", "component", "motion-window", "error", err)
		return fmt.Errorf("获取可执行文件路径失败: %w", err)
	}

	// 启动独立进程，通过环境变量传递 motion-only 模式；同时把父进程 PID 传过去，
	// 子进程的 watchdog 在父进程消失时触发自杀，避免任务管理器强杀父进程留下孤儿。
	childEnv := append(os.Environ(), "WINDLABX4_MOTION_ONLY=1", fmt.Sprintf("WINDLABX4_PARENT_PID=%d", os.Getpid()))
	cmd := exec.Command(exePath)
	cmd.Env = childEnv
	// 独立进程继承标准输出和错误，便于调试
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// 不等待子进程，解耦父子进程生命周期
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		slog.Error("启动运动控制器独立窗口失败", "component", "motion-window", "error", err)
		return fmt.Errorf("启动独立窗口失败: %w", err)
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

	return nil
}

// ResolvePath 实现 api.AppHandler.ResolvePath。
// 将相对路径解析到用户可写的应用目录（%APPDATA%\windlabx4），避免安装目录不可写。
// 绝对路径原样返回（仅做 Clean）；空字符串返回空字符串。
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

// writableUserConfigDir 返回用户可写的应用配置目录（Windows 下为 %APPDATA%\windlabx4）。
// 命名对齐 os.UserConfigDir() 语义：既是配置目录也是数据目录基础。
func writableUserConfigDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "windlabx4"), nil
}

// callMgr 通用 manager 方法调用辅助。
// 统一处理 nil App / nil appContext / nil manager 三种初始化失败场景，
// 避免每个包装方法重复写 if a == nil || ... 检查。
func (a *App) callMgr(mgr any, name string, fn func() error) GenericResponse {
	if a == nil || a.appContext == nil || mgr == nil {
		return GenericResponse{Success: false, Error: name + "未初始化"}
	}
	if err := fn(); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// storageRecorder 返回存储记录器实例（仅用于 nil 检查包装）。
func (a *App) storageRecorder() any {
	if a == nil || a.appContext == nil {
		return nil
	}
	return a.appContext.StorageRecorder
}

// configManager 返回配置管理器实例（仅用于 nil 检查包装）。
func (a *App) configManager() any {
	if a == nil || a.appContext == nil {
		return nil
	}
	return a.appContext.ConfigManager
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
// 失败仅记录日志，不阻塞采集流程；异步调用，不阻塞 startAcquisition 响应。
//
// 此方法作为 OnAcquisitionStarted 回调注入到 api.Deps，
// 在 HTTP /api/daq/{id}/startAcquisition 路由中异步触发，与原 Wails DeviceStartAcquisition 行为对齐。
// 参数 deviceID 由路由层传入，当前实现未使用（业务策略基于全局 storage-settings），
// 保留参数以匹配 OnAcquisitionStarted func(deviceID string) 签名。
func (a *App) tryAutoStartRecording(deviceID string) {
	_ = deviceID // 当前未使用，保留以匹配回调签名
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
	config := wind_usecase.StorageRecordingConfig{
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

// ==================== 存储 API（保留测试依赖方法） ====================
// 这两个方法仅因 app_test.go 直接调用而保留；前端通过 HTTP /api/storage/* 路由调用，
// 不再走这两个 Go 方法。

// StorageGetStatus 返回存储录制状态。
// 保留此方法是因为 app_test.go 的 TestStorageStartRecordingResolvesRelativeOutputDir 测试
// 通过 StorageGetStatus 验证 StorageStartRecording 是否成功设置 OutputDir。
func (a *App) StorageGetStatus() wind_usecase.StorageRecordingStatus {
	if a == nil || a.appContext == nil || a.appContext.StorageRecorder == nil {
		return wind_usecase.StorageRecordingStatus{}
	}
	return a.appContext.StorageRecorder.Status()
}

// StorageStartRecording 启动数据录制。
// 接收完整 RecordingConfig（含 StopConditions/FileRotation/Format 等业务级字段），
// 路径解析统一在后端完成（前端不需要预 resolve），避免双轨配置与重复解析。
// 保留此方法是因为 app_test.go 的 TestStorageStartRecordingResolvesRelativeOutputDir 测试
// 直接调用此方法验证相对路径会被解析到 %APPDATA%\windlabx4 下。
func (a *App) StorageStartRecording(config wind_usecase.StorageRecordingConfig) GenericResponse {
	return a.callMgr(a.storageRecorder(), "存储记录器", func() error {
		resolvedOutputDir, err := a.ResolvePath(config.OutputDir)
		if err != nil {
			return err
		}
		config.OutputDir = resolvedOutputDir
		return a.appContext.StorageRecorder.Start(config)
	})
}

// ==================== 配置 API（保留测试依赖方法） ====================
// 这两个方法仅因 app_test.go 直接调用而保留；前端通过 HTTP /api/config/* 路由调用，
// 不再走这两个 Go 方法。

// ConfigLoad 从配置管理器加载指定 key 的配置。
// 保留此方法是因为 app_test.go 中有 3 个测试直接调用此方法验证配置读写往返。
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

// ConfigSave 将配置 JSON 保存到指定 key。
// 保留此方法是因为 app_test.go 中有 3 个测试直接调用此方法验证配置读写。
func (a *App) ConfigSave(key string, configJSON string) GenericResponse {
	if a == nil {
		return GenericResponse{Success: false, Error: "应用服务未初始化"}
	}
	return a.callMgr(a.configManager(), "配置管理器", func() error {
		return a.appContext.ConfigManager.SaveConfig(key, json.RawMessage(configJSON))
	})
}
