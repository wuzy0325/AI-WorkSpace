// Package httpserver recording_handler：App 中录制相关方法的 HTTP 包装。
//
// 路由约定：
//   - POST /api/recording/start               开始录制，body: {"outputDir":"...","filePrefix":"..."}
//   - POST /api/recording/start-with-config   开始录制（带完整滚动+停止条件配置）
//   - POST /api/recording/stop                停止录制
//   - GET  /api/recording/status              查询当前录制状态
//
// 与 wista 的差异：
//   - 多了 start-with-config 端点，wispa 的录制支持 FileRotation + StopConditions
//   - start 端点仅传 (outputDir, filePrefix)，等价于 start-with-config 传空配置
//
// 注意：PickDirectory 在 Win7 分支返回 ErrDialogNotSupported，
// 目录选择改由 Electron 主进程通过 IPC 处理，前端拿到路径后调用 /start。
package httpserver

import (
	"net/http"

	"wispa/core"
)

// recordingStartRequest 是 /api/recording/start 的请求体。
// 仅传必要参数，rotation/stopConditions 用零值（不限制）。
type recordingStartRequest struct {
	OutputDir  string `json:"outputDir"`
	FilePrefix string `json:"filePrefix"`
}

// recordingStartWithConfigRequest 是 /api/recording/start-with-config 的请求体。
// 直接复用 core.FileRotation + core.StopConditions 类型，
// 避免 handler 与 core 之间的重复类型定义；JSON tag 由 core 类型自带。
type recordingStartWithConfigRequest struct {
	OutputDir      string               `json:"outputDir"`
	FilePrefix     string               `json:"filePrefix"`
	Rotation       core.FileRotation    `json:"rotation"`
	StopConditions core.StopConditions  `json:"stopConditions"`
}

// handleRecordingStart POST /api/recording/start
// 启动录制（不带滚动/停止条件，等价于 start-with-config 传零值）。
// 启动后立即广播一次状态（由 App.StartRecording 内部完成）。
//
// 适用于简单录制场景；复杂场景（需要按大小/时长滚动、自动停止）请用
// /api/recording/start-with-config。
func (s *Server) handleRecordingStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body recordingStartRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.OutputDir == "" {
		writeError(w, http.StatusBadRequest, "missing outputDir")
		return
	}
	if err := s.app.StartRecording(body.OutputDir, body.FilePrefix); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, nil)
}

// handleRecordingStartWithConfig POST /api/recording/start-with-config
// 启动录制并透传 FileRotation + StopConditions 配置。
//
// 请求体示例：
//
//	{
//	  "outputDir": "C:/data",
//	  "filePrefix": "exp01",
//	  "rotation": { "maxSizeBytes": 104857600 },  // 单文件 100MB 滚动
//	  "stopConditions": { "maxDurationMs": 3600000 } // 1 小时自动停止
//	}
//
// 启动后立即广播一次状态（由 App.StartRecordingWithConfig 内部完成）。
func (s *Server) handleRecordingStartWithConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body recordingStartWithConfigRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.OutputDir == "" {
		writeError(w, http.StatusBadRequest, "missing outputDir")
		return
	}
	if err := s.app.StartRecordingWithConfig(body.OutputDir, body.FilePrefix, body.Rotation, body.StopConditions); err != nil {
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
	if err := s.app.StopRecording(); err != nil {
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
	writeOK(w, s.app.GetRecordingStatus())
}
