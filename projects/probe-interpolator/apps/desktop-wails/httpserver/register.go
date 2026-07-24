// Package httpserver 把 backend.App 包装成 HTTP handler。
//
// 设计要点：
//   - RPC 调用通过 net/http 路由，POST/GET 请求体用 JSON
//   - 不依赖 Wails，纯标准库
//   - 无 WebSocket 事件流（探针插值是纯请求-响应模式，无实时推送需求）
//
// 与 daq-p1604 / daq-t1603 httpserver 的差异：
//   - 无 ws_hub.go / WebSocket 端点
//   - 无 device/recording/log handlers（无硬件设备、无录制、无日志文件）
//   - 多了 probe selector + 5/3/7 孔插值相关端点
//   - 文件选择对话框由 Electron IPC 处理，后端 LoadPrb/ImportCsv 接收文件路径参数
//
// 入口：
//   - RegisterHandlers(mux, app)：注册全部路由
package httpserver

import (
	"net/http"

	"probe-interpolator/backend"
)

// Server 持有所有依赖，供 handler 共享。
type Server struct {
	app *backend.App
}

// NewServer 构造 Server 实例。
func NewServer(app *backend.App) *Server {
	return &Server{app: app}
}

// RegisterHandlers 在给定的 mux 上注册所有 HTTP endpoint。
// 路由约定：
//   - /api/health                         健康检查（Electron 主进程就绪探测）
//   - /api/probe/*                        探针选择 + 5/3/7 孔插值（详见 probe_handler.go）
//
// 路由冲突说明：Go 1.20 ServeMux 不支持路径参数与 method 区分，
// 同路径的 GET/POST 通过 handler 内 method 校验分流。
func RegisterHandlers(mux *http.ServeMux, app *backend.App) {
	s := NewServer(app)

	// 健康检查
	mux.HandleFunc("/api/health", s.handleHealth)

	// Probe selector endpoints
	mux.HandleFunc("/api/probe/available", s.handleProbeAvailable)
	mux.HandleFunc("/api/probe/active", s.handleProbeActive)
	mux.HandleFunc("/api/probe/clear", s.handleProbeClear)

	// 5 孔 endpoints
	mux.HandleFunc("/api/five/load-prb", s.handleFiveLoadPrb)
	mux.HandleFunc("/api/five/is-loaded", s.handleFiveIsLoaded)
	mux.HandleFunc("/api/five/prb-files", s.handleFivePrbFiles)
	mux.HandleFunc("/api/five/mach-range", s.handleFiveMachRange)
	mux.HandleFunc("/api/five/calculate", s.handleFiveCalculate)
	mux.HandleFunc("/api/five/batch-calculate", s.handleFiveBatchCalculate)
	mux.HandleFunc("/api/five/import-csv", s.handleFiveImportCsv)
	mux.HandleFunc("/api/five/help-doc", s.handleFiveHelpDoc)

	// 3 孔 endpoints
	mux.HandleFunc("/api/three/load-prb", s.handleThreeLoadPrb)
	mux.HandleFunc("/api/three/is-loaded", s.handleThreeIsLoaded)
	mux.HandleFunc("/api/three/prb-files", s.handleThreePrbFiles)
	mux.HandleFunc("/api/three/mach-range", s.handleThreeMachRange)
	mux.HandleFunc("/api/three/calculate", s.handleThreeCalculate)
	mux.HandleFunc("/api/three/batch-calculate", s.handleThreeBatchCalculate)
	mux.HandleFunc("/api/three/import-csv", s.handleThreeImportCsv)
	mux.HandleFunc("/api/three/help-doc", s.handleThreeHelpDoc)

	// 7 孔 endpoints
	mux.HandleFunc("/api/seven/load-prb", s.handleSevenLoadPrb)
	mux.HandleFunc("/api/seven/is-loaded", s.handleSevenIsLoaded)
	mux.HandleFunc("/api/seven/prb-files", s.handleSevenPrbFiles)
	mux.HandleFunc("/api/seven/valid-range", s.handleSevenValidRange)
	mux.HandleFunc("/api/seven/calculate", s.handleSevenCalculate)
	mux.HandleFunc("/api/seven/batch-calculate", s.handleSevenBatchCalculate)
	mux.HandleFunc("/api/seven/import-csv", s.handleSevenImportCsv)
	mux.HandleFunc("/api/seven/help-doc", s.handleSevenHelpDoc)
}

// handleHealth 健康检查 endpoint，返回 JSON {"status":"ok"}。
// Electron 主进程启动后轮询此 endpoint，确认 Go 子进程已就绪。
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// w.Write 返回 (int, error)，Go 不允许 _ = w.Write(...) 仅丢弃 1 个返回值。
	// 作为语句直接调用即可，编译器会丢弃全部返回值。
	w.Write([]byte(`{"status":"ok"}`))
}
