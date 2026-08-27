package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"cal1604/internal/domain"
	apperrors "cal1604/internal/errors"
)

type calibrationSetDevicesRequest struct {
	MeasureDeviceID  string   `json:"measureDeviceId"`
	MeasureDeviceIDs []string `json:"measureDeviceIds"`
	PressureDeviceID string   `json:"pressureDeviceId"`
}

type setConfigRequest struct {
	Channels       []int   `json:"channels"`
	PressurePoints int     `json:"pressurePoints"`
	AverageCount   int     `json:"averageCount"`
	MinPressure    float64 `json:"minPressure"`
	MaxPressure    float64 `json:"maxPressure"`
	StableWaitMs   int     `json:"stableWaitMs"`
	ControlMode    domain.ControlMode  `json:"controlMode,omitempty"`
	PressureMode   domain.PressureMode `json:"pressureMode,omitempty"`
	Precision      int     `json:"precision,omitempty"`
	PrecisionLevel float64 `json:"precisionLevel,omitempty"`
}

type setChannelsRequest struct {
	Channels []int `json:"channels"`
}

type pointIndexRequest struct {
	PointIndex int `json:"pointIndex"`
}

type channelsResponse struct {
	Channels []int `json:"channels"`
}

type alarmDecisionRequest struct {
	Decision string `json:"decision"`
}

func (s *apiServer) calibrationSetDevicesHandler(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[calibrationSetDevicesRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}

	measureDevIDs := req.MeasureDeviceIDs
	if len(measureDevIDs) == 0 && req.MeasureDeviceID != "" {
		measureDevIDs = []string{req.MeasureDeviceID}
	}
	if len(measureDevIDs) == 0 {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	if err := s.calibrationService.SetMeasureDevices(measureDevIDs, req.PressureDeviceID); err != nil {
		writeError(w, err)
		return
	}
	// 绑定成功后记录设备集合，供下次启动恢复勾选。
	s.persistLastDevices(measureDevIDs, req.PressureDeviceID)

	writeSuccess(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *apiServer) calibrationSetConfigHandler(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[setConfigRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}

	config := domain.WorkflowConfig{
		Channels:       req.Channels,
		PointCount:     req.PressurePoints,
		AverageCount:   req.AverageCount,
		MinPressure:    req.MinPressure,
		MaxPressure:    req.MaxPressure,
		StableWaitMs:   req.StableWaitMs,
		ControlMode:    req.ControlMode,
		PressureMode:   req.PressureMode,
		Precision:      req.Precision,
		PrecisionLevel: req.PrecisionLevel,
	}
	s.calibrationService.SetConfig(config)

	writeSuccess(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *apiServer) calibrationSetChannelsHandler(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[setChannelsRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}

	s.calibrationService.SetChannels(req.Channels)
	writeSuccess(w, http.StatusOK, channelsResponse(req))
}

func (s *apiServer) calibrationGetChannelsHandler(w http.ResponseWriter, _ *http.Request) {
	channels := s.calibrationService.GetChannels()
	writeSuccess(w, http.StatusOK, channelsResponse{Channels: channels})
}

func (s *apiServer) calibrationGeneratePointsHandler(w http.ResponseWriter, _ *http.Request) {
	points, err := s.calibrationService.GeneratePressurePoints()
	if err != nil {
		writeError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, points)
}

func (s *apiServer) calibrationGetPointsHandler(w http.ResponseWriter, _ *http.Request) {
	points := s.calibrationService.GetPressurePoints()
	writeSuccess(w, http.StatusOK, points)
}

func (s *apiServer) calibrationPressurizeHandler(w http.ResponseWriter, r *http.Request) {
	pointIndex, ok := decodePointIndexRequest(r, w)
	if !ok {
		return
	}

	if err := s.calibrationService.Pressurize(r.Context(), pointIndex); err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]string{"status": "pressurizing"})
}

func (s *apiServer) calibrationCollectHandler(w http.ResponseWriter, r *http.Request) {
	pointIndex, ok := decodePointIndexRequest(r, w)
	if !ok {
		return
	}

	// devices 为设备维度完整结果（deviceID -> 通道数据）；
	// data 保留首个绑定设备数据，兼容旧前端单设备字段。
	devices, err := s.calibrationService.CollectDevices(r.Context(), pointIndex)
	if err != nil {
		writeError(w, err)
		return
	}

	var data []float64
	if ids := s.calibrationService.MeasureDeviceIDs(); len(ids) > 0 {
		data = devices[ids[0]]
	}

	writeSuccess(w, http.StatusOK, map[string]any{"data": data, "devices": devices})
}

func (s *apiServer) calibrationFitHandler(w http.ResponseWriter, r *http.Request) {
	result, err := s.calibrationService.Fit(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, result)
}

func (s *apiServer) calibrationResolveAlarmHandler(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[alarmDecisionRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}

	if err := s.calibrationService.ResolveAlarm(req.Decision); err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]string{"status": "ok"})
}

// skipDeviceRequest 跳过指定设备的请求体。
type skipDeviceRequest struct {
	DeviceID string `json:"deviceId"`
	Reason   string `json:"reason"`
}

// calibrationSkipDeviceHandler 用户选择永久跳过指定计量设备。
func (s *apiServer) calibrationSkipDeviceHandler(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[skipDeviceRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}

	if req.DeviceID == "" {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	if err := s.calibrationService.ResolveSkipDevice(req.DeviceID, req.Reason); err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *apiServer) calibrationRetryPointHandler(w http.ResponseWriter, r *http.Request) {
	pointIndex, ok := decodePointIndexRequest(r, w)
	if !ok {
		return
	}

	if err := s.calibrationService.RetryPoint(r.Context(), pointIndex); err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]string{"status": "ok"})
}

func decodePointIndexRequest(r *http.Request, w http.ResponseWriter) (int, bool) {
	req, err := decodeJSON[pointIndexRequest](r)
	if err != nil {
		// 尝试从查询参数读取
		idxStr := r.URL.Query().Get("pointIndex")
		if idxStr != "" {
			idx, err := strconv.Atoi(idxStr)
			if err != nil || idx < 1 {
				writeError(w, apperrors.ErrInvalidArgument)
				return 0, false
			}
			return idx, true
		}
		writeError(w, apperrors.ErrInvalidArgument)
		return 0, false
	}

	if req.PointIndex < 1 {
		writeError(w, apperrors.ErrInvalidArgument)
		return 0, false
	}

	return req.PointIndex, true
}

func (s *apiServer) calibrationGetAlarmConfigHandler(w http.ResponseWriter, _ *http.Request) {
	config := s.calibrationService.GetAlarmConfig()
	writeSuccess(w, http.StatusOK, config)
}

func (s *apiServer) calibrationSetAlarmConfigHandler(w http.ResponseWriter, r *http.Request) {
	config, err := decodeJSON[domain.AlarmConfig](r)
	if err != nil {
		writeError(w, err)
		return
	}

	s.calibrationService.SetAlarmConfig(config)
	writeSuccess(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *apiServer) calibrationGetSessionHandler(w http.ResponseWriter, _ *http.Request) {
	session := s.calibrationService.GetCalibrationSession()
	writeSuccess(w, http.StatusOK, session)
}

// calibrationUpdateTargetPressureHandler 更新指定压力点的目标压力值。
func (s *apiServer) calibrationUpdateTargetPressureHandler(w http.ResponseWriter, r *http.Request) {
	pointIndexStr := r.PathValue("index")
	if pointIndexStr == "" {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}
	pointIndex, err := strconv.Atoi(pointIndexStr)
	if err != nil || pointIndex < 1 {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	req, decodeErr := decodeJSON[updateTargetPressureRequest](r)
	if decodeErr != nil {
		writeError(w, decodeErr)
		return
	}

	if err := s.calibrationService.UpdatePointTargetPressure(pointIndex, req.TargetPressure); err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]string{"status": "ok"})
}

type updateTargetPressureRequest struct {
	TargetPressure float64 `json:"targetPressure"`
}

type manualPressurizeRequest struct {
	TargetPressure float64 `json:"targetPressure"`
}

func (s *apiServer) calibrationManualPressurizeHandler(w http.ResponseWriter, r *http.Request) {
	var req manualPressurizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	if err := s.calibrationService.ManualPressurize(r.Context(), req.TargetPressure); err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *apiServer) calibrationManualCollectHandler(w http.ResponseWriter, r *http.Request) {
	data, err := s.calibrationService.ManualCollect(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{"data": data})
}
