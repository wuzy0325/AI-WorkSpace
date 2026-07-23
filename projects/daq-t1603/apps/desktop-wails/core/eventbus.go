package core

// 前端事件名常量。所有 Service 推送事件时统一引用，避免字符串散落各处。
const (
	EventPayload                = "daq:payload"                // 温度快照（DeviceService.relayStream 推送）
	EventLog                    = "daq:log"                    // 日志事件（LogService.EmitLog 推送）
	EventRecordingStatus        = "daq:recording-status"        // 录制状态变化（RecordingService.EmitStatus 推送）
	EventRecordingBackpressure  = "daq:recording-backpressure"  // 录制背压（丢帧）
	EventRecordingFatal         = "daq:recording-fatal"         // 录制不可恢复错误
)

// EventBus 抽象"向前端推送事件"的能力。
//
// 设计动机：原 Wails v3 实现通过 application.App.Event.Emit 推送，
// Win7 分支改用 HTTP + WebSocket 后由 httpserver.WSHub 实现。
// backend 层依赖此抽象而非具体传输，便于在不同壳（Wails / Electron+WS）间复用。
type EventBus interface {
	// Emit 推送一个事件到所有订阅的前端连接。
	//   - name：事件名（见上方常量）
	//   - data：事件 payload，会被 JSON 序列化
	Emit(name string, data ...any)
}

// noopEventBus 在 EventBus 未注入时使用，丢弃所有事件，保证启动期不 panic。
type noopEventBus struct{}

func (noopEventBus) Emit(string, ...any) {}