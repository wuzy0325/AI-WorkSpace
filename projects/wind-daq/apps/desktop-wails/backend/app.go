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
	ctx              context.Context
	cancel           context.CancelFunc
	appContext       *appcontext.AppContext
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

	// 初始化应用上下文
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

// ==================== 设备管理 API ====================

// DeviceGetProfiles 获取所有设备配置
func (a *App) DeviceGetProfiles() []types.DeviceProfile {
	if a.appContext == nil || a.appContext.DeviceManager == nil {
		return nil
	}
	return a.appContext.DeviceManager.GetProfiles()
}

// DeviceUpsertProfile 更新或创建设备配置
func (a *App) DeviceUpsertProfile(profile types.DeviceProfile) GenericResponse {
	if a.appContext == nil || a.appContext.DeviceManager == nil {
		return GenericResponse{Success: false, Error: "设备管理器未初始化"}
	}
	if err := a.appContext.DeviceManager.UpsertProfile(profile); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// DeviceDeleteProfile 删除设备配置
func (a *App) DeviceDeleteProfile(id string) GenericResponse {
	if a.appContext == nil || a.appContext.DeviceManager == nil {
		return GenericResponse{Success: false, Error: "设备管理器未初始化"}
	}
	if err := a.appContext.DeviceManager.DeleteProfile(id); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// DeviceScanDevices 扫描设备
func (a *App) DeviceScanDevices() ([]types.DeviceScanResult, error) {
	if a.appContext == nil || a.appContext.DeviceManager == nil {
		return nil, fmt.Errorf("设备管理器未初始化")
	}
	return a.appContext.DeviceManager.ScanDevices()
}

// DeviceConnect 连接设备
func (a *App) DeviceConnect(id string) GenericResponse {
	if a.appContext == nil || a.appContext.DeviceManager == nil {
		return GenericResponse{Success: false, Error: "设备管理器未初始化"}
	}
	if err := a.appContext.DeviceManager.Connect(id); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// DeviceDisconnect 断开设备
func (a *App) DeviceDisconnect(id string) GenericResponse {
	if a.appContext == nil || a.appContext.DeviceManager == nil {
		return GenericResponse{Success: false, Error: "设备管理器未初始化"}
	}
	if err := a.appContext.DeviceManager.Disconnect(id); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// DeviceStartAcquisition 开始采集
func (a *App) DeviceStartAcquisition(id string) GenericResponse {
	if a.appContext == nil || a.appContext.DeviceManager == nil {
		return GenericResponse{Success: false, Error: "设备管理器未初始化"}
	}
	if err := a.appContext.DeviceManager.StartAcquisition(id); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// DeviceStopAcquisition 停止采集
func (a *App) DeviceStopAcquisition(id string) GenericResponse {
	if a.appContext == nil || a.appContext.DeviceManager == nil {
		return GenericResponse{Success: false, Error: "设备管理器未初始化"}
	}
	if err := a.appContext.DeviceManager.StopAcquisition(id); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// DeviceGetStatus 获取设备状态
func (a *App) DeviceGetStatus(id string) (types.DeviceStatus, bool) {
	if a.appContext == nil || a.appContext.DeviceManager == nil {
		return types.DeviceStatus{}, false
	}
	return a.appContext.DeviceManager.GetStatus(id)
}

// DeviceGetLatestData 获取设备最新数据
func (a *App) DeviceGetLatestData(deviceID string) (types.DeviceDataPayload, bool) {
	if a.appContext == nil || a.appContext.AcquisitionHub == nil {
		return types.DeviceDataPayload{}, false
	}
	return a.appContext.AcquisitionHub.GetLatestData(deviceID)
}

// DeviceSetPublishRate 设置数据发布速率
func (a *App) DeviceSetPublishRate(hz float64) GenericResponse {
	if a.appContext == nil || a.appContext.AcquisitionHub == nil {
		return GenericResponse{Success: false, Error: "采集中心未初始化"}
	}
	if err := a.appContext.AcquisitionHub.SetPublishRate(hz); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// DeviceGetPublishRate 获取数据发布速率
func (a *App) DeviceGetPublishRate() float64 {
	if a.appContext == nil || a.appContext.AcquisitionHub == nil {
		return 0
	}
	return a.appContext.AcquisitionHub.PublishRate()
}

// ==================== 运动控制 API ====================

// MotionGetProfiles 获取运动控制器配置
func (a *App) MotionGetProfiles() []types.MotionControllerProfile {
	if a.appContext == nil || a.appContext.MotionManager == nil {
		return nil
	}
	profiles, _ := a.appContext.MotionManager.LoadProfiles()
	return profiles
}

// MotionUpsertProfile 更新或创建运动控制器配置
func (a *App) MotionUpsertProfile(profile types.MotionControllerProfile) GenericResponse {
	if a.appContext == nil || a.appContext.MotionManager == nil {
		return GenericResponse{Success: false, Error: "运动管理器未初始化"}
	}
	if err := a.appContext.MotionManager.UpsertProfile(profile); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// MotionDeleteProfile 删除运动控制器配置
func (a *App) MotionDeleteProfile(id string) GenericResponse {
	if a.appContext == nil || a.appContext.MotionManager == nil {
		return GenericResponse{Success: false, Error: "运动管理器未初始化"}
	}
	if err := a.appContext.MotionManager.DeleteProfile(id); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// MotionGetStatus 获取所有运动控制器状态
func (a *App) MotionGetStatus() []types.MotionControllerStatus {
	if a.appContext == nil || a.appContext.MotionManager == nil {
		return nil
	}
	return a.appContext.MotionManager.StatusAll()
}

// MotionConnect 连接运动控制器
func (a *App) MotionConnect(id string) GenericResponse {
	if a.appContext == nil || a.appContext.MotionManager == nil {
		return GenericResponse{Success: false, Error: "运动管理器未初始化"}
	}
	if err := a.appContext.MotionManager.Connect(id); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// MotionDisconnect 断开运动控制器
func (a *App) MotionDisconnect(id string) GenericResponse {
	if a.appContext == nil || a.appContext.MotionManager == nil {
		return GenericResponse{Success: false, Error: "运动管理器未初始化"}
	}
	if err := a.appContext.MotionManager.Disconnect(id); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// MotionHome 回原点
func (a *App) MotionHome(id string, axis string) GenericResponse {
	if a.appContext == nil || a.appContext.MotionManager == nil {
		return GenericResponse{Success: false, Error: "运动管理器未初始化"}
	}
	if err := a.appContext.MotionManager.Home(id, types.MotionAxisName(axis)); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// MotionStop 停止运动
func (a *App) MotionStop(id string, axis string) GenericResponse {
	if a.appContext == nil || a.appContext.MotionManager == nil {
		return GenericResponse{Success: false, Error: "运动管理器未初始化"}
	}
	var axisName types.MotionAxisName
	if axis != "" {
		axisName = types.MotionAxisName(axis)
	}
	if err := a.appContext.MotionManager.Stop(id, axisName); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// MotionEmergencyStop 紧急停止
func (a *App) MotionEmergencyStop(id string) GenericResponse {
	if a.appContext == nil || a.appContext.MotionManager == nil {
		return GenericResponse{Success: false, Error: "运动管理器未初始化"}
	}
	if err := a.appContext.MotionManager.EmergencyStop(id); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// MotionMoveTo 移动到指定位置
func (a *App) MotionMoveTo(id string, axis string, position float64) GenericResponse {
	if a.appContext == nil || a.appContext.MotionManager == nil {
		return GenericResponse{Success: false, Error: "运动管理器未初始化"}
	}
	if err := a.appContext.MotionManager.MoveTo(id, types.MotionAxisName(axis), position); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// MotionMoveBy 相对移动
func (a *App) MotionMoveBy(id string, axis string, delta float64) GenericResponse {
	if a.appContext == nil || a.appContext.MotionManager == nil {
		return GenericResponse{Success: false, Error: "运动管理器未初始化"}
	}
	if err := a.appContext.MotionManager.MoveBy(id, types.MotionAxisName(axis), delta); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// MotionJog 点动控制
func (a *App) MotionJog(id string, axis string, velocity float64) GenericResponse {
	if a.appContext == nil || a.appContext.MotionManager == nil {
		return GenericResponse{Success: false, Error: "运动管理器未初始化"}
	}
	if err := a.appContext.MotionManager.Jog(id, types.MotionAxisName(axis), velocity); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// MotionDefinePosition 定义当前位置
func (a *App) MotionDefinePosition(id string, axis string, position float64) GenericResponse {
	if a.appContext == nil || a.appContext.MotionManager == nil {
		return GenericResponse{Success: false, Error: "运动管理器未初始化"}
	}
	if err := a.appContext.MotionManager.DefinePosition(id, types.MotionAxisName(axis), position); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// ==================== 校准 API ====================

// CalibrationStart 开始校准
func (a *App) CalibrationStart(config types.CalibrationConfig) GenericResponse {
	if a.appContext == nil || a.appContext.CalibrationMgr == nil {
		return GenericResponse{Success: false, Error: "校准管理器未初始化"}
	}
	if err := a.appContext.CalibrationMgr.Start(config); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// CalibrationStatus 获取校准状态
func (a *App) CalibrationStatus() types.CalibrationStatus {
	if a.appContext == nil || a.appContext.CalibrationMgr == nil {
		return types.CalibrationStatus{}
	}
	return a.appContext.CalibrationMgr.Status()
}

// CalibrationCollect 采集当前点
func (a *App) CalibrationCollect() GenericResponse {
	if a.appContext == nil || a.appContext.CalibrationMgr == nil {
		return GenericResponse{Success: false, Error: "校准管理器未初始化"}
	}
	if err := a.appContext.CalibrationMgr.CollectCurrentPoint(); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// CalibrationPause 暂停校准
func (a *App) CalibrationPause() GenericResponse {
	if a.appContext == nil || a.appContext.CalibrationMgr == nil {
		return GenericResponse{Success: false, Error: "校准管理器未初始化"}
	}
	if err := a.appContext.CalibrationMgr.Pause(); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// CalibrationResume 恢复校准
func (a *App) CalibrationResume() GenericResponse {
	if a.appContext == nil || a.appContext.CalibrationMgr == nil {
		return GenericResponse{Success: false, Error: "校准管理器未初始化"}
	}
	if err := a.appContext.CalibrationMgr.Resume(); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// CalibrationStop 停止校准
func (a *App) CalibrationStop() GenericResponse {
	if a.appContext == nil || a.appContext.CalibrationMgr == nil {
		return GenericResponse{Success: false, Error: "校准管理器未初始化"}
	}
	if err := a.appContext.CalibrationMgr.Stop(); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// CalibrationGetResult 获取校准结果
func (a *App) CalibrationGetResult(taskID string) (types.CalibrationStatus, bool) {
	if a.appContext == nil || a.appContext.CalibrationMgr == nil {
		return types.CalibrationStatus{}, false
	}
	return a.appContext.CalibrationMgr.GetResult(taskID)
}

// ==================== 存储 API ====================

// StorageGetStatus 获取存储记录器状态
func (a *App) StorageGetStatus() wind_usecase.StorageRecordingStatus {
	if a.appContext == nil || a.appContext.StorageRecorder == nil {
		return wind_usecase.StorageRecordingStatus{}
	}
	return a.appContext.StorageRecorder.Status()
}

// StorageStartRecording 开始记录
func (a *App) StorageStartRecording(outputDir string, filePrefix string) GenericResponse {
	if a.appContext == nil || a.appContext.StorageRecorder == nil {
		return GenericResponse{Success: false, Error: "存储记录器未初始化"}
	}
	cfg := wind_usecase.StorageRecordingConfig{
		OutputDir:  outputDir,
		FilePrefix: filePrefix,
	}
	if err := a.appContext.StorageRecorder.Start(cfg); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// StorageStopRecording 停止记录
func (a *App) StorageStopRecording() GenericResponse {
	if a.appContext == nil || a.appContext.StorageRecorder == nil {
		return GenericResponse{Success: false, Error: "存储记录器未初始化"}
	}
	if err := a.appContext.StorageRecorder.Stop(); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// ==================== 报告 API ====================

// ReportGetStatus 获取报告管理器状态
func (a *App) ReportGetStatus() wind_usecase.ReportStatus {
	if a.appContext == nil || a.appContext.ReportManager == nil {
		return wind_usecase.ReportStatus{}
	}
	return a.appContext.ReportManager.Status()
}
