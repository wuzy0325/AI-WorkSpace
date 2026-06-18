package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"wind-daq/services/api-go/api"
	"wind-daq/services/api-go/pkg/appcontext"
	"wind-daq/services/api-go/pkg/types"
	wind_usecase "wind-daq/services/api-go/pkg/usecase"
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
type GenericResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// App 是 Wails 应用的主结构体
type App struct {
	ctx        context.Context
	cancel     context.CancelFunc
	appContext *appcontext.AppContext
	apiServer  *http.Server
	relayStop  func()
	// mode 启动模式：normal 或 motion，决定 Startup 时加载哪些后台服务
	mode string
	// motionWindowMu 保护 motionWindowCmd，避免重复启动独立窗口进程
	motionWindowMu sync.Mutex
	// motionWindowCmd 已启动的运动控制器独立窗口进程句柄（可能已 Release）
	motionWindowCmd *exec.Cmd
	// parentPID 仅 ModeMotion 子进程使用：父进程消失时本进程自杀，避免成为孤儿
	parentPID int
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

// Startup 应用启动时调用
func (a *App) Startup(ctx context.Context) {
	a.ctx, a.cancel = context.WithCancel(ctx)

	var err error
	a.appContext, err = appcontext.NewAppContext("")
	if err != nil {
		log.Printf("服务初始化错误: %v", err)
		runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "初始化错误",
			Message: fmt.Sprintf("服务初始化失败: %v", err),
		})
		return
	}

	// 运动控制器独立窗口进程：仅启动运动状态轮询，避免与主窗口进程冲突
	// （API 服务器端口、数据中继、硬件采集由主窗口进程负责）
	if a.mode == ModeMotion {
		a.startMotionPoller()
		// 启动父进程看护：父进程消失时本进程自杀，避免任务管理器强杀父进程后留下孤儿。
		if a.parentPID > 0 {
			a.startParentWatchdog()
		}
		log.Println("Wind-DAQ 运动控制器独立窗口已初始化（仅运动服务）")
		return
	}

	// 主窗口进程：启动全部后台服务
	a.startDataRelay()
	a.startMotionPoller()
	a.startLocalAPIServer()

	log.Println("Wind-DAQ 应用已成功初始化")
}

func (a *App) startLocalAPIServer() {
	if a.appContext == nil {
		return
	}
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
		}),
	}
	go func() {
		log.Println("Wind-DAQ local API listening on http://127.0.0.1:8900")
		if err := a.apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Wind-DAQ local API failed: %v", err)
		}
	}()
}

// startDataRelay 启动采集数据中继，将 DataStreamRelay 输出通过 Wails EventsEmit 推送到前端
func (a *App) startDataRelay() {
	relay := a.appContext.DataStreamRelay
	if relay == nil {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.relayStop = cancel
	go func() {
		for {
			select {
			case <-ctx.Done():
				relay.Stop()
				return
			case payload, ok := <-relay.Payloads():
				if !ok {
					return
				}
				runtime.EventsEmit(a.ctx, "daq:payload", payload)
			}
		}
	}()
}

// startMotionPoller 启动运动状态轮询，将 MotionStatusPoller 输出通过 Wails EventsEmit 推送到前端
func (a *App) startMotionPoller() {
	poller := a.appContext.MotionStatusPoller
	if poller == nil {
		return
	}
	poller.Start(a.ctx)
	go func() {
		for statuses := range poller.Status() {
			runtime.EventsEmit(a.ctx, "motion:status", statuses)
		}
	}()
}

// DeviceSubscribeStream 前端调用此方法来订阅/取消订阅某个设备的采集数据流
func (a *App) DeviceSubscribeStream(deviceID string, subscribe bool) GenericResponse {
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

// Shutdown 应用关闭时调用
func (a *App) Shutdown(ctx context.Context) {
	// 先关闭子窗口进程：保证父进程退出时不会留下孤儿运动控制器窗口。
	// 顺序放在最前是因为子进程仍可能向父进程的日志/HTTP 写数据，先停子再停父更稳。
	a.terminateMotionWindow()

	if a.relayStop != nil {
		a.relayStop()
	}
	if a.appContext != nil && a.appContext.MotionStatusPoller != nil {
		a.appContext.MotionStatusPoller.Stop()
	}
	if a.cancel != nil {
		a.cancel()
	}
	if a.apiServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = a.apiServer.Shutdown(shutdownCtx)
	}
	log.Println("Wind-DAQ 应用已关闭")
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
		log.Printf("kill motion window (pid=%d) failed: %v", pid, err)
		return
	}
	log.Printf("已关闭运动控制器独立窗口子进程 (pid=%d)", pid)
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
		log.Printf("获取可执行文件路径失败: %v", err)
		return GenericResponse{Success: false, Error: fmt.Sprintf("获取可执行文件路径失败: %v", err)}
	}

	// 启动独立进程，传入 --motion-only 参数；同时把父进程 PID 传过去，
	// 子进程的 watchdog 在父进程消失时触发自杀，避免任务管理器强杀父进程留下孤儿。
	cmd := exec.Command(exePath, "--motion-only", fmt.Sprintf("--parent-pid=%d", os.Getpid()))
	// 独立进程继承标准输出和错误，便于调试
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// 不等待子进程，解耦父子进程生命周期
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		log.Printf("启动运动控制器独立窗口失败: %v", err)
		return GenericResponse{Success: false, Error: fmt.Sprintf("启动独立窗口失败: %v", err)}
	}

	log.Printf("运动控制器独立窗口已启动，PID: %d", cmd.Process.Pid)
	a.motionWindowCmd = cmd

	// 后台监控子进程退出，退出后清理句柄以允许再次启动
	go func(c *exec.Cmd) {
		_ = c.Wait()
		a.motionWindowMu.Lock()
		if a.motionWindowCmd == c {
			a.motionWindowCmd = nil
		}
		a.motionWindowMu.Unlock()
		log.Println("运动控制器独立窗口进程已退出")
	}(cmd)

	return GenericResponse{Success: true}
}

// PickDirectory 选择目录对话框
func (a *App) PickDirectory() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("应用上下文未初始化")
	}
	opts := runtime.OpenDialogOptions{
		Title:                "选择保存目录",
		CanCreateDirectories: true,
	}
	return runtime.OpenDirectoryDialog(a.ctx, opts)
}

func (a *App) PickFile(title string, filters []runtime.FileFilter) (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("应用上下文未初始化")
	}
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   title,
		Filters: filters,
	})
}

func (a *App) PickFiles(title string, filters []runtime.FileFilter) ([]string, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("应用上下文未初始化")
	}
	return runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   title,
		Filters: filters,
	})
}

// callMgr 通用 manager 方法调用辅助
func (a *App) callMgr(mgr any, name string, fn func() error) GenericResponse {
	if a.appContext == nil || mgr == nil {
		return GenericResponse{Success: false, Error: name + "未初始化"}
	}
	if err := fn(); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// ==================== 设备管理 API ====================

func (a *App) DeviceGetProfiles() []types.DeviceProfile {
	if a.appContext == nil || a.appContext.DeviceManager == nil {
		return nil
	}
	return a.appContext.DeviceManager.GetProfiles()
}

func (a *App) DeviceUpsertProfile(profile types.DeviceProfile) GenericResponse {
	return a.callMgr(a.appContext.DeviceManager, "设备管理器", func() error {
		return a.appContext.DeviceManager.UpsertProfile(profile)
	})
}

func (a *App) DeviceDeleteProfile(id string) GenericResponse {
	return a.callMgr(a.appContext.DeviceManager, "设备管理器", func() error {
		return a.appContext.DeviceManager.DeleteProfile(id)
	})
}

func (a *App) DeviceScanDevices() ([]types.DeviceScanResult, error) {
	if a.appContext == nil || a.appContext.DeviceManager == nil {
		return nil, fmt.Errorf("设备管理器未初始化")
	}
	return a.appContext.DeviceManager.ScanDevices()
}

func (a *App) DeviceConnect(id string) GenericResponse {
	return a.callMgr(a.appContext.DeviceManager, "设备管理器", func() error {
		return a.appContext.DeviceManager.Connect(id)
	})
}

func (a *App) DeviceDisconnect(id string) GenericResponse {
	return a.callMgr(a.appContext.DeviceManager, "设备管理器", func() error {
		return a.appContext.DeviceManager.Disconnect(id)
	})
}

func (a *App) DeviceStartAcquisition(id string) GenericResponse {
	return a.callMgr(a.appContext.DeviceManager, "设备管理器", func() error {
		return a.appContext.DeviceManager.StartAcquisition(id)
	})
}

func (a *App) DeviceStopAcquisition(id string) GenericResponse {
	return a.callMgr(a.appContext.DeviceManager, "设备管理器", func() error {
		return a.appContext.DeviceManager.StopAcquisition(id)
	})
}

func (a *App) DeviceGetStatus(id string) (types.DeviceStatus, bool) {
	if a.appContext == nil || a.appContext.DeviceManager == nil {
		return types.DeviceStatus{}, false
	}
	return a.appContext.DeviceManager.GetStatus(id)
}

func (a *App) DeviceGetLatestData(deviceID string) (types.DeviceDataPayload, bool) {
	if a.appContext == nil || a.appContext.AcquisitionHub == nil {
		return types.DeviceDataPayload{}, false
	}
	return a.appContext.AcquisitionHub.GetLatestData(deviceID)
}

func (a *App) DeviceSetPublishRate(hz float64) GenericResponse {
	return a.callMgr(a.appContext.AcquisitionHub, "采集中心", func() error {
		return a.appContext.AcquisitionHub.SetPublishRate(hz)
	})
}

func (a *App) DeviceGetPublishRate() float64 {
	if a.appContext == nil || a.appContext.AcquisitionHub == nil {
		return 0
	}
	return a.appContext.AcquisitionHub.PublishRate()
}

// ==================== 运动控制 API ====================

func (a *App) MotionGetProfiles() ([]types.MotionControllerProfile, error) {
	if a.appContext == nil || a.appContext.MotionManager == nil {
		return nil, fmt.Errorf("运动管理器未初始化")
	}
	return a.appContext.MotionManager.LoadProfiles()
}

func (a *App) MotionUpsertProfile(profile types.MotionControllerProfile) GenericResponse {
	return a.callMgr(a.appContext.MotionManager, "运动管理器", func() error {
		return a.appContext.MotionManager.UpsertProfile(profile)
	})
}

func (a *App) MotionDeleteProfile(id string) GenericResponse {
	return a.callMgr(a.appContext.MotionManager, "运动管理器", func() error {
		return a.appContext.MotionManager.DeleteProfile(id)
	})
}

func (a *App) MotionGetStatus() []types.MotionControllerStatus {
	if a.appContext == nil || a.appContext.MotionManager == nil {
		return nil
	}
	return a.appContext.MotionManager.StatusAll(a.ctx)
}

func (a *App) MotionConnect(id string) GenericResponse {
	return a.callMgr(a.appContext.MotionManager, "运动管理器", func() error {
		return a.appContext.MotionManager.Connect(a.ctx, id)
	})
}

func (a *App) MotionDisconnect(id string) GenericResponse {
	return a.callMgr(a.appContext.MotionManager, "运动管理器", func() error {
		return a.appContext.MotionManager.Disconnect(a.ctx, id)
	})
}

func (a *App) MotionHome(id string, axis string) GenericResponse {
	return a.callMgr(a.appContext.MotionManager, "运动管理器", func() error {
		return a.appContext.MotionManager.Home(a.ctx, id, types.MotionAxisName(axis))
	})
}

func (a *App) MotionStop(id string, axis string) GenericResponse {
	return a.callMgr(a.appContext.MotionManager, "运动管理器", func() error {
		var axisName types.MotionAxisName
		if axis != "" {
			axisName = types.MotionAxisName(axis)
		}
		return a.appContext.MotionManager.Stop(a.ctx, id, axisName)
	})
}

func (a *App) MotionEmergencyStop(id string) GenericResponse {
	return a.callMgr(a.appContext.MotionManager, "运动管理器", func() error {
		return a.appContext.MotionManager.EmergencyStop(a.ctx, id)
	})
}

func (a *App) MotionMoveTo(id string, axis string, position float64) GenericResponse {
	return a.callMgr(a.appContext.MotionManager, "运动管理器", func() error {
		return a.appContext.MotionManager.MoveTo(a.ctx, id, types.MotionAxisName(axis), position)
	})
}

func (a *App) MotionMoveBy(id string, axis string, delta float64) GenericResponse {
	return a.callMgr(a.appContext.MotionManager, "运动管理器", func() error {
		return a.appContext.MotionManager.MoveBy(a.ctx, id, types.MotionAxisName(axis), delta)
	})
}

func (a *App) MotionJog(id string, axis string, velocity float64) GenericResponse {
	return a.callMgr(a.appContext.MotionManager, "运动管理器", func() error {
		return a.appContext.MotionManager.Jog(a.ctx, id, types.MotionAxisName(axis), velocity)
	})
}

func (a *App) MotionDefinePosition(id string, axis string, position float64) GenericResponse {
	return a.callMgr(a.appContext.MotionManager, "运动管理器", func() error {
		return a.appContext.MotionManager.DefinePosition(a.ctx, id, types.MotionAxisName(axis), position)
	})
}

func (a *App) MotionResetEmergencyStop(id string) GenericResponse {
	return a.callMgr(a.appContext.MotionManager, "运动管理器", func() error {
		return a.appContext.MotionManager.ResetEmergencyStop(a.ctx, id)
	})
}

// ==================== 校准 API ====================

func (a *App) CalibrationStart(config types.CalibrationConfig) GenericResponse {
	return a.callMgr(a.appContext.CalibrationMgr, "校准管理器", func() error {
		return a.appContext.CalibrationMgr.Start(config)
	})
}

func (a *App) CalibrationStatus() types.CalibrationStatus {
	if a.appContext == nil || a.appContext.CalibrationMgr == nil {
		return types.CalibrationStatus{}
	}
	return a.appContext.CalibrationMgr.Status()
}

func (a *App) CalibrationCollect() GenericResponse {
	return a.callMgr(a.appContext.CalibrationMgr, "校准管理器", func() error {
		return a.appContext.CalibrationMgr.CollectCurrentPoint()
	})
}

func (a *App) CalibrationPause() GenericResponse {
	return a.callMgr(a.appContext.CalibrationMgr, "校准管理器", func() error {
		return a.appContext.CalibrationMgr.Pause()
	})
}

func (a *App) CalibrationResume() GenericResponse {
	return a.callMgr(a.appContext.CalibrationMgr, "校准管理器", func() error {
		return a.appContext.CalibrationMgr.Resume()
	})
}

func (a *App) CalibrationStop() GenericResponse {
	return a.callMgr(a.appContext.CalibrationMgr, "校准管理器", func() error {
		return a.appContext.CalibrationMgr.Stop()
	})
}

func (a *App) CalibrationGetResult(taskID string) (types.CalibrationStatus, bool) {
	if a.appContext == nil || a.appContext.CalibrationMgr == nil {
		return types.CalibrationStatus{}, false
	}
	return a.appContext.CalibrationMgr.GetResult(taskID)
}

// ==================== 存储 API ====================

func (a *App) StorageGetStatus() wind_usecase.StorageRecordingStatus {
	if a.appContext == nil || a.appContext.StorageRecorder == nil {
		return wind_usecase.StorageRecordingStatus{}
	}
	return a.appContext.StorageRecorder.Status()
}

func (a *App) StorageStartRecording(outputDir string, filePrefix string) GenericResponse {
	return a.callMgr(a.appContext.StorageRecorder, "存储记录器", func() error {
		return a.appContext.StorageRecorder.Start(wind_usecase.StorageRecordingConfig{
			OutputDir: outputDir, FilePrefix: filePrefix,
		})
	})
}

func (a *App) StorageStopRecording() GenericResponse {
	return a.callMgr(a.appContext.StorageRecorder, "存储记录器", func() error {
		return a.appContext.StorageRecorder.Stop()
	})
}

// ==================== 报告 API ====================

func (a *App) ReportGetStatus() wind_usecase.ReportStatus {
	if a.appContext == nil || a.appContext.ReportManager == nil {
		return wind_usecase.ReportStatus{}
	}
	return a.appContext.ReportManager.Status()
}

// ==================== 配置 API ====================

func (a *App) ConfigLoad(key string) (map[string]any, error) {
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
	return map[string]any{"success": true, "data": json.RawMessage(data)}, nil
}

func (a *App) ConfigSave(key string, configJSON string) GenericResponse {
	return a.callMgr(a.appContext.ConfigManager, "配置管理器", func() error {
		return a.appContext.ConfigManager.SaveConfig(key, json.RawMessage(configJSON))
	})
}
