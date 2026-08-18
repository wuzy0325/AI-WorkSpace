// Package httpserver 把 backend.App 包装成 HTTP handler + WebSocket 事件流。
//
// 设计要点：
//   - RPC 调用通过 net/http 路由，POST/GET 请求体用 JSON
//   - 事件流（daq:log / daq:recording-* / daq:device-state）通过 WebSocket 推送
//   - 不依赖 Wails，纯标准库 + nhooyr.io/websocket
//
// 与 wista httpserver 的差异：
//   - Server 持有 *backend.App 单体（wista 持有 3 个 Service）
//   - 多了 latest-snapshot 端点（wispa 前端 500ms 轮询读取快照）
//   - 多了 start-with-config 端点（wispa 录制带 FileRotation + StopConditions）
//   - WSHub.Emit 支持多参数事件（daq:device-state 双参数 id + state）
//
// 入口：
//   - NewWSHub()：构造 WebSocket hub
//   - WSHub.Run(ctx)：启动 hub 主循环（独立 goroutine）
//   - RegisterHandlers(mux, app, hub, wsHub)：注册全部路由
package httpserver

import (
	"encoding/json"
	"net/http"

	"wispa/backend"
	"wispa/core"
)

// Server 持有所有依赖，供 handler 共享。
// wsHub 字段保留供后续扩展（如 /api/health 内部汇报客户端数）。
type Server struct {
	app   *backend.App
	hub   *core.Hub
	wsHub *WSHub
}

// NewServer 构造 Server 实例。
func NewServer(app *backend.App, hub *core.Hub, wsHub *WSHub) *Server {
	return &Server{app: app, hub: hub, wsHub: wsHub}
}

// RegisterHandlers 在给定的 mux 上注册所有 HTTP endpoint。
// 路由约定：
//   - /api/health                       健康检查（Electron 主进程就绪探测）
//   - /api/device/*                     DeviceService（详见 device_handler.go）
//   - /api/recording/*                  RecordingService（详见 recording_handler.go）
//   - /api/log/*                        LogService（详见 log_handler.go）
//   - /ws                               WebSocket 事件流
//
// 路由冲突说明：Go 1.20 ServeMux 不支持路径参数与 method 区分，
// /api/device/profile（POST 精确匹配）与 /api/device/profile/（DELETE 子树匹配）
// 通过尾斜杠差异化分流，详见各 handler 内的 method 校验。
func RegisterHandlers(
	mux *http.ServeMux,
	app *backend.App,
	hub *core.Hub,
	wsHub *WSHub,
) {
	s := NewServer(app, hub, wsHub)

	// 健康检查
	mux.HandleFunc("/api/health", s.handleHealth)

	// Device endpoints（详见 device_handler.go）
	mux.HandleFunc("/api/device/scan", s.handleDeviceScan)
	mux.HandleFunc("/api/device/profiles", s.handleDeviceProfiles)
	mux.HandleFunc("/api/device/profile", s.handleDeviceProfileUpsert)
	mux.HandleFunc("/api/device/profile/", s.handleDeviceProfileDelete)
	mux.HandleFunc("/api/device/connect", s.handleDeviceConnect)
	mux.HandleFunc("/api/device/disconnect", s.handleDeviceDisconnect)
	mux.HandleFunc("/api/device/start", s.handleDeviceStart)
	mux.HandleFunc("/api/device/stop", s.handleDeviceStop)
	mux.HandleFunc("/api/device/zero-calibration", s.handleDeviceZeroCalibration)
	mux.HandleFunc("/api/device/status/", s.handleDeviceStatus)
	mux.HandleFunc("/api/device/apply-config", s.handleDeviceApplyConfig)
	// wispa 专属：前端 500ms 轮询读取最新快照
	mux.HandleFunc("/api/device/latest-snapshots", s.handleDeviceLatestSnapshots)
	mux.HandleFunc("/api/device/latest-snapshot/", s.handleDeviceLatestSnapshot)

	// Recording endpoints（详见 recording_handler.go）
	mux.HandleFunc("/api/recording/start", s.handleRecordingStart)
	mux.HandleFunc("/api/recording/start-with-config", s.handleRecordingStartWithConfig)
	mux.HandleFunc("/api/recording/stop", s.handleRecordingStop)
	mux.HandleFunc("/api/recording/status", s.handleRecordingStatus)

	// Log endpoints（详见 log_handler.go）
	mux.HandleFunc("/api/log/start", s.handleLogStart)
	mux.HandleFunc("/api/log/stop", s.handleLogStop)
	mux.HandleFunc("/api/log/state", s.handleLogState)

	// WebSocket 事件流
	if wsHub != nil {
		mux.HandleFunc("/ws", wsHub.ServeWS)
	}
}

// handleHealth 健康检查 endpoint，返回 JSON {"status":"ok"}。
// Electron 主进程启动后轮询此 endpoint，确认 Go 子进程已就绪。
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
