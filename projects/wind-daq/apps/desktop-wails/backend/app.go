package backend

import (
	"context"
	"fmt"
	"log"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"wind-daq/services/api-go/pkg/appcontext"
	"wind-daq/services/api-go/pkg/types"
	wind_usecase "wind-daq/services/api-go/pkg/usecase"
)

// App 是 Wails 应用的主结构体
type App struct {
	ctx        context.Context
	cancel     context.CancelFunc
	appContext *appcontext.AppContext
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

	log.Println("Wind-DAQ 应用已成功初始化")
}

// Shutdown 应用关闭时调用
func (a *App) Shutdown(ctx context.Context) {
	if a.cancel != nil {
		a.cancel()
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
	return a.appContext.MotionManager.StatusAll()
}

func (a *App) MotionConnect(id string) GenericResponse {
	return a.callMgr(a.appContext.MotionManager, "运动管理器", func() error {
		return a.appContext.MotionManager.Connect(id)
	})
}

func (a *App) MotionDisconnect(id string) GenericResponse {
	return a.callMgr(a.appContext.MotionManager, "运动管理器", func() error {
		return a.appContext.MotionManager.Disconnect(id)
	})
}

func (a *App) MotionHome(id string, axis string) GenericResponse {
	return a.callMgr(a.appContext.MotionManager, "运动管理器", func() error {
		return a.appContext.MotionManager.Home(id, types.MotionAxisName(axis))
	})
}

func (a *App) MotionStop(id string, axis string) GenericResponse {
	return a.callMgr(a.appContext.MotionManager, "运动管理器", func() error {
		var axisName types.MotionAxisName
		if axis != "" {
			axisName = types.MotionAxisName(axis)
		}
		return a.appContext.MotionManager.Stop(id, axisName)
	})
}

func (a *App) MotionEmergencyStop(id string) GenericResponse {
	return a.callMgr(a.appContext.MotionManager, "运动管理器", func() error {
		return a.appContext.MotionManager.EmergencyStop(id)
	})
}

func (a *App) MotionMoveTo(id string, axis string, position float64) GenericResponse {
	return a.callMgr(a.appContext.MotionManager, "运动管理器", func() error {
		return a.appContext.MotionManager.MoveTo(id, types.MotionAxisName(axis), position)
	})
}

func (a *App) MotionMoveBy(id string, axis string, delta float64) GenericResponse {
	return a.callMgr(a.appContext.MotionManager, "运动管理器", func() error {
		return a.appContext.MotionManager.MoveBy(id, types.MotionAxisName(axis), delta)
	})
}

func (a *App) MotionJog(id string, axis string, velocity float64) GenericResponse {
	return a.callMgr(a.appContext.MotionManager, "运动管理器", func() error {
		return a.appContext.MotionManager.Jog(id, types.MotionAxisName(axis), velocity)
	})
}

func (a *App) MotionDefinePosition(id string, axis string, position float64) GenericResponse {
	return a.callMgr(a.appContext.MotionManager, "运动管理器", func() error {
		return a.appContext.MotionManager.DefinePosition(id, types.MotionAxisName(axis), position)
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
