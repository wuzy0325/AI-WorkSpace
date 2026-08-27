package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"cal1604/internal/config"
	"cal1604/internal/domain"
	apperrors "cal1604/internal/errors"
)

type deviceConnectConfigPayload struct {
	ConnectAttemptTimeoutMs    int `json:"connectAttemptTimeoutMs"`
	ConnectMaxAttempts         int `json:"connectMaxAttempts"`
	ConnectInitialBackoffMs    int `json:"connectInitialBackoffMs"`
	ConnectMaxBackoffMs        int `json:"connectMaxBackoffMs"`
	DisconnectAttemptTimeoutMs int `json:"disconnectAttemptTimeoutMs"`
	DisconnectMaxAttempts      int `json:"disconnectMaxAttempts"`
	DisconnectInitialBackoffMs int `json:"disconnectInitialBackoffMs"`
	DisconnectMaxBackoffMs     int `json:"disconnectMaxBackoffMs"`
}

// deviceConnectConfigHandler 返回当前生效的连接可靠性配置。
// 该接口用于前端设备面板可视化 timeout/retry 策略，便于现场排障与参数核对。
func (s *apiServer) deviceConnectConfigHandler(w http.ResponseWriter, _ *http.Request) {
	payload := deviceConnectConfigPayload{
		ConnectAttemptTimeoutMs:    durationToMilliseconds(s.connectConfig.ConnectAttemptTimeout),
		ConnectMaxAttempts:         s.connectConfig.ConnectMaxAttempts,
		ConnectInitialBackoffMs:    durationToMilliseconds(s.connectConfig.ConnectInitialBackoff),
		ConnectMaxBackoffMs:        durationToMilliseconds(s.connectConfig.ConnectMaxBackoff),
		DisconnectAttemptTimeoutMs: durationToMilliseconds(s.connectConfig.DisconnectAttemptTimeout),
		DisconnectMaxAttempts:      s.connectConfig.DisconnectMaxAttempts,
		DisconnectInitialBackoffMs: durationToMilliseconds(s.connectConfig.DisconnectInitialBackoff),
		DisconnectMaxBackoffMs:     durationToMilliseconds(s.connectConfig.DisconnectMaxBackoff),
	}

	writeSuccess(w, http.StatusOK, payload)
}

func durationToMilliseconds(value time.Duration) int {
	if value <= 0 {
		return 0
	}
	return int(value / time.Millisecond)
}

// gatesConfigPayload 描述启动门禁开关，用于前端与后端保持开关同源。
type gatesConfigPayload struct {
	EnforceValveCalibrationGate bool `json:"enforceValveCalibrationGate"`
}

// gatesConfigHandler 返回当前生效的启动门禁开关。
// 前端在 bootstrap 阶段拉取该接口，用于决定计量/标定启动前是否需要阀门=校准。
func (s *apiServer) gatesConfigHandler(w http.ResponseWriter, _ *http.Request) {
	writeSuccess(w, http.StatusOK, gatesConfigPayload{
		EnforceValveCalibrationGate: s.calibrationConfig.EnforceValveCalibrationGate,
	})
}

// calibrationReadConfigHandler 返回当前校准参数。
func (s *apiServer) calibrationReadConfigHandler(w http.ResponseWriter, _ *http.Request) {
	if s.appConfig != nil {
		writeSuccess(w, http.StatusOK, s.appConfig.CalibrationParams)
		return
	}
	writeSuccess(w, http.StatusOK, config.Default().CalibrationParams)
}

// calibrationSaveConfigHandler 更新校准参数并持久化到配置文件。
func (s *apiServer) calibrationSaveConfigHandler(w http.ResponseWriter, r *http.Request) {
	var params config.CalibrationParamsConfig
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}
	if s.appConfig != nil {
		s.appConfig.CalibrationParams = params
		s.persistConfig()
	}
	s.calibrationService.SetConfig(calibrationConfigFromParams(params))
	writeSuccess(w, http.StatusOK, map[string]string{"status": "ok"})
}

// measurementReadConfigHandler 返回当前计量参数。
func (s *apiServer) measurementReadConfigHandler(w http.ResponseWriter, _ *http.Request) {
	if s.appConfig != nil {
		writeSuccess(w, http.StatusOK, s.appConfig.MeasurementParams)
		return
	}
	writeSuccess(w, http.StatusOK, config.Default().MeasurementParams)
}

// measurementSaveConfigHandler 更新计量参数并持久化到配置文件。
func (s *apiServer) measurementSaveConfigHandler(w http.ResponseWriter, r *http.Request) {
	var params config.MeasurementParamsConfig
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}
	if err := validateMeasurementParams(params); err != nil {
		writeError(w, err)
		return
	}

	if s.appConfig != nil {
		s.appConfig.MeasurementParams = params
		s.persistConfig()
	}

	s.measurementService.SetConfig(measurementConfigFromParams(params))

	writeSuccess(w, http.StatusOK, map[string]string{"status": "ok"})
}

// alarmReadConfigHandler 返回当前报警配置。
func (s *apiServer) alarmReadConfigHandler(w http.ResponseWriter, _ *http.Request) {
	if s.appConfig != nil {
		writeSuccess(w, http.StatusOK, s.appConfig.Alarm)
		return
	}
	writeSuccess(w, http.StatusOK, config.Default().Alarm)
}

// alarmSaveConfigHandler 更新报警配置并持久化到配置文件。
func (s *apiServer) alarmSaveConfigHandler(w http.ResponseWriter, r *http.Request) {
	var alarmCfg struct {
		Enabled            bool    `json:"enabled"`
		PrecisionThreshold float64 `json:"precisionThreshold"`
		SoundEnabled       bool    `json:"soundEnabled"`
		ConfirmOnAlarm     bool    `json:"confirmOnAlarm"`
		EnabledChannels    []int   `json:"enabledChannels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&alarmCfg); err != nil {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}
	cfg := domain.AlarmConfig{
		Enabled:            alarmCfg.Enabled,
		PrecisionThreshold: alarmCfg.PrecisionThreshold,
		SoundEnabled:       alarmCfg.SoundEnabled,
		ConfirmOnAlarm:     alarmCfg.ConfirmOnAlarm,
		EnabledChannels:    alarmCfg.EnabledChannels,
	}
	if s.appConfig != nil {
		s.appConfig.Alarm = cfg
		s.persistConfig()
	}
	s.calibrationService.SetAlarmConfig(cfg)
	s.measurementService.SetAlarmConfig(cfg)
	writeSuccess(w, http.StatusOK, map[string]string{"status": "ok"})
}

// persistConfig 将当前配置持久化到文件（如果配置了路径）。
func (s *apiServer) persistConfig() {
	if s.appConfig != nil && s.configPath != "" {
		_ = s.appConfig.SaveToFile(s.configPath)
	}
}

// lastDevicesReadHandler 返回上次成功绑定的设备 ID 集合。
// 前端页面加载时拉取该接口，恢复上次的设备勾选（多设备按勾选顺序）。
func (s *apiServer) lastDevicesReadHandler(w http.ResponseWriter, _ *http.Request) {
	if s.appConfig != nil {
		writeSuccess(w, http.StatusOK, s.appConfig.LastDevices)
		return
	}
	writeSuccess(w, http.StatusOK, config.Default().LastDevices)
}

// persistLastDevices 记录本次成功绑定的设备集合并落盘，供下次启动恢复勾选。
// 只在绑定成功后调用；绑定失败不覆盖上次记录。
func (s *apiServer) persistLastDevices(measureDevIDs []string, pressureDevID string) {
	if s.appConfig == nil {
		return
	}
	s.appConfig.LastDevices = config.LastDevicesConfig{
		PressureDeviceID: pressureDevID,
		MeasureDeviceIDs: append([]string(nil), measureDevIDs...),
	}
	s.persistConfig()
}

// calibrationConfigFromParams 将持久化参数转换为校准服务配置。
func calibrationConfigFromParams(params config.CalibrationParamsConfig) domain.WorkflowConfig {
	return domain.WorkflowConfig{
		MinPressure:    params.MinPressure,
		MaxPressure:    params.MaxPressure,
		PointCount:     params.PointCount,
		Precision:      params.Precision,
		AverageCount:   params.AverageCount,
		StableWaitMs:   params.StableDurationMs,
		PrecisionLevel: params.PrecisionLevel,
		PressureMode:   params.PressureMode,
		ControlMode:    params.ControlMode,
	}
}

// measurementConfigFromParams 将持久化参数转换为计量服务配置。
func measurementConfigFromParams(params config.MeasurementParamsConfig) domain.WorkflowConfig {
	return domain.WorkflowConfig{
		MinPressure:    params.MinPressure,
		MaxPressure:    params.MaxPressure,
		PointCount:     params.PointCount,
		Precision:      params.Precision,
		AverageCount:   params.AverageCount,
		StableWaitMs:   params.StableDurationMs,
		PrecisionLevel: params.PrecisionLevel,
		PressureMode:   params.PressureMode,
		ControlMode:    params.ControlMode,
		CustomPoints:   params.CustomPoints,
	}
}

func validateMeasurementParams(params config.MeasurementParamsConfig) error {
	if params.PointCount < 2 {
		return fmt.Errorf("%w: pointCount must be at least 2", apperrors.ErrInvalidArgument)
	}
	if params.Precision < 0 {
		return fmt.Errorf("%w: precision must be non-negative", apperrors.ErrInvalidArgument)
	}
	if params.Precision > 6 {
		return fmt.Errorf("%w: precision must be at most 6", apperrors.ErrInvalidArgument)
	}
	if params.AverageCount < 1 {
		return fmt.Errorf("%w: averageCount must be at least 1", apperrors.ErrInvalidArgument)
	}
	if params.StableDurationMs < 0 {
		return fmt.Errorf("%w: stableDurationMs must be non-negative", apperrors.ErrInvalidArgument)
	}
	if params.PressureMode != domain.PressureModeSingle && params.PressureMode != domain.PressureModeRoundTrip {
		return fmt.Errorf("%w: pressureMode must be single or roundTrip", apperrors.ErrInvalidArgument)
	}
	if params.ControlMode != domain.ControlModeAuto && params.ControlMode != domain.ControlModeManual {
		return fmt.Errorf("%w: controlMode must be auto or manual", apperrors.ErrInvalidArgument)
	}
	return nil
}
