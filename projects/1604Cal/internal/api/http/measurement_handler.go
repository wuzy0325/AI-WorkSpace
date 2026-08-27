package http

import (
	"encoding/json"
	"net/http"

	"cal1604/internal/application/measurement"
	"cal1604/internal/domain"
	apperrors "cal1604/internal/errors"
)

type measurementStartRequest struct {
	Channels []int `json:"channels"`
}

func (s *apiServer) measurementGeneratePointsHandler(w http.ResponseWriter, _ *http.Request) {
	points, err := s.measurementService.GeneratePressurePoints()
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, points)
}

func (s *apiServer) measurementPointsHandler(w http.ResponseWriter, _ *http.Request) {
	writeSuccess(w, http.StatusOK, s.measurementService.GetPoints())
}

func (s *apiServer) measurementStateHandler(w http.ResponseWriter, _ *http.Request) {
	writeSuccess(w, http.StatusOK, map[string]string{"state": string(s.measurementService.State())})
}

func (s *apiServer) measurementStartHandler(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[measurementStartRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}

	if len(req.Channels) == 0 {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	// 开始采集前以服务端当前配置重新生成测点，确保实际打压目标不受前端旧缓存影响。
	if _, err := s.measurementService.GeneratePressurePoints(); err != nil {
		writeError(w, err)
		return
	}

	// 创建工作流会话（校验点位、绑定设备），状态变为 ready
	if err := s.measurementService.StartWorkflow(r.Context(), req.Channels); err != nil {
		writeError(w, err)
		return
	}

	// 后台自动按点采集（逐点打压→稳定→采集），通过 StopAutoCollect 可控停止
	s.measurementService.StartAutoCollect()

	writeSuccess(w, http.StatusOK, map[string]string{"state": string(s.measurementService.State())})
}

func (s *apiServer) measurementPauseHandler(w http.ResponseWriter, _ *http.Request) {
	if err := s.measurementService.Pause(); err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]string{"state": "paused"})
}

func (s *apiServer) measurementStopHandler(w http.ResponseWriter, _ *http.Request) {
	if err := s.measurementService.Stop(); err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]string{"state": "idle"})
}

func (s *apiServer) measurementDataHandler(w http.ResponseWriter, _ *http.Request) {
	rows, total := s.measurementService.GetData()
	writeSuccess(w, http.StatusOK, map[string]any{"rows": rows, "total": total})
}

func (s *apiServer) measurementExportHandler(w http.ResponseWriter, r *http.Request) {
	var req exportReportRequest
	if r.Method == http.MethodPost {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if req.OutputPath == "" {
		req.OutputPath = r.URL.Query().Get("outputPath")
	}

	if req.OutputPath == "" {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	points := s.measurementService.GetPoints()
	config := s.measurementService.GetConfig()

	// 多设备时每台设备生成独立报告文件，返回完整路径列表；
	// path 保留首个文件路径，兼容旧前端单文件字段。
	// 读取计量设备真实压力单位，保证导出文档单位与硬件一致（不再写死 kPa）。
	unit := readSessionUnit(r.Context(), s)
	paths, err := s.reportService.ExportMeasurementReport(r.Context(), points, config, req.OutputPath, unit)
	if err != nil {
		writeError(w, err)
		return
	}

	path := req.OutputPath
	if len(paths) > 0 {
		path = paths[0]
	}
	writeSuccess(w, http.StatusOK, map[string]any{"status": "ok", "path": path, "paths": paths})
}

func (s *apiServer) measurementGetAlarmConfigHandler(w http.ResponseWriter, _ *http.Request) {
	cfg := s.measurementService.GetAlarmConfig()
	writeSuccess(w, http.StatusOK, cfg)
}

func (s *apiServer) measurementSetAlarmConfigHandler(w http.ResponseWriter, r *http.Request) {
	cfg, err := decodeJSON[domain.AlarmConfig](r)
	if err != nil {
		writeError(w, err)
		return
	}
	s.measurementService.SetAlarmConfig(cfg)
	writeSuccess(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *apiServer) measurementAlarmResolveHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Decision string `json:"decision"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	if err := s.measurementService.ResolveAlarm(req.Decision); err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]string{"status": "resolved"})
}

// measurementSkipDeviceHandler 用户选择永久跳过指定计量设备。
func (s *apiServer) measurementSkipDeviceHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceID string `json:"deviceId"`
		Reason   string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	if req.DeviceID == "" {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	if err := s.measurementService.ResolveSkipDevice(req.DeviceID, req.Reason); err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]string{"status": "skipped"})
}

func (s *apiServer) measurementAlarmPendingHandler(w http.ResponseWriter, _ *http.Request) {
	pending := s.measurementService.IsAlarmPending()
	var alarm *measurement.Alarm
	if pending {
		// 挂起时附带报警详情：页面刷新后 SSE 事件已错过，
		// 前端据此恢复报警弹窗/自动放行判断。
		alarm = s.measurementService.GetCurrentAlarm()
	}
	writeSuccess(w, http.StatusOK, map[string]any{"pending": pending, "alarm": alarm})
}

// measurementStabilityTimeoutPendingHandler 查询稳定超时是否挂起（页面刷新恢复用）。
func (s *apiServer) measurementStabilityTimeoutPendingHandler(w http.ResponseWriter, _ *http.Request) {
	pending, pointIndex := s.measurementService.GetStabilityTimeoutPending()
	writeSuccess(w, http.StatusOK, map[string]any{"pending": pending, "pointIndex": pointIndex})
}

func (s *apiServer) measurementAutoCollectHandler(w http.ResponseWriter, _ *http.Request) {
	s.measurementService.StartAutoCollect()

	writeSuccess(w, http.StatusOK, map[string]string{"state": string(s.measurementService.State())})
}

// measurementManualStartHandler 仅启动工作流（进入 ready 状态），不启动实时采样。
// 手动模式使用此端点，允许后续手动打压或直接采集。
func (s *apiServer) measurementManualStartHandler(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[measurementStartRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}

	if len(req.Channels) == 0 {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	if err := s.measurementService.StartWorkflow(r.Context(), req.Channels); err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]string{"state": string(s.measurementService.State())})
}

func (s *apiServer) measurementManualPressurizeHandler(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[struct {
		PointIndex int `json:"pointIndex"`
	}](r)
	if err != nil {
		writeError(w, err)
		return
	}

	if err := s.measurementService.ManualPressurize(r.Context(), req.PointIndex); err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]string{"state": string(s.measurementService.State())})
}

func (s *apiServer) measurementManualCollectHandler(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[struct {
		PointIndex int `json:"pointIndex"`
	}](r)
	if err != nil {
		writeError(w, err)
		return
	}

	if err := s.measurementService.ManualCollect(r.Context(), req.PointIndex); err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]string{"state": string(s.measurementService.State())})
}

// measurementStabilityTimeoutResolveHandler 接收前端用户对稳定超时的决定。
func (s *apiServer) measurementStabilityTimeoutResolveHandler(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[struct {
		Decision string `json:"decision"`
	}](r)
	if err != nil {
		writeError(w, err)
		return
	}

	s.measurementService.ResolveStabilityTimeout(req.Decision)
	writeSuccess(w, http.StatusOK, map[string]string{"status": "resolved"})
}
