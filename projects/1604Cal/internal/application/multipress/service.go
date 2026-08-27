package multipress

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"cal1604/internal/application/session"
	"cal1604/internal/device"
	"cal1604/internal/domain"
)

// DevicePressureState 单台打压设备的运行状态。
type DevicePressureState struct {
	DeviceID        string  `json:"deviceId"`
	CurrentPressure float64 `json:"currentPressure"`
	TargetPressure  float64 `json:"targetPressure"`
	Unit            string  `json:"unit"`
	Stable          bool    `json:"stable"`
	Status          string  `json:"status"` // idle, pressurizing, exhausting, error
	ErrorMessage    string  `json:"errorMessage,omitempty"`
}

// StatusPublisher 广播事件到 SSE 通道。
type StatusPublisher func(eventType string, data any)

// lastPublished 上次已推送到 SSE 的 pressure.update 快照：
// 轮询值无变化时不重复发布，避免空闲期空转事件放大前端渲染与分配压力。
type lastPublished struct {
	sent     bool
	pressure float64
	stable   bool
	status   string
}

// deviceEntry 已注册设备的驱动实例与运行状态。
type deviceEntry struct {
	driver            device.PressureDriver
	state             DevicePressureState
	mu                sync.Mutex // 串行化单台设备的命令，多设备可并行
	consecutiveErrors int        // 连续轮询失败次数，>=3 时自动标记断连
	lastPub           lastPublished
}

// Service 多设备打压控制服务。
// 每台设备独立运行，无需会话状态机。
type Service struct {
	mu            sync.Mutex
	entries       map[string]*deviceEntry // deviceID -> entry
	factory       device.DriverFactory
	deviceManager device.DeviceStore
	publish       StatusPublisher

	pollCancel context.CancelFunc
	pollDone   chan struct{}
}

// NewService 创建多设备打压控制服务。
func NewService(
	factory device.DriverFactory,
	deviceManager device.DeviceStore,
	publisher StatusPublisher,
) *Service {
	if publisher == nil {
		publisher = func(string, any) {}
	}
	s := &Service{
		entries:       make(map[string]*deviceEntry),
		factory:       factory,
		deviceManager: deviceManager,
		publish:       publisher,
	}
	return s
}

// StartPolling 启动后台轮询，定期读取所有打压中设备的压力和稳定状态。
// 调用方应在服务就绪后调用一次，StopPolling 在服务关闭时调用。
func (s *Service) StartPolling() {
	s.mu.Lock()
	if s.pollCancel != nil {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.pollCancel = cancel
	s.pollDone = done
	s.mu.Unlock()

	go s.pollLoop(ctx, done)
}

// StopPolling 停止后台轮询。
func (s *Service) StopPolling() {
	s.mu.Lock()
	cancel := s.pollCancel
	done := s.pollDone
	s.pollCancel = nil
	s.pollDone = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
		if done != nil {
			<-done
		}
	}
}

// RegisterDevice 注册打压设备：查设备配置 -> 创建驱动 -> 连接 -> 存入活跃列表。
func (s *Service) RegisterDevice(ctx context.Context, deviceID string) error {
	dev, ok := s.deviceManager.Get(deviceID)
	if !ok {
		return fmt.Errorf("device %s not found", deviceID)
	}

	pDrv, err := s.factory.CreatePressureDriver(dev)
	if err != nil {
		return fmt.Errorf("create pressure driver: %w", err)
	}

	if err := pDrv.Connect(ctx); err != nil {
		return fmt.Errorf("connect device %s: %w", deviceID, err)
	}

	// 读取当前单位并规范化大小写
	unit := device.NormalizePressureUnit(dev.Unit)
	readUnit := ""
	if u, err := pDrv.ReadUnit(ctx); err == nil && u != "" {
		unit = u
		readUnit = u
	}
	log.Printf("[multipress.RegisterDevice] %s model=%q configUnit=%q readUnit=%q final=%q", deviceID, dev.Model, dev.Unit, readUnit, unit)

	entry := &deviceEntry{
		driver: pDrv,
		state: DevicePressureState{
			DeviceID: deviceID,
			Unit:     unit,
			Status:   "idle",
		},
	}

	s.mu.Lock()
	s.entries[deviceID] = entry
	s.mu.Unlock()

	s.deviceManager.UpdateStatus(deviceID, domain.DeviceStatusConnected)
	s.deviceManager.UpdateUnit(deviceID, unit)

	s.publish("multipress.device.registered", map[string]any{
		"deviceId": deviceID,
		"name":     dev.Name,
		"model":    dev.Model,
	})

	return nil
}

// UnregisterDevice 注销打压设备：停止 -> 断开 -> 从活跃列表移除。
// 幂等：设备未注册时视为已断开，仅同步 DeviceManager 状态，
// 避免设备管理模块已断开后，业务模块再次点击断开报 "not registered"。
func (s *Service) UnregisterDevice(ctx context.Context, deviceID string) error {
	s.mu.Lock()
	entry, ok := s.entries[deviceID]
	s.mu.Unlock()

	if !ok {
		s.deviceManager.UpdateStatus(deviceID, domain.DeviceStatusDisconnected)
		s.deviceManager.UpdateUnit(deviceID, "")
		return nil
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()

	_ = entry.driver.Stop(ctx)
	_ = entry.driver.Disconnect(ctx)

	s.mu.Lock()
	delete(s.entries, deviceID)
	s.mu.Unlock()

	s.deviceManager.UpdateStatus(deviceID, domain.DeviceStatusDisconnected)
	s.deviceManager.UpdateUnit(deviceID, "")

	s.publish("multipress.device.unregistered", map[string]any{
		"deviceId": deviceID,
	})

	return nil
}

// SetTargetPressure 设置指定设备的目标压力并启动压力控制。
func (s *Service) SetTargetPressure(ctx context.Context, deviceID string, target float64) error {
	entry, err := s.getEntry(deviceID)
	if err != nil {
		return err
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()

	if err := entry.driver.SetTargetPressure(ctx, target); err != nil {
		entry.state.Status = "error"
		entry.state.ErrorMessage = err.Error()
		s.publish("multipress.device.status", map[string]any{
			"deviceId":     deviceID,
			"status":       "error",
			"errorMessage": err.Error(),
		})
		return fmt.Errorf("set target pressure: %w", err)
	}

	// 启动压力控制（ConST 系列支持 StartControl）
	if ctrl, ok := entry.driver.(device.PressureControlCapable); ok {
		if err := ctrl.StartControl(ctx); err != nil {
			entry.state.Status = "error"
			entry.state.ErrorMessage = err.Error()
			return fmt.Errorf("start pressure control: %w", err)
		}
	}

	entry.state.TargetPressure = target
	entry.state.Status = "pressurizing"
	entry.state.ErrorMessage = ""

	s.publish("multipress.pressure.changed", map[string]any{
		"deviceId":       deviceID,
		"targetPressure": target,
	})

	return nil
}

// Stop 停止指定设备的压力控制。
func (s *Service) Stop(ctx context.Context, deviceID string) error {
	entry, err := s.getEntry(deviceID)
	if err != nil {
		return err
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()

	if err := entry.driver.Stop(ctx); err != nil {
		return fmt.Errorf("stop device %s: %w", deviceID, err)
	}

	entry.state.Status = "idle"
	entry.state.ErrorMessage = ""

	s.publish("multipress.device.status", map[string]any{
		"deviceId": deviceID,
		"status":   "idle",
	})

	return nil
}

// Exhaust 排空指定设备的压力。
func (s *Service) Exhaust(ctx context.Context, deviceID string) error {
	entry, err := s.getEntry(deviceID)
	if err != nil {
		return err
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()

	if err := entry.driver.Exhaust(ctx); err != nil {
		return fmt.Errorf("exhaust device %s: %w", deviceID, err)
	}

	entry.state.Status = "exhausting"
	entry.state.ErrorMessage = ""

	s.publish("multipress.device.status", map[string]any{
		"deviceId": deviceID,
		"status":   "exhausting",
	})

	return nil
}

// ReadCurrentPressure 读取指定设备的当前压力。
func (s *Service) ReadCurrentPressure(ctx context.Context, deviceID string) (float64, error) {
	entry, err := s.getEntry(deviceID)
	if err != nil {
		return 0, err
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()

	pressure, err := entry.driver.ReadCurrentPressure(ctx)
	if err != nil {
		return 0, fmt.Errorf("read pressure from %s: %w", deviceID, err)
	}

	entry.state.CurrentPressure = pressure
	return pressure, nil
}

// ReadStability 读取指定设备的稳定状态。
func (s *Service) ReadStability(ctx context.Context, deviceID string) (bool, error) {
	entry, err := s.getEntry(deviceID)
	if err != nil {
		return false, err
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()

	stable, err := entry.driver.ReadStability(ctx)
	if err != nil {
		return false, fmt.Errorf("read stability from %s: %w", deviceID, err)
	}

	entry.state.Stable = stable
	return stable, nil
}

// ReadUnit 读取指定设备的压力单位。
func (s *Service) ReadUnit(ctx context.Context, deviceID string) (string, error) {
	entry, err := s.getEntry(deviceID)
	if err != nil {
		return "", err
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()

	unit, err := entry.driver.ReadUnit(ctx)
	if err != nil {
		log.Printf("[multipress.ReadUnit] %s error: %v", deviceID, err)
		return "", fmt.Errorf("read unit from %s: %w", deviceID, err)
	}

	unit = device.NormalizePressureUnit(unit)
	entry.state.Unit = unit
	log.Printf("[multipress.ReadUnit] %s → %q", deviceID, unit)
	return unit, nil
}

// SetUnit 设置指定设备的压力单位，成功后立即重读压力值以显示新单位下的正确数值。
func (s *Service) SetUnit(ctx context.Context, deviceID string, unit string) error {
	entry, err := s.getEntry(deviceID)
	if err != nil {
		return err
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()

	if err := entry.driver.SetUnit(ctx, unit); err != nil {
		return fmt.Errorf("set unit on %s: %w", deviceID, err)
	}

	entry.state.Unit = device.NormalizePressureUnit(unit)

	// 同步单位到设备配置存储，保证单位一致性检查读取到最新设定值。
	if s.deviceManager != nil {
		s.deviceManager.UpdateUnit(deviceID, entry.state.Unit)
		log.Printf("[multipress.SetUnit] %s: unit=%q synced to device store", deviceID, entry.state.Unit)
	}

	// 切换单位后立即重读压力，确保显示值与新单位匹配
	if pressure, err := entry.driver.ReadCurrentPressure(ctx); err == nil {
		entry.state.CurrentPressure = pressure
		log.Printf("[multipress.SetUnit] %s: unit=%q re-read pressure=%f", deviceID, entry.state.Unit, pressure)
	} else {
		log.Printf("[multipress.SetUnit] %s: unit=%q re-read pressure error: %v", deviceID, entry.state.Unit, err)
	}

	return nil
}

// ListDeviceStates 返回所有已注册设备的状态快照。
func (s *Service) ListDeviceStates() []DevicePressureState {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]DevicePressureState, 0, len(s.entries))
	for _, entry := range s.entries {
		entry.mu.Lock()
		snapshot := entry.state
		entry.mu.Unlock()
		result = append(result, snapshot)
	}
	return result
}

// StopAll 紧急停止所有已注册设备。
func (s *Service) StopAll(ctx context.Context) error {
	s.mu.Lock()
	entries := make([]*deviceEntry, 0, len(s.entries))
	for _, entry := range s.entries {
		entries = append(entries, entry)
	}
	s.mu.Unlock()

	var firstErr error
	for _, entry := range entries {
		entry.mu.Lock()
		if err := entry.driver.Stop(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
		entry.state.Status = "idle"
		entry.mu.Unlock()
	}

	s.publish("multipress.device.status", map[string]any{
		"status": "all_stopped",
	})

	return firstErr
}

// getEntry 获取已注册设备的 entry（不加设备级锁）。
func (s *Service) getEntry(deviceID string) (*deviceEntry, error) {
	s.mu.Lock()
	entry, ok := s.entries[deviceID]
	s.mu.Unlock()

	if !ok {
		return nil, session.ErrDeviceNotFound
	}
	return entry, nil
}

// GetActiveDriver 返回指定设备的已连接驱动实例，供校准服务复用。
// 设备未注册时返回 nil。
func (s *Service) GetActiveDriver(id string) device.ConnectionDriver {
	s.mu.Lock()
	entry, ok := s.entries[id]
	s.mu.Unlock()
	if !ok || entry == nil {
		return nil
	}
	return entry.driver
}

// pollLoop 后台轮询所有打压中设备的压力和稳定状态。
func (s *Service) pollLoop(ctx context.Context, done chan<- struct{}) {
	defer close(done)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pollAllDevices(ctx)
		}
	}
}

// pollAllDevices 并发读取所有已注册设备的状态（含空闲设备，以便实时显示当前压力）。
func (s *Service) pollAllDevices(ctx context.Context) {
	s.mu.Lock()
	var targets []*deviceEntry
	for _, entry := range s.entries {
		entry.mu.Lock()
		status := entry.state.Status
		entry.mu.Unlock()
		if status != "" && status != "error" {
			targets = append(targets, entry)
		}
	}
	s.mu.Unlock()

	if len(targets) == 0 {
		return
	}

	results := s.pollDevicesConcurrently(ctx, targets)
	s.processPollResults(results)
}

// pollResult 单台设备的轮询结果。
type pollResult struct {
	deviceID string
	pressure float64
	stable   bool
	err      error
}

// pollDevicesConcurrently 并发读取所有目标设备的状态。
func (s *Service) pollDevicesConcurrently(ctx context.Context, targets []*deviceEntry) []pollResult {
	results := make([]pollResult, len(targets))
	var wg sync.WaitGroup
	for i, entry := range targets {
		wg.Add(1)
		go func(idx int, e *deviceEntry) {
			defer wg.Done()
			pollCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			pollCtx = device.WithPollContext(pollCtx)

			pressure, pErr := e.driver.ReadCurrentPressure(pollCtx)
			stable, sErr := e.driver.ReadStability(pollCtx)

			r := pollResult{deviceID: e.state.DeviceID}
			if pErr != nil {
				r.err = pErr
				log.Printf("[multipress.poll] %s ReadCurrentPressure error: %v", e.state.DeviceID, pErr)
			} else {
				r.pressure = pressure
			}
			if sErr == nil {
				r.stable = stable
			}
			results[idx] = r
		}(i, entry)
	}
	wg.Wait()
	return results
}

// processPollResults 处理轮询结果，更新设备状态并发布 SSE 事件。
func (s *Service) processPollResults(results []pollResult) {
	for _, r := range results {
		s.mu.Lock()
		entry, ok := s.entries[r.deviceID]
		s.mu.Unlock()
		if !ok {
			continue
		}

		status, shouldPublish := s.mergePollResult(entry, r)

		if shouldPublish {
			s.publish("multipress.pressure.update", map[string]any{
				"deviceId":        r.deviceID,
				"currentPressure": r.pressure,
				"stable":          r.stable,
				"status":          status,
			})
		}
	}
}

// mergePollResult 把单台设备的轮询结果合并进 entry 状态，
// 返回合并后的状态与是否需要发布 pressure.update（值无变化时去重不发布）。
func (s *Service) mergePollResult(entry *deviceEntry, r pollResult) (string, bool) {
	entry.mu.Lock()
	defer entry.mu.Unlock()

	if r.err != nil {
		entry.state.ErrorMessage = r.err.Error()
		entry.consecutiveErrors++
		if entry.consecutiveErrors >= 3 {
			entry.state.Status = "error"
			s.deviceManager.UpdateStatus(r.deviceID, domain.DeviceStatusError)
			s.publish("device.status.changed", map[string]any{
				"id":          r.deviceID,
				"type":        "pressure",
				"status":      string(domain.DeviceStatusError),
				"errorReason": r.err.Error(),
			})
			return entry.state.Status, false
		}
		return entry.state.Status, false
	}

	entry.consecutiveErrors = 0
	entry.state.CurrentPressure = r.pressure
	entry.state.Stable = r.stable

	if entry.state.Status == "exhausting" && r.stable {
		entry.state.Status = "idle"
	}

	status := entry.state.Status
	changed := !entry.lastPub.sent ||
		entry.lastPub.pressure != r.pressure ||
		entry.lastPub.stable != r.stable ||
		entry.lastPub.status != status
	if changed {
		entry.lastPub = lastPublished{sent: true, pressure: r.pressure, stable: r.stable, status: status}
	}
	return status, changed
}
