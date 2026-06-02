package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"wind-daq/services/api-go/api"
	"wind-daq/services/api-go/pkg/appcontext"
	"wind-daq/services/api-go/pkg/types"
	wind_usecase "wind-daq/services/api-go/pkg/usecase"
)

// App 是 Wails 应用的主结构体
type App struct {
	ctx        context.Context
	cancel     context.CancelFunc
	appContext *appcontext.AppContext
	apiServer  *http.Server

	// 采集数据推送相关
	streamMu       sync.Mutex
	streamCancel   context.CancelFunc
	streamChannels map[string]chan types.DeviceDataPayload

	// 运动状态推送相关
	motionStatusCancel context.CancelFunc
}

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

// NewApp 创建新的 App 实例
func NewApp() *App {
	return &App{}
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

	// 启动采集数据推送 goroutine，将 AcquisitionHub 数据通过 Wails Events 推送到前端
	a.startStreamRelay()
	// 启动运动状态定时推送
	a.startMotionStatusRelay()
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

// startStreamRelay 启动采集数据中继，将 hub 数据通过 Wails EventsEmit 推送到前端
func (a *App) startStreamRelay() {
	a.streamMu.Lock()
	defer a.streamMu.Unlock()

	if a.streamCancel != nil {
		a.streamCancel()
	}
	streamCtx, cancel := context.WithCancel(a.ctx)
	a.streamCancel = cancel
	a.streamChannels = make(map[string]chan types.DeviceDataPayload)

	go a.streamRelayLoop(streamCtx)
}

// streamRelayLoop 监听所有已订阅设备的采集数据并推送到前端
func (a *App) streamRelayLoop(ctx context.Context) {
	if a.appContext == nil || a.appContext.AcquisitionHub == nil {
		return
	}
	type sub struct {
		ch          <-chan types.DeviceDataPayload
		unsubscribe func()
	}
	var subs sync.Map

	for {
		select {
		case <-ctx.Done():
			// 清理所有订阅
			subs.Range(func(key, value any) bool {
				if s, ok := value.(sub); ok {
					s.unsubscribe()
				}
				return true
			})
			return
		default:
		}

		// 检查当前需要订阅的设备列表
		a.streamMu.Lock()
		needed := make(map[string]struct{}, len(a.streamChannels))
		for id := range a.streamChannels {
			needed[id] = struct{}{}
		}
		a.streamMu.Unlock()

		// 订阅新设备
		for id := range needed {
			if _, exists := subs.Load(id); exists {
				continue
			}
			ch, unsub := a.appContext.AcquisitionHub.Subscribe(id, 16)
			subs.Store(id, sub{ch: ch, unsubscribe: unsub})
			go func(deviceID string, payloadCh <-chan types.DeviceDataPayload) {
				for {
					select {
					case <-ctx.Done():
						return
					case payload, ok := <-payloadCh:
						if !ok {
							return
						}
						runtime.EventsEmit(a.ctx, "daq:payload", payload)
					}
				}
			}(id, ch)
		}

		// 取消不再需要的订阅
		subs.Range(func(key, value any) bool {
			id := key.(string)
			if _, ok := needed[id]; !ok {
				if s, ok := value.(sub); ok {
					s.unsubscribe()
				}
				subs.Delete(key)
			}
			return true
		})

		// 短暂休眠避免忙等
		select {
		case <-ctx.Done():
			return
		case <-streamCtxSleep(ctx, 500*time.Millisecond):
		}
	}
}

func streamCtxSleep(ctx context.Context, d time.Duration) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
		case <-time.After(d):
		}
		close(ch)
	}()
	return ch
}

// startMotionStatusRelay 启动运动状态定时推送，将控制器状态通过 Wails EventsEmit 推送到前端
func (a *App) startMotionStatusRelay() {
	if a.motionStatusCancel != nil {
		a.motionStatusCancel()
	}
	motionCtx, cancel := context.WithCancel(a.ctx)
	a.motionStatusCancel = cancel

	go a.motionStatusRelayLoop(motionCtx)
}

// motionStatusRelayLoop 定时轮询运动控制器状态并推送到前端
func (a *App) motionStatusRelayLoop(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if a.appContext == nil || a.appContext.MotionManager == nil {
				continue
			}
			statuses := a.appContext.MotionManager.StatusAll(a.ctx)
			if len(statuses) > 0 {
				runtime.EventsEmit(a.ctx, "motion:status", statuses)
			}
		}
	}
}

// DeviceSubscribeStream 前端调用此方法来订阅/取消订阅某个设备的采集数据流
func (a *App) DeviceSubscribeStream(deviceID string, subscribe bool) GenericResponse {
	a.streamMu.Lock()
	defer a.streamMu.Unlock()

	if a.streamChannels == nil {
		a.streamChannels = make(map[string]chan types.DeviceDataPayload)
	}

	if subscribe {
		a.streamChannels[deviceID] = make(chan types.DeviceDataPayload, 16)
	} else {
		delete(a.streamChannels, deviceID)
	}

	return GenericResponse{Success: true}
}

// Shutdown 应用关闭时调用
func (a *App) Shutdown(ctx context.Context) {
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

// GetVersion 获取版本信息
func (a *App) GetVersion() VersionInfo {
	return VersionInfo{
		Name:    "Wind-DAQ",
		Version: "1.0.0",
	}
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
