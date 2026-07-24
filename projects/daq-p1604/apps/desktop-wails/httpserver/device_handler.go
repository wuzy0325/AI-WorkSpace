// Package httpserver device_handler：App 中设备相关方法的 HTTP 包装。
//
// 路由约定（详见 register.go）：
//   - POST   /api/device/scan                 扫描设备
//   - GET    /api/device/profiles             列出全部配置
//   - POST   /api/device/profile              新增/更新配置
//   - DELETE /api/device/profile/{id}         删除配置
//   - POST   /api/device/connect              连接设备，body: {"id":"..."}
//   - POST   /api/device/disconnect           断开设备，body: {"id":"..."}
//   - POST   /api/device/start                启动采集，body: {"id":"..."}
//   - POST   /api/device/stop                 停止采集，body: {"id":"..."}
//   - GET    /api/device/status/{id}          查询设备状态
//   - POST   /api/device/apply-config         下发配置，body: {"id":"...","config":{...}}
//   - GET    /api/device/latest-snapshots     批量获取所有设备最新快照
//   - GET    /api/device/latest-snapshot/{id} 获取指定设备最新快照
//
// 错误码约定：
//   - 400：参数缺失或格式错误；
//   - 404：设备不存在（GetStatus 专属）；
//   - 405：HTTP 方法不匹配；
//   - 500：Service 内部错误（硬件异常、配置持久化失败等）。
package httpserver

import (
	"net/http"
	"strings"

	"daq-p1604/backend"
	"daq-p1604/core"
)

// handleDeviceScan POST /api/device/scan
// 触发设备扫描，返回发现的设备列表。
// 扫描可能耗时（数秒），前端应禁用按钮并显示 loading。
func (s *Server) handleDeviceScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	results, err := s.app.ScanDevices()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, results)
}

// handleDeviceProfiles GET /api/device/profiles
// 返回所有已保存的设备配置。无配置时返回空数组。
func (s *Server) handleDeviceProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeOK(w, s.app.GetProfiles())
}

// handleDeviceProfileUpsert POST /api/device/profile
// 请求体为完整 PressureProfile，按 ID 判断新增/更新。
func (s *Server) handleDeviceProfileUpsert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var profile core.PressureProfile
	if !decodeJSON(w, r, &profile) {
		return
	}
	if profile.ID == "" {
		writeError(w, http.StatusBadRequest, "missing profile id")
		return
	}
	if err := s.app.UpsertProfile(profile); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, nil)
}

// handleDeviceProfileDelete DELETE /api/device/profile/{id}
// id 从 URL path 末段提取，URL 解码由 http.ServeMux 自动完成。
func (s *Server) handleDeviceProfileDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/device/profile/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing profile id in path")
		return
	}
	if err := s.app.DeleteProfile(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, nil)
}

// handleDeviceConnect POST /api/device/connect
// 连接指定设备，连接结果通过 hub.EmitLog 推送，前端订阅 daq:log 实时查看。
func (s *Server) handleDeviceConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id, ok := decodeIDRequest(w, r)
	if !ok {
		return
	}
	if err := s.app.Connect(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, nil)
}

// handleDeviceDisconnect POST /api/device/disconnect
// 断开指定设备，会等待 relay 协程收尾后再返回，确保状态一致。
func (s *Server) handleDeviceDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id, ok := decodeIDRequest(w, r)
	if !ok {
		return
	}
	if err := s.app.Disconnect(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, nil)
}

// handleDeviceStart POST /api/device/start
// 启动指定设备采集，成功后开始向 latestSnapshots 缓存写入快照供前端轮询。
func (s *Server) handleDeviceStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id, ok := decodeIDRequest(w, r)
	if !ok {
		return
	}
	if err := s.app.StartAcquisition(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, nil)
}

// handleDeviceStop POST /api/device/stop
// 停止指定设备采集，等待 relay 协程收尾后返回。
func (s *Server) handleDeviceStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id, ok := decodeIDRequest(w, r)
	if !ok {
		return
	}
	if err := s.app.StopAcquisition(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, nil)
}

// handleDeviceStatus GET /api/device/status/{id}
// 返回指定设备的当前状态。设备不存在时返回 404，前端用 .catch(()=>false) 兼容。
//
// 注意：与 backend.ErrDeviceNotFound 强耦合，若错误类型变化需同步更新这里。
func (s *Server) handleDeviceStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/device/status/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing device id in path")
		return
	}
	state, err := s.app.GetStatus(id)
	if err != nil {
		// ErrDeviceNotFound 与其他内部错误统一映射到 404；
		// 前端 deviceStore.ts 已用 .catch(()=>false) 兼容此签名。
		if err == backend.ErrDeviceNotFound {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, state)
}

// handleDeviceApplyConfig POST /api/device/apply-config
// 请求体：{"id":"<deviceId>","config":{...core.P1604Config}}
// 采集进行中调用会返回错误（详见 usecase.DeviceUsecase.ApplyConfig）。
func (s *Server) handleDeviceApplyConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		ID     string          `json:"id"`
		Config core.P1604Config `json:"config"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.ID == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	if err := s.app.ApplyConfig(body.ID, body.Config); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, nil)
}

// handleDeviceLatestSnapshots GET /api/device/latest-snapshots
// 返回所有设备的最新快照 map[deviceId]PressureSnapshot。
//
// 设计动机：daq-p1604 v0.3.0 移除了 daq:payload 事件，改为前端 500ms 轮询。
// 多设备场景下用此端点一次拉取所有设备快照，减少 HTTP 请求次数。
//
// 返回值：data 字段是 map[string]PressureSnapshot，无活跃设备时返回空 map。
func (s *Server) handleDeviceLatestSnapshots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeOK(w, s.app.GetLatestSnapshots())
}

// handleDeviceLatestSnapshot GET /api/device/latest-snapshot/{id}
// 返回指定设备的最新快照。设备无快照时返回 404，前端可据此跳过该设备渲染。
//
// 与 handleDeviceLatestSnapshots 的选择：
//   - 单设备仪表盘视图：用此端点减少响应体大小
//   - 多设备总览视图：用批量端点减少 HTTP 请求次数
//
// 返回值：设备存在快照时返回 {"ok":true,"data":<PressureSnapshot>}；
// 设备无快照时返回 404 {"ok":false,"error":"no snapshot"}。
func (s *Server) handleDeviceLatestSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/device/latest-snapshot/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing device id in path")
		return
	}
	snapshot, ok := s.app.GetLatestSnapshot(id)
	if !ok {
		writeError(w, http.StatusNotFound, "no snapshot")
		return
	}
	writeOK(w, snapshot)
}
