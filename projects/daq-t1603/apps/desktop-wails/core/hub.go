package core

import (
	"context"
	"sync"
)

// RelayControl 表示一个数据中继协程的控制句柄，
// 用于 DeviceService 在启动/停止采集时配对管理后台 goroutine。
type RelayControl struct {
	Cancel context.CancelFunc
	Done   chan struct{}
}

// LogEvent 是前后端共享的日志事件结构。
// 放到 core 层方便 DeviceService / RecordingService / LogService 共同发布。
type LogEvent struct {
	Level     string `json:"level"`
	Category  string `json:"category"`
	DeviceID  string `json:"deviceId,omitempty"`
	Source    string `json:"source"`
	Message   string `json:"message"`
	Detail    string `json:"detail,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// LogEmitter 抽象出日志事件的发送目标。
// 在 Wails v3 下由 backend.LogService 实现，借助 application.App.Event.Emit 推送给前端。
type LogEmitter interface {
	EmitLog(entry LogEvent)
}

// StateEmitter 抽象出设备状态变更事件的发送目标（ACQ-010/STB-003）。
// 由 backend.DeviceService 实现，借助 application.App.Event.Emit 推送给前端。
// adapter 在 OnReadLoopExit 等异步状态变化时通过 hub.EmitDeviceState 触发，
// 让前端 statusMap 实时同步，避免依赖轮询。
type StateEmitter interface {
	EmitDeviceState(deviceID string, state DeviceState)
}

// Hub 是 backend 各 Service 共享的运行期状态容器。
//
// 设计目标：
//   1. 集中保存应用生命周期 ctx（在 ServiceStartup 时注入），
//      让所有 Service 共享一个可被统一取消的上下文；
//   2. 维护 deviceID -> *RelayControl 的中继协程映射，
//      DeviceService 启停采集时通过 Hub 管理；
//   3. 通过 LogEmitter 抽象提供给所有 Service 一个统一的日志发布入口，
//      避免循环依赖（DeviceService/RecordingService 不必直接依赖 LogService）。
//
// 并发性：所有方法都加锁；中继 goroutine 在被取消后会调用 ClearRelay 自清理。
type Hub struct {
	mu        sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
	relays    map[string]*RelayControl
	emitter   LogEmitter
	stateEmit StateEmitter
}

// NewHub 创建一个空的 Hub。ctx 在应用启动时由 SetContext 注入。
func NewHub() *Hub {
	return &Hub{
		relays: make(map[string]*RelayControl),
	}
}

// SetContext 在应用启动时由 ServiceStartup 调用一次。
// 它会派生出一个可取消的子 context，供 EmitLog 等场景判断应用是否仍在运行。
func (h *Hub) SetContext(parent context.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ctx, h.cancel = context.WithCancel(parent)
}

// Context 返回当前 Hub 持有的应用级 context（可能为 nil，调用方需自行判空）。
func (h *Hub) Context() context.Context {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.ctx
}

// CancelContext 在应用关闭时调用，主动释放 Hub 中保存的 ctx。
func (h *Hub) CancelContext() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
}

// SetEmitter 注入实际的日志发布器（通常是 LogService）。
// 在 services 注册完成后调用一次即可。
func (h *Hub) SetEmitter(emitter LogEmitter) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.emitter = emitter
}

// EmitLog 统一日志事件入口；如果尚未注入 emitter 则静默丢弃。
func (h *Hub) EmitLog(entry LogEvent) {
	h.mu.Lock()
	emitter := h.emitter
	h.mu.Unlock()
	if emitter != nil {
		emitter.EmitLog(entry)
	}
}

// SetStateEmitter 注入设备状态变更事件发布器（通常是 DeviceService）。
// 在 DeviceService.ServiceStartup 时调用一次即可。
func (h *Hub) SetStateEmitter(emitter StateEmitter) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stateEmit = emitter
}

// EmitDeviceState 设备状态变更事件入口；如果尚未注入 emitter 则静默丢弃。
// adapter 在 OnReadLoopExit 等异步状态变化时调用此方法，
// 让前端通过 daq:device-state 事件实时同步 statusMap（ACQ-010/STB-003）。
func (h *Hub) EmitDeviceState(deviceID string, state DeviceState) {
	h.mu.Lock()
	emitter := h.stateEmit
	h.mu.Unlock()
	if emitter != nil {
		emitter.EmitDeviceState(deviceID, state)
	}
}

// RegisterRelay 登记某设备的中继协程；
// 若该设备已存在旧控制句柄，会先 cancel 旧的，再写入新的，避免泄漏。
func (h *Hub) RegisterRelay(deviceID string, control *RelayControl) {
	h.mu.Lock()
	if old, ok := h.relays[deviceID]; ok {
		old.Cancel()
	}
	h.relays[deviceID] = control
	h.mu.Unlock()
}

// ClearRelay 在 relay goroutine 收尾时调用，仅在传入的 control 仍是当前持有的那个时移除。
// 这样可以避免 "已被新的 register 覆盖" 时被错误删除。
func (h *Hub) ClearRelay(deviceID string, control *RelayControl) {
	h.mu.Lock()
	if h.relays[deviceID] == control {
		delete(h.relays, deviceID)
	}
	h.mu.Unlock()
}

// WaitRelay 阻塞等待指定设备的 relay goroutine 收尾；不存在则立即返回。
func (h *Hub) WaitRelay(deviceID string) {
	h.mu.Lock()
	control := h.relays[deviceID]
	h.mu.Unlock()
	if control != nil {
		<-control.Done
	}
}

// StopAllRelays 取消并清空所有中继协程，用于应用关闭。
func (h *Hub) StopAllRelays() {
	h.mu.Lock()
	relays := h.relays
	h.relays = make(map[string]*RelayControl)
	h.mu.Unlock()
	for _, control := range relays {
		control.Cancel()
	}
}
