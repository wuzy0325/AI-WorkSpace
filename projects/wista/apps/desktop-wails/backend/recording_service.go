package backend

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"wista/core"
	"wista/usecase"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// RecordingService 暴露录制相关能力给前端，并对外提供事件发布。
//
// 与 DeviceService 解耦：DeviceService 中的 relay 协程通过 Hub 获取本 Service，
// 借助 IsActive/Write/EmitStatus 完成录制热路径调用，无需直接依赖 RecordingService 类型。
type RecordingService struct {
	hub      *core.Hub
	recordUC *usecase.RecordingUsecase

	mu  sync.Mutex
	app *application.App

	// bpLastEmitMs 限频背压事件到前端的发射时间戳，避免 10 台同时背压时
	// Event.Emit 刷屏（默认每设备每秒最多 1 条）。
	bpLastEmitMs atomic.Int64

	// fatalLastEmitMs 限频 fatal 事件到前端的发射时间戳。
	// 与 bpLastEmitMs 独立，避免背压事件挤占 fatal 事件配额。
	fatalLastEmitMs atomic.Int64
}

// NewRecordingService 创建录制 Service。
func NewRecordingService(hub *core.Hub, recordUC *usecase.RecordingUsecase) *RecordingService {
	return &RecordingService{
		hub:      hub,
		recordUC: recordUC,
	}
}

// ServiceName Wails 绑定时使用的服务名。
func (s *RecordingService) ServiceName() string {
	return "RecordingService"
}

// ServiceStartup 缓存 application 引用，并注入背压/错误回调。
//
// 回调由 recorder 在丢帧/不可恢复错误时同步调用，因此必须非阻塞：
//   - 日志走 hub.EmitLog（已是异步广播）；
//   - Event.Emit 每类事件 1s 最多 1 次，由对应 atomic 限频。
func (s *RecordingService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	s.mu.Lock()
	s.app = application.Get()
	s.mu.Unlock()

	s.recordUC.SetBackpressureHandler(s.handleBackpressure)
	s.recordUC.SetFatalErrorHandler(s.handleFatal)
	return nil
}

// ServiceShutdown 关闭录制（如有），保证文件正常 flush。
func (s *RecordingService) ServiceShutdown() error {
	_ = s.recordUC.Stop()
	return nil
}

// IsActive 暴露给 DeviceService 中继协程的无锁热路径查询。
func (s *RecordingService) IsActive() bool {
	return s.recordUC.IsActive()
}

// Write 暴露给 DeviceService 中继协程的写入入口。
func (s *RecordingService) Write(snapshot core.TemperatureSnapshot) error {
	return s.recordUC.Write(snapshot)
}

// SetDeviceProfile 注入设备通道配置到 recorder（REC-006）。
// 由 DeviceService 在设备 Connect / StartAcquisition / ApplyConfig 时调用，
// 让 recorder 在 CSV 写入时将禁用通道的列写空值，便于操作员区分
// "通道禁用"与"通道无数据"。
func (s *RecordingService) SetDeviceProfile(deviceID string, channels []core.ChannelConfig) {
	s.recordUC.SetDeviceProfile(deviceID, channels)
}

// EmitStatus 通过 Wails Event 总线广播当前录制状态。
// 在 DeviceService 中继协程内被高频调用，因此实现需轻量。
func (s *RecordingService) EmitStatus() {
	s.mu.Lock()
	app := s.app
	s.mu.Unlock()
	if app == nil {
		return
	}
	app.Event.Emit("daq:recording-status", s.recordUC.Status())
}

// StartRecording 开始录制；启动后立刻广播一次状态。
func (s *RecordingService) StartRecording(outputDir string, filePrefix string) error {
	if err := s.recordUC.Start(outputDir, filePrefix); err != nil {
		return err
	}
	s.EmitStatus()
	return nil
}

// StopRecording 停止录制；停止后立刻广播一次状态。
func (s *RecordingService) StopRecording() error {
	if err := s.recordUC.Stop(); err != nil {
		return err
	}
	s.EmitStatus()
	return nil
}

// GetRecordingStatus 返回当前录制状态。
func (s *RecordingService) GetRecordingStatus() core.RecordingSession {
	return s.recordUC.Status()
}

// PickDirectory 打开系统目录选择对话框。
func (s *RecordingService) PickDirectory() (string, error) {
	s.mu.Lock()
	app := s.app
	s.mu.Unlock()
	return pickDirectory(app)
}

// handleBackpressure 是 recorder 丢帧时同步调用的回调。
//
// 实现约束：
//   - 必须非阻塞（recorder 持锁内调用）；
//   - 禁止回调 recordUC 任何方法（死锁）；
//   - 日志每条都发（前端 LogPanel 可见），Event.Emit 限频到全局 1Hz
//     避免 10 台并发背压时 WebView2 ExecuteScript 刷屏。
func (s *RecordingService) handleBackpressure(e core.BackpressureEvent) {
	if s.hub != nil {
		s.hub.EmitLog(core.LogEvent{
			Level:    "warn",
			Category: "recording",
			DeviceID: e.DeviceID,
			Source:   "recorder",
			Message:  "录制队列背压，丢帧",
			Detail: detailJSON(e),
		})
	}

	now := core.TimestampMs()
	last := s.bpLastEmitMs.Load()
	if now-last < 1000 {
		return
	}
	// CAS 限频，多设备并发时只有一个能成功
	if !s.bpLastEmitMs.CompareAndSwap(last, now) {
		return
	}

	s.mu.Lock()
	app := s.app
	s.mu.Unlock()
	if app == nil {
		return
	}
	app.Event.Emit("daq:recording-backpressure", e)
}

// detailJSON 构造背压详情的简易 JSON 字符串。
// 不引入 encoding/json 以避免热路径分配；
// 字段顺序固定，前端可按需解析。
func detailJSON(e core.BackpressureEvent) string {
	return fmt.Sprintf("{\"deviceId\":\"%s\",\"queueLen\":%d,\"queueCap\":%d,\"droppedTotal\":%d}",
		e.DeviceID, e.QueueLen, e.QueueCap, e.DroppedTotal)
}

// handleFatal 是 recorder 不可恢复 I/O 错误时同步调用的回调。
//
// 触发场景：
//   - deviceWriter 文件创建失败（磁盘满/权限）→ 每设备每秒最多 1 条 fatal
//   - bufio.Write 失败（磁盘掉线/网络盘断开）→ 首次错误时 1 条 fatal
//
// 实现约束：
//   - 必须非阻塞（recorder 持锁内调用）；
//   - 禁止回调 recordUC 任何方法（死锁）；
//   - 日志每条都发，Event.Emit 限频到 1Hz。
func (s *RecordingService) handleFatal(deviceID string, err error) {
	if s.hub != nil {
		s.hub.EmitLog(core.LogEvent{
			Level:    "error",
			Category: "recording",
			DeviceID: deviceID,
			Source:   "recorder",
			Message:  "录制不可恢复错误，该设备后续帧将丢弃",
			Detail:   fmt.Sprintf("deviceID=%s err=%s", deviceID, err.Error()),
		})
	}

	now := core.TimestampMs()
	last := s.fatalLastEmitMs.Load()
	if now-last < 1000 {
		return
	}
	if !s.fatalLastEmitMs.CompareAndSwap(last, now) {
		return
	}

	s.mu.Lock()
	app := s.app
	s.mu.Unlock()
	if app == nil {
		return
	}
	app.Event.Emit("daq:recording-fatal", map[string]string{
		"deviceId": deviceID,
		"error":    err.Error(),
	})
}
