// Package httpserver 把 backend 3 个 Service 包装成 HTTP handler + WebSocket 事件流。
//
// 设计要点：
//   - RPC 调用通过 net/http 路由，POST/GET 请求体用 JSON
//   - 事件流（daq:payload / daq:log / daq:recording-*）通过 WebSocket 推送
//   - 不依赖 Wails，纯标准库 + github.com/coder/websocket
//
// 入口：
//   - NewWSHub()：构造 WebSocket hub
//   - WSHub.Run(ctx)：启动 hub 主循环（独立 goroutine）
//   - RegisterHandlers(mux, device, recording, log, hub, wsHub)：注册全部路由
package httpserver

import (
	"encoding/json"
	"net/http"

	"daq-t1603/backend"
	"daq-t1603/core"
)

// Server 持有所有依赖，供 handler 共享。
// wsHub 字段保留供后续扩展（如 /api/health 内部汇报客户端数）。
type Server struct {
	device    *backend.DeviceService
	recording *backend.RecordingService
	log       *backend.LogService
	hub       *core.Hub
	wsHub     *WSHub
}

// NewServer 构造 Server 实例。
func NewServer(
	device *backend.DeviceService,
	recording *backend.RecordingService,
	log *backend.LogService,
	hub *core.Hub,
	wsHub *WSHub,
) *Server {
	return &Server{device: device, recording: recording, log: log, hub: hub, wsHub: wsHub}
}

// RegisterHandlers 在给定的 mux 上注册所有 HTTP endpoint。
// 路由约定：
//   - /api/health              健康检查（Electron 主进程就绪探测）
//   - /api/device/*            DeviceService（详见 device_handler.go）
//   - /api/recording/*         RecordingService（详见 recording_handler.go）
//   - /api/log/*               LogService（详见 log_handler.go）
//   - /ws                      WebSocket 事件流
//
// 路由冲突说明：Go 1.20 ServeMux 不支持路径参数与 method 区分，
// /api/device/profile（POST 精确匹配）与 /api/device/profile/（DELETE 子树匹配）
// 通过尾斜杠差异化分流，详见各 handler 内的 method 校验。
func RegisterHandlers(
	mux *http.ServeMux,
	device *backend.DeviceService,
	recording *backend.RecordingService,
	log *backend.LogService,
	hub *core.Hub,
	wsHub *WSHub,
) {
	s := NewServer(device, recording, log, hub, wsHub)

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
	mux.HandleFunc("/api/device/status/", s.handleDeviceStatus)
	mux.HandleFunc("/api/device/apply-config", s.handleDeviceApplyConfig)

	// Recording endpoints（详见 recording_handler.go）
	mux.HandleFunc("/api/recording/start", s.handleRecordingStart)
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
