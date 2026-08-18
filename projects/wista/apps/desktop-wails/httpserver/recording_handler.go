// Package httpserver recording_handler：RecordingService 的 HTTP 包装。
//
// 路由约定：
//   - POST /api/recording/start   开始录制，body: {"outputDir":"...","filePrefix":"..."}
//   - POST /api/recording/stop    停止录制
//   - GET  /api/recording/status  查询当前录制状态
//
// 注意：PickDirectory 在 Win7 分支返回 ErrDialogNotSupported，
// 目录选择改由 Electron 主进程通过 IPC 处理，前端拿到路径后调用 /start。
package httpserver

import "net/http"

// handleRecordingStart POST /api/recording/start
// 启动录制后立即广播一次状态（由 RecordingService.StartRecording 内部完成）。
func (s *Server) handleRecordingStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		OutputDir  string `json:"outputDir"`
		FilePrefix string `json:"filePrefix"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.OutputDir == "" {
		writeError(w, http.StatusBadRequest, "missing outputDir")
		return
	}
	if err := s.recording.StartRecording(body.OutputDir, body.FilePrefix); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, nil)
}

// handleRecordingStop POST /api/recording/stop
// 停止录制并 flush 文件，立即广播一次最终状态。
func (s *Server) handleRecordingStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.recording.StopRecording(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, nil)
}

// handleRecordingStatus GET /api/recording/status
// 返回当前 RecordingSession，包含已写入帧数、累计丢帧数等指标。
func (s *Server) handleRecordingStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeOK(w, s.recording.GetRecordingStatus())
}
