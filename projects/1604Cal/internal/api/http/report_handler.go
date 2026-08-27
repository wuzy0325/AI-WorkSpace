package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	apperrors "cal1604/internal/errors"
	"cal1604/internal/report"
)

type reportTemplateSelection struct {
	Filename string `json:"filename"`
}

func (s *apiServer) reportTemplateSelectHandler(w http.ResponseWriter, r *http.Request) {
	pointsText := strings.TrimSpace(r.URL.Query().Get("points"))
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
	if pointsText == "" || mode == "" {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	points, err := strconv.Atoi(pointsText)
	if err != nil {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	filename, err := report.SelectTemplate(points, mode)
	if err != nil {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	writeSuccess(w, http.StatusOK, reportTemplateSelection{Filename: filename})
}

type exportReportRequest struct {
	OutputPath string `json:"outputPath"`
}

// exportReportHandler 根据当前校准会话导出校准报告。
func (s *apiServer) exportReportHandler(w http.ResponseWriter, r *http.Request) {
	var req exportReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}
	if req.OutputPath == "" {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	session := s.calibrationService.GetCalibrationSession()
	if session == nil {
		writeError(w, apperrors.ErrNoActiveSession)
		return
	}

	// 从计量设备读取真实压力单位，保证导出文档单位与硬件一致（不再写死 kPa）。
	unit := readSessionUnit(r.Context(), s)

	// 多设备时每台设备生成独立报告文件，返回完整路径列表；
	// path 保留首个文件路径，兼容旧前端单文件字段。
	paths, err := s.reportService.ExportReport(r.Context(), session, req.OutputPath, unit)
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

// listTemplatesHandler 返回可用的报告模板列表。
func (s *apiServer) listTemplatesHandler(w http.ResponseWriter, _ *http.Request) {
	templates, err := s.reportService.GetTemplates()
	if err != nil {
		writeError(w, err)
		return
	}

	if templates == nil {
		templates = []report.ReportTemplate{}
	}
	writeSuccess(w, http.StatusOK, map[string]any{"templates": templates})
}

// readSessionUnit 读取当前会话首个计量设备的真实压力单位。
// 导出报告时用于替换写死的单位，保证文档单位与硬件一致。
// 读取失败或无绑定设备时返回空字符串，由导出层回退到默认值。
func readSessionUnit(ctx context.Context, s *apiServer) string {
	token := s.sessionService.Token()
	if token.BoundBy == "" {
		return ""
	}
	unit, err := s.sessionService.ReadMeasureUnit(ctx, token)
	if err != nil || unit == "" {
		return ""
	}
	return unit
}
