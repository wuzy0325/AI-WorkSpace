package backend

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"wista/core"
	"wista/usecase"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// 前端推送节奏默认值：UI 快照 100ms（10Hz）一次，录制状态 1s 一次。
// UI 推送间隔可由前端通过 SetUIRefreshRateHz 动态调整；
// 录制状态间隔固定，与采集帧率解耦。
const (
	uiPayloadRefreshDefault    = 100 * time.Millisecond
	recordingStatusEmitInterval = time.Second

	// UI 刷新率合法范围（Hz）。
	// 下限 1Hz 避免图表长时间不更新误导用户；
	// 上限 60Hz 保护 WebView2 GUI 线程，与 displayStore 限制一致。
	uiRefreshRateMinHz = 1
	uiRefreshRateMaxHz = 60
)

// DeviceService 暴露设备相关能力给前端：
//   - 扫描 / 配置 CRUD
//   - 连接 / 断开 / 应用配置
//   - 启动 / 停止采集（带后台中继协程，按用户设置的刷新率推送温度快照、按 1s 频率推送录制状态）
//   - 设置 UI 刷新率（SetUIRefreshRateHz，动态调整推送节奏，无需重启采集）
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

	// uiRefreshInterval UI 快照推送间隔（纳秒）。
	// 由 SetUIRefreshRateHz 更新；relayStream 在 uiTicker.C 分支触发时
	// 通过 lastInterval 缓存对比检测变化并 Reset ticker。
	// 用 atomic 而非 mutex：relayStream 是热路径，避免持锁阻塞；
	// 用户调刷新率是低频操作，竞态可接受（读到旧值最多多等一个旧间隔）。
	uiRefreshInterval atomic.Int64

	mu  sync.Mutex
	app *application.App

	// userConfirmedExit 用户已确认退出的标志位
	// RegisterExitConfirmationHook 内 hook 检查该标志：
	//   - false → event.Cancel() 阻止默认关闭 listener，并 EmitEvent 通知前端弹确认对话框
	//   - true  → 放行，默认 listener 真正关闭窗口
	// 由 RequestExit binding 在用户确认后置 true，避免 hook 二次触发时再次 Cancel 导致死循环
	userConfirmedExit atomic.Bool
}

// NewDeviceService 创建设备 Service。
func NewDeviceService(hub *core.Hub, deviceUC *usecase.DeviceUsecase, recording *RecordingService) *DeviceService {
	s := &DeviceService{
		hub:       hub,
		deviceUC:  deviceUC,
		recording: recording,
	}
	// 默认 100ms（10Hz），与前端 displayStore 默认值对齐。
	// 启动后前端 App.onMounted 会再次调用 SetUIRefreshRateHz 同步 localStorage 保存的偏好。
	s.uiRefreshInterval.Store(int64(uiPayloadRefreshDefault))
	return s
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
// 同时清空 s.app，让后续可能到达的 EmitDeviceState（adapter readLoop 收尾时）
// 走 app == nil 早退分支，避免 Event.Emit 在已关闭 app 上 panic。
func (s *DeviceService) ServiceShutdown() error {
	s.hub.StopAllRelays()
	s.mu.Lock()
	s.app = nil
	s.mu.Unlock()
	return nil
}

// ----------------------------------------------------------------------------
// 前端可调用方法（被 wails3 generate bindings 扫描后生成对应 TS 函数）
// ----------------------------------------------------------------------------

// SetUIRefreshRateHz 设置 UI 快照推送频率（Hz）。
//
// 设计意图：
//   - 前端 MainTopBar 提供 2/5/10/15/20/30 Hz 档位，让用户平衡"画面流畅度"与"CPU 占用"；
//   - 后端 relayStream 按此频率通过 daq:payload 事件推送最新快照；
//   - relayStream 在下次 uiTicker.C 触发时检测 atomic 变化并 Reset ticker，
//     最长等一个旧间隔生效（如 10Hz→2Hz 最长 100ms+500ms=600ms），无需重启采集。
//
// 范围校验：[1, 60] Hz。
//   - 下限 1Hz：低于 1Hz 会让图表长时间不更新，误导用户以为采集卡死；
//   - 上限 60Hz：超过 60Hz 会拖慢 WebView2 GUI 线程，与前端 displayStore 限制一致。
//
// 该方法只更新 atomic 值，不持任何锁，可安全在 Wails Bind 调用栈中使用。
func (s *DeviceService) SetUIRefreshRateHz(hz int) error {
	if hz < uiRefreshRateMinHz || hz > uiRefreshRateMaxHz {
		return fmt.Errorf("UI refresh rate %d Hz out of range [%d, %d]", hz, uiRefreshRateMinHz, uiRefreshRateMaxHz)
	}
	intervalNs := int64(time.Second) / int64(hz)
	s.uiRefreshInterval.Store(intervalNs)
	slog.Debug("UI refresh rate updated", "hz", hz, "intervalMs", intervalNs/int64(time.Millisecond))
	return nil
}

// RegisterExitConfirmationHook 注册主窗口的 WindowClosing hook，
// 拦截 X 按钮关闭流程：未确认时取消默认关闭并向前端推送确认请求事件。
func (s *DeviceService) RegisterExitConfirmationHook(win *application.WebviewWindow) {
	win.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if s.userConfirmedExit.Load() {
			return
		}
		event.Cancel()
		go func() {
			app := application.Get()
			if app == nil {
				return
			}
			app.Event.Emit("app:exit-requested")
		}()
	})
}

// RequestExit 由前端在用户确认退出对话框后调用。
func (s *DeviceService) RequestExit() error {
	s.userConfirmedExit.Store(true)
	s.mu.Lock()
	app := s.app
	s.mu.Unlock()
	if app == nil {
		app = application.Get()
	}
	if app == nil {
		return fmt.Errorf("application not initialized")
	}
	app.Quit()
	return nil
}

// ExitApplication 主动退出应用（保留供程序化调用，非 UI 主路径）。
func (s *DeviceService) ExitApplication() error {
	s.emitLog("info", "system", "", "app", "WISTA application exit requested by user", "")
	s.mu.Lock()
	app := s.app
	s.mu.Unlock()
	if app == nil {
		app = application.Get()
	}
	if app == nil {
		return fmt.Errorf("application not initialized")
	}
	// Quit() 内部会按注册顺序触发各 Service 的 ServiceShutdown，
	// 与用户点击原生 X 按钮的退出路径完全等价。
	app.Quit()
	return nil
}

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
		defer func() {
			if r := recover(); r != nil {
				slog.Warn("relay goroutine panic recovered", "deviceId", deviceID, "panic", r)
			}
		}()
		defer close(control.Done)
		defer s.hub.ClearRelay(deviceID, control)
		s.relayStream(ctx, deviceID, ch)
	}()
}

// relayStream 中继协程主循环。
//
// 节流策略：
//   - UI payload: 按 s.uiRefreshInterval 间隔推送最新值，间隔由前端 SetUIRefreshRateHz 动态调整；
//   - 录制写入: 直接逐帧写入（hot path），通过 atomic.Bool 判活无锁；
//   - 录制状态: 每 1s 广播一次（statusTicker），且仅在 IsActive 时广播。
//
// 动态刷新率实现：
//   - 用 Ticker 而非 time.After：Ticker 在独立 goroutine 计时，不受 ch 数据流影响；
//     time.After 是 case 表达式，每轮 select 重新求值，高频 ch 场景下永远到不了指定时长。
//   - 在 uiTicker.C 分支检测 atomic 变化并 Reset：用户切换刷新率后，最长等一个旧间隔
//     即生效（如 10Hz→2Hz 最长 100ms+500ms=600ms）。
func (s *DeviceService) relayStream(ctx context.Context, deviceID string, ch <-chan core.TemperatureSnapshot) {
	// lastInterval 缓存上次使用的间隔，用于检测 SetUIRefreshRateHz 引起的变化。
	// 读取一次 atomic 即可，后续在 select 循环内对比，避免每帧 Load。
	lastInterval := s.uiRefreshInterval.Load()
	uiTicker := time.NewTicker(time.Duration(lastInterval))
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
			// 动态调整：检测 atomic 变化并 Reset ticker。
			// Reset 后下一 tick 按新间隔计算，从现在起重新计时。
			if cur := s.uiRefreshInterval.Load(); cur != lastInterval {
				lastInterval = cur
				uiTicker.Reset(time.Duration(cur))
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
	// 与 EmitDeviceState 一致：Wails app 关闭/重启时 Event.Emit 可能 panic，
	// recover 避免 panic 终止 relay/readLoop 清理流程。
	defer func() {
		if r := recover(); r != nil {
			slog.Debug("emitPayload recovered from panic", "panic", r)
		}
	}()
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
// 实现轻量：仅持 s.mu 读取 app 字段后立即释放，无 I/O 阻塞，
// 可在 adapter 持锁回调中安全调用（s.mu 与 adapter 的 a.mu 是不同锁，无嵌套死锁风险）。
// 加 recover 保护：应用退出阶段 app 可能已关闭，Event.Emit 在已关闭的 app 上可能 panic，
// recover 避免 panic 终止 adapter readLoop 的清理流程；
// recover 内记录 slog.Debug 保留可观测性，避免静默吞掉真实 bug（如 state 字段 nil）。
func (s *DeviceService) EmitDeviceState(deviceID string, state core.DeviceState) {
	s.mu.Lock()
	app := s.app
	s.mu.Unlock()
	if app == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			slog.Debug("EmitDeviceState recovered from panic",
				"deviceId", deviceID, "panic", r)
		}
	}()
	app.Event.Emit("daq:device-state", deviceID, state)
}
