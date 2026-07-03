package backend

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"daq-p1604/core"
	"daq-p1604/usecase"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	// recordingStatusEmitInterval 录制状态 emit 间隔（仅状态变更通知，不传数据）
	recordingStatusEmitInterval = time.Second
)

// App Wails 后端应用
type App struct {
	ctx      context.Context
	cancel   context.CancelFunc
	deviceUC *usecase.DeviceUsecase
	recordUC *usecase.RecordingUsecase
	logUC    *usecase.LogUsecase
	logDir   string
	app      *application.App
	mu       sync.Mutex
	relays   map[string]*relayControl

	// latestSnapshots 各设备最新快照（前端轮询用，避免 Event.Emit 触发 WebView2 同步阻塞）
	latestMu         sync.RWMutex
	latestSnapshots  map[string]core.PressureSnapshot
}

// relayControl 采集数据中继控制
type relayControl struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// LogEvent 日志事件
type LogEvent struct {
	Level     string `json:"level"`
	Category  string `json:"category"`
	DeviceID  string `json:"deviceId,omitempty"`
	Source    string `json:"source"`
	Message   string `json:"message"`
	Detail    string `json:"detail,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// LogFileState 日志文件写入状态
type LogFileState struct {
	Active    bool   `json:"active"`
	OutputDir string `json:"outputDir,omitempty"`
}

// RecordingWarningEvent 录制期间的设备健康度警告事件载荷。
// 用于多设备录制场景下某台设备断连时通知前端，与 RecordingSession 状态变更分离，
// 避免前端混淆"录制真的停了"和"只是有设备掉线"。
type RecordingWarningEvent struct {
	DeviceID  string `json:"deviceId"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

// NewApp 创建后端应用
func NewApp(deviceUC *usecase.DeviceUsecase, recordUC *usecase.RecordingUsecase, logUC *usecase.LogUsecase, logDir string) *App {
	return &App{
		deviceUC:        deviceUC,
		recordUC:        recordUC,
		logUC:           logUC,
		logDir:          logDir,
		relays:          make(map[string]*relayControl),
		latestSnapshots: make(map[string]core.PressureSnapshot),
	}
}

// ServiceStartup 应用启动回调
func (a *App) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.app = application.Get()

	// 自动启动日志文件写入
	if a.logDir != "" {
		if err := a.logUC.Start(a.logDir, "daq-log"); err != nil {
			slog.Error("自动启动日志文件写入失败", "error", err)
		} else {
			slog.Info("日志文件自动保存已开启", "dir", a.logDir)
		}
	}

	slog.Info("DAQ-P-1604 application started")
	a.EmitLog(LogEvent{
		Level:    "info",
		Category: "system",
		Source:   "app",
		Message:  "DAQ-P-1604 application started",
	})
	return nil
}

// ServiceShutdown 应用关闭回调
func (a *App) ServiceShutdown() error {
	_ = a.recordUC.Stop()
	_ = a.logUC.Stop()
	a.stopAllRelays()
	if a.cancel != nil {
		a.cancel()
	}
	a.EmitLog(LogEvent{
		Level:    "info",
		Category: "system",
		Source:   "app",
		Message:  "DAQ-P-1604 application shut down",
	})
	slog.Info("DAQ-P-1604 application shut down")
	return nil
}

// EmitLog 发送日志事件
func (a *App) EmitLog(entry LogEvent) {
	if entry.Timestamp == 0 {
		entry.Timestamp = core.TimestampMs()
	}
	if entry.Source == "" {
		entry.Source = "backend"
	}

	// 同步写入日志文件
	if a.logUC != nil && a.logUC.IsActive() {
		if err := a.logUC.Write(entry.Timestamp, entry.Level, entry.Category, entry.DeviceID, entry.Source, entry.Message, entry.Detail); err != nil {
			slog.Error("写入日志文件失败", "error", err)
		}
	}

	if a.app == nil {
		return
	}
	a.app.Event.Emit("daq:log", entry)
}

func (a *App) emitRecordingStatus(session core.RecordingSession) {
	if a.app == nil {
		return
	}
	a.app.Event.Emit("daq:recording-status", session)
}

// emitRecordingWarning 推送设备断连警告事件到前端。
// 用于多设备录制场景下某台设备断连时通知前端展示黄色警告条，
// 其他设备继续录制。同时写入日志便于排查。
func (a *App) emitRecordingWarning(deviceID, msg string) {
	if a.app != nil {
		a.app.Event.Emit("daq:recording-warning", RecordingWarningEvent{
			DeviceID:  deviceID,
			Message:   msg,
			Timestamp: core.TimestampMs(),
		})
	}
	a.EmitLog(LogEvent{
		Level:    "warn",
		Category: "recording",
		DeviceID: deviceID,
		Source:   "recording",
		Message:  msg,
	})
}

// EmitDeviceState 发送设备状态变更事件到前端
func (a *App) EmitDeviceState(id string, state core.DeviceState) {
	if a.app == nil {
		return
	}
	a.app.Event.Emit("daq:device-state", id, state)
}

// ScanDevices 扫描设备
func (a *App) ScanDevices() ([]core.ScanResult, error) {
	return a.deviceUC.ScanDevices()
}

// GetProfiles 获取所有设备配置
func (a *App) GetProfiles() []core.PressureProfile {
	return a.deviceUC.GetProfiles()
}

// UpsertProfile 保存设备配置
func (a *App) UpsertProfile(profile core.PressureProfile) error {
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: profile.ID, Source: "config", Message: "保存设备配置"})
	if err := a.deviceUC.UpsertProfile(profile); err != nil {
		a.EmitLog(LogEvent{Level: "error", Category: "system", DeviceID: profile.ID, Source: "config", Message: "保存设备配置失败", Detail: err.Error()})
		return err
	}
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: profile.ID, Source: "config", Message: "设备配置已保存"})
	return nil
}

// DeleteProfile 删除设备配置
func (a *App) DeleteProfile(id string) error {
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: id, Source: "config", Message: "删除设备配置"})
	if err := a.deviceUC.DeleteProfile(id); err != nil {
		a.EmitLog(LogEvent{Level: "error", Category: "system", DeviceID: id, Source: "config", Message: "删除设备配置失败", Detail: err.Error()})
		return err
	}
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: id, Source: "config", Message: "设备配置已删除"})
	return nil
}

// Connect 连接设备
func (a *App) Connect(id string) error {
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: id, Source: "device", Message: "Connect requested"})
	if err := a.deviceUC.Connect(id); err != nil {
		a.EmitLog(LogEvent{Level: "error", Category: "system", DeviceID: id, Source: "device", Message: "Connect failed", Detail: err.Error()})
		return err
	}
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: id, Source: "device", Message: "Device connected"})
	return nil
}

// Disconnect 断开设备
func (a *App) Disconnect(id string) error {
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: id, Source: "device", Message: "Disconnect requested"})
	if err := a.deviceUC.Disconnect(id); err != nil {
		a.EmitLog(LogEvent{Level: "error", Category: "system", DeviceID: id, Source: "device", Message: "Disconnect failed", Detail: err.Error()})
		return err
	}
	a.waitRelay(id)
	// 清理最新快照缓存
	a.latestMu.Lock()
	delete(a.latestSnapshots, id)
	a.latestMu.Unlock()
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: id, Source: "device", Message: "Device disconnected"})
	return nil
}

// StartAcquisition 启动采集
func (a *App) StartAcquisition(id string) error {
	a.EmitLog(LogEvent{Level: "info", Category: "acquisition", DeviceID: id, Source: "device", Message: "Start acquisition requested"})
	ch, err := a.deviceUC.StartAcquisition(id)
	if err != nil {
		a.EmitLog(LogEvent{Level: "error", Category: "acquisition", DeviceID: id, Source: "device", Message: "Start acquisition failed", Detail: err.Error()})
		return err
	}
	a.startRelay(id, ch)
	a.EmitLog(LogEvent{Level: "info", Category: "acquisition", DeviceID: id, Source: "device", Message: "Acquisition started"})
	return nil
}

// StopAcquisition 停止采集
func (a *App) StopAcquisition(id string) error {
	a.EmitLog(LogEvent{Level: "info", Category: "acquisition", DeviceID: id, Source: "device", Message: "Stop acquisition requested"})
	if err := a.deviceUC.StopAcquisition(id); err != nil {
		a.EmitLog(LogEvent{Level: "error", Category: "acquisition", DeviceID: id, Source: "device", Message: "Stop acquisition failed", Detail: err.Error()})
		return err
	}
	a.waitRelay(id)
	a.EmitLog(LogEvent{Level: "info", Category: "acquisition", DeviceID: id, Source: "device", Message: "Acquisition stopped"})
	return nil
}

// GetStatus 获取设备状态
func (a *App) GetStatus(id string) (core.DeviceState, bool) {
	return a.deviceUC.GetStatus(id)
}

// ApplyConfig 应用设备配置
func (a *App) ApplyConfig(id string, cfg core.P1604Config) error {
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: id, Source: "device", Message: "Apply config requested"})
	if err := a.deviceUC.ApplyConfig(id, cfg); err != nil {
		a.EmitLog(LogEvent{Level: "error", Category: "system", DeviceID: id, Source: "device", Message: "Apply config failed", Detail: err.Error()})
		return err
	}
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: id, Source: "device", Message: "Config applied"})
	return nil
}

// GetLatestSnapshot 获取指定设备的最新快照（前端 500ms 轮询调用）
// 替代原有的 daq:payload Event.Emit，避免 Wails v3 Event.Emit 触发
// WebView2 同步 ExecuteScript 调用导致的 GUI 线程阻塞和 Eval errors。
func (a *App) GetLatestSnapshot(id string) (core.PressureSnapshot, bool) {
	a.latestMu.RLock()
	defer a.latestMu.RUnlock()
	s, ok := a.latestSnapshots[id]
	return s, ok
}

// GetLatestSnapshots 批量获取所有设备的最新快照（减少前端轮询次数）
func (a *App) GetLatestSnapshots() map[string]core.PressureSnapshot {
	a.latestMu.RLock()
	defer a.latestMu.RUnlock()
	result := make(map[string]core.PressureSnapshot, len(a.latestSnapshots))
	for k, v := range a.latestSnapshots {
		result[k] = v
	}
	return result
}

func (a *App) startRelay(deviceID string, ch <-chan core.PressureSnapshot) {
	baseCtx := a.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(baseCtx)
	control := &relayControl{cancel: cancel, done: make(chan struct{})}

	a.mu.Lock()
	if oldControl, ok := a.relays[deviceID]; ok {
		oldControl.cancel()
	}
	a.relays[deviceID] = control
	a.mu.Unlock()

	go func() {
		defer close(control.done)
		// handleRelayExit 必须在 clearRelay 之后执行（defer 是 LIFO）：
		// 此时 relays[deviceID] 已被删除，len(a.relays) 反映的是其他活跃设备数，
		// 据此判定单设备场景（len==0，自动停止录制）或多设备场景（len>0，emit 警告）。
		defer a.handleRelayExit(deviceID)
		defer a.clearRelay(deviceID, control)
		a.relayStream(ctx, deviceID, ch)
	}()
}

func (a *App) waitRelay(deviceID string) {
	a.mu.Lock()
	control := a.relays[deviceID]
	a.mu.Unlock()
	if control != nil {
		<-control.done
	}
}

func (a *App) stopAllRelays() {
	a.mu.Lock()
	relays := a.relays
	a.relays = make(map[string]*relayControl)
	a.mu.Unlock()
	for _, control := range relays {
		control.cancel()
	}
}

func (a *App) clearRelay(deviceID string, control *relayControl) {
	a.mu.Lock()
	if a.relays[deviceID] == control {
		delete(a.relays, deviceID)
	}
	a.mu.Unlock()
}

// relayStream 中继数据流：从设备 channel 读取快照，更新最新快照缓存，异步投递到录制器
//
// 与原实现的关键差异：
//  1. 移除 daq:payload Event.Emit（改为前端 500ms 轮询 GetLatestSnapshot）
//  2. recordUC.Write 改为异步非阻塞投递（recorder 内部 queue chan + select default）
//  3. 仅保留最新快照在 latestSnapshots，前端按需轮询
//  4. 跟踪 recordingActive 状态，检测 recorder auto-stop（I/O 错误）时立即推送最终状态，
//     避免 IsActive() 变 false 后 statusTicker 不再推送导致前端状态与后端不同步。
//
// defer emit 策略：设备处于 StatusError（断连）时跳过 emit，由 handleRelayExit 统一推送
// 最终状态（Idle + LastError）。避免"先 Active 后 Idle+Error"的双 emit 导致前端 UI 闪烁。
// 非断连退出（主动 StopAcquisition/Disconnect）仍由此处 emit，此时录制状态无异常变更。
func (a *App) relayStream(ctx context.Context, deviceID string, ch <-chan core.PressureSnapshot) {
	statusTicker := time.NewTicker(recordingStatusEmitInterval)
	defer statusTicker.Stop()

	defer func() {
		// 设备断连：由 handleRelayExit 负责最终状态推送，此处跳过避免双 emit 闪烁
		if st, ok := a.deviceUC.GetStatus(deviceID); ok && st.Status == core.StatusError {
			return
		}
		a.emitRecordingStatus(a.recordUC.Status())
	}()

	// 跟踪录制活跃状态，检测 auto-stop（I/O 错误后 started=false）
	recordingActive := a.recordUC.IsActive()

	for {
		select {
		case <-ctx.Done():
			return
		case snapshot, ok := <-ch:
			if !ok {
				return
			}
			// 更新最新快照缓存（前端轮询读取）
			a.latestMu.Lock()
			a.latestSnapshots[deviceID] = snapshot
			a.latestMu.Unlock()

			// 异步投递到录制器（非阻塞，队列满时丢弃并计数）
			// 用无锁 IsActive() 判活，避免每帧 Status() 与 writer goroutine 争用 statsMu
			if a.recordUC.IsActive() {
				recordingActive = true // 同步：录制已开始（覆盖初始 false）
				if err := a.recordUC.Write(snapshot); err != nil {
					a.EmitLog(LogEvent{Level: "error", Category: "acquisition", DeviceID: deviceID, Source: "recording", Message: "Record snapshot failed", Detail: err.Error()})
				}
			} else if recordingActive {
				// 检测到 recorder auto-stop（I/O 错误）：立即推送最终状态，
				// 让前端及时停止录制显示并展示 lastError
				recordingActive = false
				a.emitRecordingStatus(a.recordUC.Status())
			}
		case <-statusTicker.C:
			nowActive := a.recordUC.IsActive()
			if nowActive {
				recordingActive = true // 同步：录制已开始（覆盖初始 false）
				a.emitRecordingStatus(a.recordUC.Status())
			} else if recordingActive {
				// 周期检测到 auto-stop（防止上面 case 路径漏触发）：补推最终状态
				recordingActive = false
				a.emitRecordingStatus(a.recordUC.Status())
			}
		}
	}
}

// handleRelayExit 在 relay goroutine 退出时（clearRelay 之后）判定是否需要
// 因设备断连而自动停止录制或 emit 警告。
//
// 判定逻辑：
//  1. 检查设备状态：仅当设备处于 StatusError（断连）时才触发自动停止/警告。
//     主动 StopAcquisition/Disconnect 不会触发（设备状态为 Connected/Disconnected）。
//  2. 检查录制是否活跃：录制未启动则无需处理。
//  3. 检查剩余 relay 数：
//     - len==0：单设备场景或所有设备都断连 → 自动停止录制，LastError 填充断连原因
//     - len>0：多设备场景 → emit 警告事件，其他设备继续录制
//
// 竞态保护：用户主动 StopRecording 与此处自动 Stop 竞争时，recorder.StopWithError
// 内部 CompareAndSwap 保护，CAS 失败方静默返回 false 且不修改 LastError，
// 避免把"用户主动停止"误覆盖为"设备断连自动停止"。
//
// 阻塞耗时说明：StopWithError 内部 close(stopCh) 后同步等待 writer goroutine
// drain 队列 + fsync 落盘，单设备断连场景下可能阻塞数百毫秒到秒级（取决于队列
// 积压量与磁盘 I/O）。此阻塞会延长 relay goroutine 生命周期（延迟 close(control.done)），
// 但保证了 CAS 成功后才 emit 最终状态的一致性，可接受。
func (a *App) handleRelayExit(deviceID string) {
	// 仅当设备处于 Error 状态（断连）时才触发自动停止/警告
	st, ok := a.deviceUC.GetStatus(deviceID)
	if !ok || st.Status != core.StatusError {
		return
	}

	// 录制未启动则无需处理
	if !a.recordUC.IsActive() {
		return
	}

	a.mu.Lock()
	remainingRelays := len(a.relays)
	a.mu.Unlock()

	if remainingRelays == 0 {
		// 单设备场景或所有设备都断连：自动停止录制。
		// StopWithError CAS 成功才写 LastError，避免覆盖用户已主动停止的状态。
		msg := fmt.Sprintf("因设备 %s 断连自动停止录制", deviceID)
		if a.recordUC.StopWithError(msg) {
			// 推送最终录制状态，让前端感知录制已停止并展示 LastError
			a.emitRecordingStatus(a.recordUC.Status())
		}
		// CAS 失败：用户已主动 StopRecording，不再 emit 状态（避免重复推送）
	} else {
		// 多设备场景：emit 警告，继续录制其他设备
		a.emitRecordingWarning(deviceID, fmt.Sprintf("设备 %s 断连，其他设备继续录制", deviceID))
	}
}

// StartRecording 开始录制
// 多设备精度合并：聚合所有已配置设备的通道精度，避免仅用第一个设备导致精度错误。
func (a *App) StartRecording(outputDir string, filePrefix string) error {
	return a.StartRecordingWithConfig(outputDir, filePrefix, core.FileRotation{}, core.StopConditions{})
}

// StartRecordingWithConfig 开始录制（带完整滚动与停止条件配置）
func (a *App) StartRecordingWithConfig(outputDir string, filePrefix string, rotation core.FileRotation, stopCond core.StopConditions) error {
	// 聚合所有已配置设备的通道精度：多设备时取每通道精度的最大值，确保不丢失精度
	// 若所有设备均未配置某通道精度，回退到默认值（由 recorder 内部处理）
	mergedChannels := mergeChannelPrecisions(a.deviceUC.GetProfiles())

	cfg := core.RecordingConfig{
		OutputDir:      outputDir,
		FilePrefix:     filePrefix,
		Channels:       mergedChannels,
		Rotation:       rotation,
		StopConditions: stopCond,
	}
	if err := a.recordUC.Start(cfg); err != nil {
		return err
	}
	a.emitRecordingStatus(a.recordUC.Status())
	return nil
}

// mergeChannelPrecisions 合并多设备通道精度配置
// 策略：以所有设备中最大的通道数作为模板长度，每通道取所有设备中配置的最大精度
// （保留更多有效位）。设备通道数不一致时（混合设备类型）按索引对齐，缺该通道的
// 设备不参与该通道的精度比较；未配置精度的通道由 recorder 回退到默认值。
func mergeChannelPrecisions(profiles []core.PressureProfile) []core.ChannelConfig {
	if len(profiles) == 0 {
		return nil
	}
	maxLen := 0
	for _, p := range profiles {
		if len(p.Channels) > maxLen {
			maxLen = len(p.Channels)
		}
	}
	if maxLen == 0 {
		return nil
	}
	// 以第一个设备的通道结构作为模板基础（含通道名/单位等元信息），缺位补零值
	merged := make([]core.ChannelConfig, maxLen)
	if t := profiles[0].Channels; len(t) > 0 {
		copy(merged, t)
	}

	// 对每通道取所有设备中精度最大值
	for i := range merged {
		maxPrecision := merged[i].Precision
		for _, p := range profiles[1:] {
			if i < len(p.Channels) && p.Channels[i].Precision > maxPrecision {
				maxPrecision = p.Channels[i].Precision
			}
		}
		merged[i].Precision = maxPrecision
	}
	return merged
}

// StopRecording 停止录制
func (a *App) StopRecording() error {
	if err := a.recordUC.Stop(); err != nil {
		return err
	}
	a.emitRecordingStatus(a.recordUC.Status())
	return nil
}

// GetRecordingStatus 获取录制状态
func (a *App) GetRecordingStatus() core.RecordingSession {
	return a.recordUC.Status()
}

// StartLogFile 开始将日志写入文件
func (a *App) StartLogFile(outputDir string, prefix string) error {
	if err := a.logUC.Start(outputDir, prefix); err != nil {
		return err
	}
	a.EmitLog(LogEvent{Level: "info", Category: "system", Source: "logging", Message: "日志文件保存已开启"})
	return nil
}

// StopLogFile 停止日志文件写入
func (a *App) StopLogFile() error {
	if err := a.logUC.Stop(); err != nil {
		return err
	}
	a.EmitLog(LogEvent{Level: "info", Category: "system", Source: "logging", Message: "日志文件保存已关闭"})
	return nil
}

// GetLogFileState 获取日志文件写入状态
func (a *App) GetLogFileState() LogFileState {
	return LogFileState{
		Active:    a.logUC.IsActive(),
		OutputDir: a.logUC.GetOutputDir(),
	}
}

// PickDirectory 选择目录对话框
func (a *App) PickDirectory() (string, error) {
	app := a.app
	if app == nil {
		app = application.Get()
	}
	return app.Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true).
		SetTitle("选择保存目录").
		PromptForSingleSelection()
}
