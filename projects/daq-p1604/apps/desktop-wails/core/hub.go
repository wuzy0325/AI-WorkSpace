package core

import (
	"context"
	"sync"
)

// RelayControl 表示一个数据中继协程的控制句柄，
// 用于 App 在启动/停止采集时配对管理后台 goroutine。
//
// 与 daq-t1603 的字段命名差异：da-q1603 用 Cancel/Done 大写首字母，
// daq-p1604 沿用原 backend/app.go 内 relayControl 的小写命名以保持兼容。
// 由于此类型从 backend 包迁移到 core 包，原 backend.relayControl 字段也需同步改名。
type RelayControl struct {
	Cancel context.CancelFunc
	Done   chan struct{}
}

// LogEvent 是前后端共享的日志事件结构。
// 放到 core 层方便 App 各方法共同发布，避免 backend.LogEvent 与 core 之间的循环依赖。
//
// 字段与原 backend.LogEvent 完全一致，JSON tag 不变，前端 onLog 解析无需改动。
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
// 由 backend.App 实现（App.EmitLog 同时写日志文件 + 推送前端事件）。
type LogEmitter interface {
	EmitLog(entry LogEvent)
}

// Hub 是 backend.App 共享的运行期状态容器。
//
// 设计目标：
//  1. 集中保存应用生命周期 ctx（在 ServiceStartup 时注入），
//     让 App 中各方法共享一个可被统一取消的上下文；
//  2. 维护 deviceID -> *RelayControl 的中继协程映射，
//     App 启停采集时通过 Hub 管理；
//  3. 通过 LogEmitter 抽象提供统一的日志发布入口，
//     避免 App 内部 EmitLog 的自我递归（写日志文件 + 推事件都走同一入口）；
//  4. 通过 EventBus 抽象"推送给前端"的能力，使 App 不再耦合具体传输
//     （Wails v3 的 application.App.Event.Emit 或 httpserver 的 WebSocket 推送）。
//
// 并发性：所有方法都加锁；中继 goroutine 在被取消后会调用 ClearRelay 自清理。
//
// 与 daq-t1603 的差异：daq-p1604 未把 App 拆分为 3 个 Service，
// Hub 仍由单体 App 使用，但保留同样的接口以便未来重构。
type Hub struct {
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	relays   map[string]*RelayControl
	emitter  LogEmitter
	bus      EventBus
}

// NewHub 创建一个空的 Hub。ctx 在应用启动时由 SetContext 注入。
func NewHub() *Hub {
	return &Hub{
		relays: make(map[string]*RelayControl),
	}
}

// SetContext 在应用启动时由 App.ServiceStartup 调用一次。
// 派生出一个可取消的子 context，供 EmitLog 等场景判断应用是否仍在运行。
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

// SetEmitter 注入实际的日志发布器（通常是 App 自身）。
// 在 App 构造完成后调用一次即可。
func (h *Hub) SetEmitter(emitter LogEmitter) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.emitter = emitter
}

// EmitLog 统一日志事件入口；如果尚未注入 emitter 则静默丢弃。
// 由 App 内部各业务方法调用，避免直接耦合 LogUsecase 与 EventBus。
func (h *Hub) EmitLog(entry LogEvent) {
	h.mu.Lock()
	emitter := h.emitter
	h.mu.Unlock()
	if emitter != nil {
		emitter.EmitLog(entry)
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

// RelayCount 返回当前登记在册的中继协程数。
// 由 App.handleRelayExit 调用，区分"单设备断连自动停止录制"与"多设备场景 emit 警告"。
//
// 注意：在 handleRelayExit 调用路径上，当前退出的 relay 已通过 ClearRelay 移除，
// 因此返回值反映的是"除当前设备外的其他活跃设备数"。
func (h *Hub) RelayCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.relays)
}

// SetEventBus 注入实际的事件总线（通常是 httpserver.WSHub）。
// 必须在 ServiceStartup 之前调用一次。传入 nil 会回退到丢弃模式。
func (h *Hub) SetEventBus(bus EventBus) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.bus = bus
}

// EmitEvent 转发事件到注入的 EventBus。
// 如果尚未 SetEventBus，调用会被静默丢弃（启动早期或测试场景）。
//
// 多参数事件（如 daq:device-state 传 id + state）：调用方传 EmitEvent(name, id, state)，
// httpserver.WSHub.Emit 取 data[0] 作为 payload；为支持双参数，
// WSHub.Emit 会把所有 data 元素打包为数组（详见 ws_hub.go）。
func (h *Hub) EmitEvent(name string, data ...any) {
	h.mu.Lock()
	bus := h.bus
	h.mu.Unlock()
	if bus == nil {
		return
	}
	bus.Emit(name, data...)
}
