// Package httpserver log_handler：App 中日志相关方法的 HTTP 包装。
//
// 路由约定：
//   - POST /api/log/start   开始将日志写入文件，body: {"outputDir":"...","prefix":"..."}
//   - POST /api/log/stop    停止日志文件写入
//   - GET  /api/log/state   查询日志文件当前状态
//
// 注意：PickDirectory 在 Win7 分支返回 ErrDialogNotSupported，
// 目录选择改由 Electron 主进程通过 IPC 处理，前端拿到路径后调用 /start。
package httpserver

import "net/http"

// handleLogStart POST /api/log/start
// 开启日志文件写入；后续所有 EmitLog 调用会同时写入文件 + 推送前端。
func (s *Server) handleLogStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		OutputDir string `json:"outputDir"`
		Prefix    string `json:"prefix"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.OutputDir == "" {
		writeError(w, http.StatusBadRequest, "missing outputDir")
		return
	}
	if err := s.app.StartLogFile(body.OutputDir, body.Prefix); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, nil)
}

// handleLogStop POST /api/log/stop
// 停止日志文件写入，已写入内容会被 flush 关闭。
func (s *Server) handleLogStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.app.StopLogFile(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, nil)
}

// handleLogState GET /api/log/state
// 返回日志文件当前状态（是否激活、输出目录）。
func (s *Server) handleLogState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeOK(w, s.app.GetLogFileState())
}
