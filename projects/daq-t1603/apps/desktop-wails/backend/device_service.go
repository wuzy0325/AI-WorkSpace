package backend

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"daq-t1603/core"
	"daq-t1603/usecase"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// 前端推送节奏：UI 快照 100ms 一次，录制状态 1s 一次。
// 由 DeviceService.relayStream 使用，平衡刷新频率与 Event.Emit 开销。
const (
	uiPayloadRefreshInterval    = 100 * time.Millisecond
	recordingStatusEmitInterval = time.Second
)

// DeviceService 暴露设备相关能力给前端：
//   - 扫描 / 配置 CRUD
//   - 连接 / 断开 / 应用配置
//   - 启动 / 停止采集（带后台中继协程，按 100ms 频率推送温度快照、按 1s 频率推送录制状态）
//
// 设计要点：
//   - 录制相关的热路径调用通过 hub.RecordingService 引用直接走，
//     避免在 RecordingService 中再绕一层 Wails Bind 调用；
//   - 中继协程的生命周期由 hub 集中管理（RegisterRelay/ClearRelay/WaitRelay/StopAllRelays）。
type DeviceService struct {
	hub      *core.Hub
	deviceUC *usecase.DeviceUsecase

	// 录制 Service 引用：用于 relay 热路径 (IsActive/Write/EmitStatus)。
	// 之所以注入而不是从 hub 拿，是因为只有 DeviceService 需要这种紧耦合。
	recording *RecordingService

	mu  sync.Mutex
	app *application.App
}

// NewDeviceService 创建设备 Service。
func NewDeviceService(hub *core.Hub, deviceUC *usecase.DeviceUsecase, recording *RecordingService) *DeviceService {
	return &DeviceService{
		hub:       hub,
		deviceUC:  deviceUC,
		recording: recording,
	}
}

// ServiceName Wails 绑定时使用的服务名。
func (s *DeviceService) ServiceName() string {
	return "DeviceService"
}

// ServiceStartup 缓存 application 引用，供事件发布使用。
func (s *DeviceService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	s.mu.Lock()
	s.app = application.Get()
	s.mu.Unlock()
	// ACQ-010/STB-003：将本 service 注册为 hub 的 StateEmitter，
	// 让 adapter 在 OnReadLoopExit 等异步状态变化时通过 hub.EmitDeviceState
	// 触发本 service 的 EmitDeviceState，最终推送 daq:device-state 事件到前端。
	s.hub.SetStateEmitter(s)
	return nil
}

// ServiceShutdown 在应用关闭时停止所有 relay 协程，避免泄漏。
func (s *DeviceService) ServiceShutdown() error {
	s.hub.StopAllRelays()
	return nil
}

// ----------------------------------------------------------------------------
// 前端可调用方法（被 wails3 generate bindings 扫描后生成对应 TS 函数）
// ----------------------------------------------------------------------------

// ScanDevices 触发设备扫描。
func (s *DeviceService) ScanDevices() ([]core.ScanResult, error) {
	return s.deviceUC.ScanDevices()
}

// GetProfiles 获取所有设备配置。
func (s *DeviceService) GetProfiles() []core.TemperatureProfile {
	return s.deviceUC.GetProfiles()
}

// UpsertProfile 新增 / 更新设备配置。
func (s *DeviceService) UpsertProfile(profile core.TemperatureProfile) error {
	s.emitLog("info", "system", profile.ID, "config", "保存设备配置", "")
	if err := s.deviceUC.UpsertProfile(profile); err != nil {
		s.emitLog("error", "system", profile.ID, "config", "保存设备配置失败", err.Error())
		return err
	}
	s.emitLog("info", "system", profile.ID, "config", "设备配置已保存", "")
	return nil
}

// DeleteProfile 删除设备配置。
func (s *DeviceService) DeleteProfile(id string) error {
	s.emitLog("info", "system", id, "config", "删除设备配置", "")
	if err := s.deviceUC.DeleteProfile(id); err != nil {
		s.emitLog("error", "system", id, "config", "删除设备配置失败", err.Error())
		return err
	}
	s.emitLog("info", "system", id, "config", "设备配置已删除", "")
	return nil
}

// Connect 连接设备。
func (s *DeviceService) Connect(id string) error {
	s.emitLog("info", "system", id, "device", "Connect requested", "")
	if err := s.deviceUC.Connect(id); err != nil {
		s.emitLog("error", "system", id, "device", "Connect failed", err.Error())
		return err
	}
	// REC-006：连接成功后注入通道掩码到 recorder，
	// 保证后续 StartAcquisition 时 CSV 立即按禁用通道写空值。
	s.injectDeviceProfile(id)
	s.emitLog("info", "system", id, "device", "Device connected", "")
	return nil
}

// Disconnect 断开设备。
func (s *DeviceService) Disconnect(id string) error {
	s.emitLog("info", "system", id, "device", "Disconnect requested", "")
	if err := s.deviceUC.Disconnect(id); err != nil {
		s.emitLog("error", "system", id, "device", "Disconnect failed", err.Error())
		return err
	}
	s.hub.WaitRelay(id)
	s.emitLog("info", "system", id, "device", "Device disconnected", "")
	return nil
}

// StartAcquisition 启动采集。
func (s *DeviceService) StartAcquisition(id string) error {
	s.emitLog("info", "acquisition", id, "device", "Start acquisition requested", "")
	// REC-006：采集启动前再次注入通道掩码，覆盖用户在 Connect 后修改配置的情况。
	// 即便 recorder 尚未 Start，SetDeviceProfile 也会缓存掩码到 deviceProfiles，
	// 待 recorder Start 后 newDeviceWriter 创建时自动应用。
	s.injectDeviceProfile(id)
	ch, err := s.deviceUC.StartAcquisition(id)
	if err != nil {
		s.emitLog("error", "acquisition", id, "device", "Start acquisition failed", err.Error())
		return err
	}
	s.startRelay(id, ch)
	s.emitLog("info", "acquisition", id, "device", "Acquisition started", "")
	return nil
}

// StopAcquisition 停止采集。
func (s *DeviceService) StopAcquisition(id string) error {
	s.emitLog("info", "acquisition", id, "device", "Stop acquisition requested", "")
	if err := s.deviceUC.StopAcquisition(id); err != nil {
		s.emitLog("error", "acquisition", id, "device", "Stop acquisition failed", err.Error())
		return err
	}
	s.hub.WaitRelay(id)
	s.emitLog("info", "acquisition", id, "device", "Acquisition stopped", "")
	return nil
}

// GetStatus 返回指定设备的当前状态。
//
// 返回签名 (DeviceState, error)：
//   - 成功（设备存在）: 返回 state, nil
//   - 未找到（设备不存在 / 未连接过）: 返回 zero, ErrDeviceNotFound
//
// 前端 ts 已用 `as any + .catch(()=>false)` 兼容这个签名变更。
func (s *DeviceService) GetStatus(id string) (core.DeviceState, error) {
	state, ok := s.deviceUC.GetStatus(id)
	if !ok {
		return core.DeviceState{}, ErrDeviceNotFound
	}
	return state, nil
}

// ApplyConfig 下发设备配置。
func (s *DeviceService) ApplyConfig(id string, cfg core.T1603Config) error {
	s.emitLog("info", "system", id, "device", "Apply config requested", "")
	if err := s.deviceUC.ApplyConfig(id, cfg); err != nil {
		s.emitLog("error", "system", id, "device", "Apply config failed", err.Error())
		return err
	}
	// REC-006：配置变更后通道掩码可能变化（如 channelMask 调整），
	// 同步到 recorder 以保证后续 CSV 写入按最新掩码过滤。
	s.injectDeviceProfile(id)
	s.emitLog("info", "system", id, "device", "Config applied", "")
	return nil
}

// injectDeviceProfile 从 usecase 加载设备 profile 并注入通道掩码到 recorder（REC-006）。
//
// 静默失败策略：
//   - profile 不存在： recorder 未启动时 SetDeviceProfile 内部会缓存，
//     若 deviceUC 返回空列表则跳过（不应影响主流程）；
//   - recorder 未启动： SetDeviceProfile 仅缓存到 deviceProfiles，
//     待 recorder Start 后 newDeviceWriter 自动应用。
//
// 该方法不返回 error：通道掩码注入是录制功能的"增强"，
// 不应让 CSV 空值列问题反过来阻塞设备连接/采集主流程。
func (s *DeviceService) injectDeviceProfile(deviceID string) {
	profiles := s.deviceUC.GetProfiles()
	for i := range profiles {
		if profiles[i].ID == deviceID {
			s.recording.SetDeviceProfile(deviceID, profiles[i].Channels)
			return
		}
	}
	// 极少数 race condition 场景下可能发生（如 profile 刚被删除但连接尚未清理），
	// 记录 debug 日志便于排查"为何 CSV 禁用通道仍写值"等问题，不阻塞主流程。
	slog.Debug("injectDeviceProfile: profile not found, skip mask injection",
		"deviceId", deviceID)
}

// ----------------------------------------------------------------------------
// 内部：中继协程
// ----------------------------------------------------------------------------

// startRelay 启动一个后台 goroutine，把硬件 channel 的快照按节流策略转发给：
//   1. 前端（通过 Wails Event，"daq:payload"）
//   2. 录制写入器（条件：RecordingService.IsActive()）
//   3. 周期性广播录制状态（"daq:recording-status"）
func (s *DeviceService) startRelay(deviceID string, ch <-chan core.TemperatureSnapshot) {
	baseCtx := s.hub.Context()
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(baseCtx)
	control := &core.RelayControl{Cancel: cancel, Done: make(chan struct{})}
	s.hub.RegisterRelay(deviceID, control)

	go func() {
		defer close(control.Done)
		defer s.hub.ClearRelay(deviceID, control)
		s.relayStream(ctx, deviceID, ch)
	}()
}

// relayStream 中继协程主循环。
//
// 节流策略：
//   - UI payload: 每 100ms 推送一次最新值（uiTicker），避免高频更新拖慢前端；
//   - 录制写入: 直接逐帧写入（hot path），通过 atomic.Bool 判活无锁；
//   - 录制状态: 每 1s 广播一次（statusTicker），且仅在 IsActive 时广播。
func (s *DeviceService) relayStream(ctx context.Context, deviceID string, ch <-chan core.TemperatureSnapshot) {
	uiTicker := time.NewTicker(uiPayloadRefreshInterval)
	statusTicker := time.NewTicker(recordingStatusEmitInterval)
	defer uiTicker.Stop()
	defer statusTicker.Stop()

	var latest core.TemperatureSnapshot
	hasLatest := false

	defer func() {
		if hasLatest {
			s.emitPayload(latest)
		}
		s.recording.EmitStatus()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case snapshot, ok := <-ch:
			if !ok {
				return
			}
			latest = snapshot
			hasLatest = true
			if s.recording.IsActive() {
				if err := s.recording.Write(snapshot); err != nil {
					s.emitLog("error", "acquisition", deviceID, "recording", "Record snapshot failed", err.Error())
				}
			}
		case <-uiTicker.C:
			if hasLatest {
				s.emitPayload(latest)
			}
		case <-statusTicker.C:
			if s.recording.IsActive() {
				s.recording.EmitStatus()
			}
		}
	}
}

// emitPayload 通过 Wails Event 总线推送温度快照。
func (s *DeviceService) emitPayload(snapshot core.TemperatureSnapshot) {
	s.mu.Lock()
	app := s.app
	s.mu.Unlock()
	if app == nil {
		return
	}
	app.Event.Emit("daq:payload", snapshot)
}

// emitLog 是 hub.EmitLog 的便捷包装，免去到处构造 LogEvent 字面量的样板。
func (s *DeviceService) emitLog(level, category, deviceID, source, message, detail string) {
	s.hub.EmitLog(core.LogEvent{
		Level:    level,
		Category: category,
		DeviceID: deviceID,
		Source:   source,
		Message:  message,
		Detail:   detail,
	})
}

// EmitDeviceState 实现 core.StateEmitter 接口（ACQ-010/STB-003）。
//
// 由 adapter 在 OnReadLoopExit 等异步状态变化时通过 hub.EmitDeviceState 触发，
// 本方法将状态通过 Wails Event 总线广播为 daq:device-state 事件，
// 前端 App.vue 中 onDeviceState 订阅器接收后调用 syncStatusFromBackend，
// 让 statusMap 实时同步（如物理断网后从「采集中」直接变为「未连接」）。
//
// 实现轻量：仅做一次 Event.Emit，无锁、无 I/O，可在 adapter 持锁回调中安全调用。
func (s *DeviceService) EmitDeviceState(deviceID string, state core.DeviceState) {
	s.mu.Lock()
	app := s.app
	s.mu.Unlock()
	if app == nil {
		return
	}
	app.Event.Emit("daq:device-state", deviceID, state)
}
