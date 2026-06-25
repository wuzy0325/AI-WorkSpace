package backend

import (
	"context"
	"sync"

	"daq-t1603/core"
	"daq-t1603/usecase"

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

// ServiceStartup 仅缓存 application 引用，供后续事件发送。
func (s *RecordingService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	s.mu.Lock()
	s.app = application.Get()
	s.mu.Unlock()
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
