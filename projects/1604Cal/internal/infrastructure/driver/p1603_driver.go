package driver

// P1603Driver DAQ-P-1603 计量设备 thin adapter。
//
// 职责：在 1604Cal 的 device.MeasureDriver（同步按点采集契约）与
// device-sdk 的 sharedhw.DAQP1603（流式 sink 回调）之间做翻译。
// 真实 DLL FFI 协议逻辑全部在 device-sdk 内（单源复用，对齐 WindLabX4），
// 本文件只做三件事：
//   1. 配置翻译：1604Cal domain.Device（含每通道量程）→ device-sdk Profile
//   2. 帧缓存桥接：sink 回调缓存最新工程量帧 → CollectData 同步取值
//   3. 能力桩：阀门恒 calibration（无阀设备）、软件归零（SetTare）
//
// 关键点：
//   - device-sdk 内部已把 U16 码值 → 电流(mA) → 工程量（用每通道
//     RangeMin/RangeMax 做 4-20mA 线性映射），sink 收到的是最终工程量，
//     1604Cal 无需重复换算公式
//   - P1603 必须配置每通道量程才能输出正确的工程量（4mA→engMin、
//     20mA→engMax），未配置时回退 DefaultP1603Channels

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	sharedcore "shared.local/device-sdk/go/daq/core"
	sharedhw "shared.local/device-sdk/go/daq/hardware"

	"cal1604/internal/domain"
)

// p1603SamplingRateHz P1603 用户采样率（每秒输出数据条目数）。
// 对齐 WindLabX4 默认值 100Hz；底层硬件采样率由 device-sdk 固定 1000Hz 多点平均。
const p1603SamplingRateHz = 100

// p1603FirstFrameTimeout CollectData 首次启动采集后等待首帧的超时。
// 100Hz 采样率下首帧应 <100ms，2s 足够宽裕。
const p1603FirstFrameTimeout = 2 * time.Second

// P1603Driver 1604Cal 侧的 P1603 适配器实例。
type P1603Driver struct {
	mu sync.Mutex

	// dev 是 device-sdk 的真实 DLL FFI 驱动（Connect 时创建）。
	dev *sharedhw.DAQP1603

	// config 设备配置（含每通道量程/单位）。Connect 后只读。
	config domain.Device

	// ---- 帧缓存（sink 回调写入，CollectData 读取）----
	// latestFrame 最新一帧工程量：1-based 通道号 → 工程量（Pa 等）。
	// device-sdk 返回的 ChannelIndices 是 0-based，此处统一转 1-based，
	// 与 1604Cal 计量业务层通道语义一致。
	latestFrame map[int]float64
	frameValid  bool
	// firstFrame 首帧到达信号：首次 CollectData 等待采集启动后的首帧。
	firstFrame chan struct{}

	// acquiring 标记采集是否已启动。
	acquiring bool

	// tareOffsets 各通道软件归零偏移（1-based 通道号 → offset）。
	// 语义对齐 WindLabX4 P1603：device-sdk readLoop 输出原始工程量，
	// 归零偏移由展示方扣除。1604Cal 是同步 CollectData，故在此扣除。
	tareOffsets map[int]float64

	// unit 设备级工程单位（软件层，P1603 无硬件单位命令）。
	// 仅用于单位一致性检查展示；各通道实际单位由通道配置 unit 决定。
	unit string
}

// NewP1603Driver 创建 P1603 适配器。
// 入参 config 需包含 Host 与每通道量程配置（可为空，Connect 时回退默认）。
func NewP1603Driver(config domain.Device) *P1603Driver {
	unit := config.Unit
	if unit == "" {
		unit = p1603FirstChannelUnit(config.Channels)
	}
	return &P1603Driver{
		config: config,
		unit:   unit,
	}
}

// p1603FirstChannelUnit 取首个启用通道的单位作为设备级单位兜底。
func p1603FirstChannelUnit(channels []domain.ChannelConfig) string {
	for _, ch := range channels {
		if ch.Enabled && ch.Unit != "" {
			return ch.Unit
		}
	}
	return "Pa"
}

// ---- ConnectionDriver 实现 ----

// Connect 创建 device-sdk 驱动并建立 TCP 连接（DLL FFI 路径）。
// 重复调用安全：已连接时直接返回 nil。
func (d *P1603Driver) Connect(_ context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.dev != nil {
		return nil
	}

	dev := sharedhw.NewDAQP1603(d.buildSharedProfile())
	// 注册帧缓存回调：device-sdk readLoop 每帧回调，缓存最新工程量。
	// 代际保护：闭包捕获本次创建的 dev 指针，onDataFrame 仅在 d.dev 仍指向
	// 该 dev 时写缓存——防止 Disconnect→Connect（或 join 超时）后旧 dev 的
	// readLoop 迟到回调把帧写入新采集的代际，产生缓存串扰。
	dev.SetDataSink(func(payload sharedcore.DataPayload) {
		d.onDataFrame(dev, payload)
	})

	if err := dev.Connect(); err != nil {
		return fmt.Errorf("DAQ-P-1603 connect: %w", err)
	}
	d.dev = dev
	// 加载持久化的校零偏移（随设备配置落盘），使重连后继续扣除，避免零漂。
	d.loadTareOffsets(dev)
	return nil
}

// loadTareOffsets 从设备配置加载各通道持久化的校零偏移，写入本地 tareOffsets
// 供 CollectData 扣除，并同步到 device-sdk profile（SetTare，语义保持一致）。
//
// 调用方必须已持有 d.mu（Connect 在持锁状态下调用本方法）。本方法不再自行加锁，
// 否则会与 Connect 已持有的锁形成不可重入的自死锁（Go sync.Mutex 不可重入）。
func (d *P1603Driver) loadTareOffsets(dev *sharedhw.DAQP1603) {
	offsets := make(map[int]float64)
	for _, ch := range d.effectiveChannels() {
		if ch.TareOffset == 0 {
			continue
		}
		offsets[ch.Index] = ch.TareOffset
		if err := dev.SetTare(ch.Index-1, ch.TareOffset); err != nil {
			log.Printf("[P1603] channel %d tare offset apply failed: %v", ch.Index, err)
		}
	}
	// 始终以持久化配置为准（即使全为 0 也重置），避免重连后残留上次连接的
	// 内存偏移导致错误扣除。调用方已持有 d.mu，此处直接写入。
	d.tareOffsets = offsets
}

// Disconnect 停止采集并释放设备连接。
// 顺序：StopAcquisition（join readLoop）→ device-sdk Disconnect（StopTask+ReleaseTask+DevRelease）。
func (d *P1603Driver) Disconnect(_ context.Context) error {
	d.mu.Lock()
	dev := d.dev
	d.dev = nil
	d.acquiring = false
	d.latestFrame = nil
	d.frameValid = false
	d.mu.Unlock()

	if dev == nil {
		return nil
	}
	_ = dev.StopAcquisition()
	return dev.Disconnect()
}

// ---- 帧缓存 ----

// onDataFrame 处理 device-sdk sink 回调：缓存最新一帧工程量并通知首帧等待方。
//
// ownerDev 是回调注册那次创建的 dev。仅当 d.dev 仍指向 ownerDev 时才更新
// 帧缓存——旧 dev（Disconnect 后 join 超时、或已被新 Connect 替换）的迟到
// 回调不写新代际缓存，避免 Disconnect→Connect 期间帧串扰。
func (d *P1603Driver) onDataFrame(ownerDev *sharedhw.DAQP1603, payload sharedcore.DataPayload) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.dev != ownerDev {
		// 旧代际的 readLoop 迟到回调：忽略，不污染新采集的缓存
		return
	}
	if d.latestFrame == nil {
		d.latestFrame = make(map[int]float64, len(payload.Channels))
	}
	for ch := range d.latestFrame {
		delete(d.latestFrame, ch)
	}
	for i, idx := range payload.ChannelIndices {
		if i >= len(payload.Channels) {
			break
		}
		// ChannelIndices 为 0-based，转 1-based 与业务层一致
		d.latestFrame[idx+1] = payload.Channels[i]
	}
	d.frameValid = true
	if d.firstFrame != nil {
		close(d.firstFrame)
		d.firstFrame = nil
	}
}

// ---- MeasureDriver 接口实现 ----

// ReadValveStatus 阀门状态桩：P1603 无阀门，恒返回 calibration。
// 语义：无阀设备恒处于可用态，计量启动门禁自然通过。
func (d *P1603Driver) ReadValveStatus(_ context.Context) (string, error) {
	return string(domain.ValveStateCalibration), nil
}

// SetValveStatus 阀门切换桩：P1603 无阀门，幂等空操作。
func (d *P1603Driver) SetValveStatus(_ context.Context, _ string) error {
	return nil
}

// ReadUnit 返回设备级工程单位（软件层设置，默认取首通道单位或 Pa）。
func (d *P1603Driver) ReadUnit(_ context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.unit, nil
}

// SetUnit 设置设备级工程单位（软件层，P1603 无硬件单位命令）。
// 注意：各通道实际工程量由通道量程配置决定，SetUnit 仅更新设备级标签
// 供单位一致性检查使用，不改变已输出的工程量数值。
func (d *P1603Driver) SetUnit(_ context.Context, unit string) error {
	if unit == "" {
		return fmt.Errorf("DAQ-P-1603 set unit: empty unit")
	}
	d.mu.Lock()
	d.unit = unit
	d.mu.Unlock()
	return nil
}

// CollectData 从最新帧缓存取指定通道的工程量。
// channels 为 1-based 通道号（1-16）。未启动采集时惰性启动并等待首帧。
func (d *P1603Driver) CollectData(ctx context.Context, channels []int) ([]float64, error) {
	if len(channels) == 0 {
		return nil, fmt.Errorf("DAQ-P-1603 collect data: no channels requested")
	}
	if err := d.ensureAcquisition(ctx); err != nil {
		return nil, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.frameValid || len(d.latestFrame) == 0 {
		return nil, fmt.Errorf("DAQ-P-1603 collect data: no valid frame cached")
	}

	values := make([]float64, 0, len(channels))
	for _, ch := range channels {
		v, ok := d.latestFrame[ch]
		if !ok {
			return nil, fmt.Errorf("DAQ-P-1603 collect data: channel %d not in frame (disabled?)", ch)
		}
		// 扣除软件归零偏移（对齐 WindLabX4：readLoop 输出原始值，展示方减 TareOffset）
		if offset, has := d.tareOffsets[ch]; has {
			v -= offset
		}
		values = append(values, v)
	}
	return values, nil
}

// ensureAcquisition 确保连续采集已启动且首帧已到达。
func (d *P1603Driver) ensureAcquisition(ctx context.Context) error {
	d.mu.Lock()
	if !d.acquiring {
		if d.dev == nil {
			d.mu.Unlock()
			return fmt.Errorf("DAQ-P-1603: not connected")
		}
		dev := d.dev
		d.firstFrame = make(chan struct{})
		d.latestFrame = nil
		d.frameValid = false
		if err := dev.StartAcquisition(); err != nil {
			d.mu.Unlock()
			return fmt.Errorf("DAQ-P-1603 start acquisition: %w", err)
		}
		d.acquiring = true
	}
	firstFrame := d.firstFrame
	d.mu.Unlock()

	if firstFrame == nil {
		return nil // 首帧已到或采集早已运行
	}
	timer := time.NewTimer(p1603FirstFrameTimeout)
	defer timer.Stop()
	select {
	case <-firstFrame:
		return nil
	case <-timer.C:
		return fmt.Errorf("DAQ-P-1603 first frame not received within %v", p1603FirstFrameTimeout)
	case <-ctx.Done():
		return fmt.Errorf("DAQ-P-1603 wait first frame: %w", ctx.Err())
	}
}

// CalibrateZero 软件归零：读当前各通道工程量后记录 TareOffset（当前值）。
//
// 对齐 WindLabX4 P1603 归零语义：SetTare 写入 device-sdk profile 的
// TareOffset（持久化），同时本地记录 tareOffsets 供 CollectData 扣除。
// 返回各通道调零前的原始读数。
func (d *P1603Driver) CalibrateZero(ctx context.Context, channels []int) ([]float64, error) {
	if len(channels) == 0 {
		return nil, fmt.Errorf("DAQ-P-1603 calibrate zero: no channels requested")
	}
	if err := d.ensureAcquisition(ctx); err != nil {
		return nil, fmt.Errorf("DAQ-P-1603 calibrate zero: %w", err)
	}

	d.mu.Lock()
	frame := d.latestFrame
	valid := d.frameValid
	dev := d.dev
	if d.tareOffsets == nil {
		d.tareOffsets = make(map[int]float64)
	}
	// 关键：整个校零循环（含 tareOffsets 写）都在锁内完成，与 CollectData 的
	// 加锁读一致。若把 map 写放到锁外，实时采集轮询（CollectData 加锁读同一 map）
	// 会与这里的写形成数据竞争——Go map 并发读写可能 panic，或读到撕裂/空 map
	// 导致归零偏移未被应用，采集值退回原始（未校零）读数。
	if !valid || len(frame) == 0 {
		d.mu.Unlock()
		return nil, fmt.Errorf("DAQ-P-1603 calibrate zero: no valid frame cached")
	}

	results := make([]float64, 0, len(channels))
	for _, ch := range channels {
		current, ok := frame[ch]
		if !ok {
			d.mu.Unlock()
			return nil, fmt.Errorf("DAQ-P-1603 calibrate zero: channel %d not in frame", ch)
		}
		// device-sdk SetTare 使用 0-based 通道索引（持久化到 profile）。
		// SetTare 为纯内存操作（只写 profile.Channels），不触碰 DLL，可安全持锁。
		if err := dev.SetTare(ch-1, current); err != nil {
			d.mu.Unlock()
			return nil, fmt.Errorf("DAQ-P-1603 calibrate zero channel %d: %w", ch, err)
		}
		// 本地记录归零偏移，CollectData 时扣除（展示值 = 原始值 - offset）
		d.tareOffsets[ch] = current
		results = append(results, current)
	}
	d.mu.Unlock()
	return results, nil
}

// CalibrateFullScale 满量程校准：P1603 无硬件校准命令，返回明确错误。
func (d *P1603Driver) CalibrateFullScale(_ context.Context, _ []int, _ float64) ([]float64, error) {
	return nil, fmt.Errorf("DAQ-P-1603: full scale calibration not supported by device protocol")
}

// ReadDeviceInfo 返回设备静态描述信息。
func (d *P1603Driver) ReadDeviceInfo(_ context.Context) (map[string]string, error) {
	d.mu.Lock()
	unit := d.unit
	d.mu.Unlock()
	return map[string]string{
		"model":    "DAQ-P-1603",
		"protocol": "DLL FFI (WTNDAQ16H_64.dll)",
		"unit":     unit,
		"channels": fmt.Sprintf("%d AI (4-20mA current loop)", len(d.effectiveChannels())),
	}, nil
}

// Reset 设备复位：P1603 无复位命令（FFI 无对应 Proc），返回明确错误。
func (d *P1603Driver) Reset(_ context.Context) error {
	return fmt.Errorf("DAQ-P-1603: reset not supported")
}

// ---- 配置翻译 ----

// effectiveChannels 返回通道配置：已配置则用配置，否则回退 16 通道默认（±5000 Pa）。
func (d *P1603Driver) effectiveChannels() []domain.ChannelConfig {
	if len(d.config.Channels) > 0 {
		return d.config.Channels
	}
	return domain.DefaultP1603Channels()
}

// buildSharedProfile 把 1604Cal 设备配置翻译为 device-sdk 的 Profile。
// 关键：每通道 RangeMin/RangeMax/Unit 透传给 device-sdk 做 4-20mA→工程量映射；
// Index 从 1-based 转 0-based（device-sdk 语义）。
func (d *P1603Driver) buildSharedProfile() sharedcore.Profile {
	channels := d.effectiveChannels()
	sharedChannels := make([]sharedcore.ChannelConfig, 0, len(channels))
	for _, ch := range channels {
		sharedChannels = append(sharedChannels, sharedcore.ChannelConfig{
			Index: ch.Index - 1, // 1-based → 0-based
			Name:  ch.Name,
			// 计量工作流固定读取 1..16；设备配置的 Enabled 只表示 UI/报警选择，
			// 不能在 SDK Profile 中过滤，否则禁用通道会从帧中消失并导致整次采集失败。
			Enabled:    true,
			Unit:       ch.Unit,
			Precision:  ch.Precision,
			RangeMin:   ch.RangeMin,
			RangeMax:   ch.RangeMax,
			SensorType: sharedcore.SensorPressure,
			TareOffset: ch.TareOffset,
		})
	}
	if len(sharedChannels) == 0 {
		// 无有效配置时兜底：至少保留 16 通道（防御，避免 device-sdk nChans=0）
		sharedChannels = make([]sharedcore.ChannelConfig, 16)
		for i := range sharedChannels {
			sharedChannels[i] = sharedcore.ChannelConfig{
				Index: i, Name: fmt.Sprintf("CH%d", i+1), Enabled: true,
				Unit: "Pa", Precision: 3, RangeMin: -5000, RangeMax: 5000,
				SensorType: sharedcore.SensorPressure,
			}
		}
	}
	return sharedcore.Profile{
		ID:           d.config.ID,
		Name:         d.config.Name,
		Type:         sharedcore.DeviceDAQP1603,
		Address:      d.config.Host,
		SamplingRate: p1603SamplingRateHz,
		Channels:     sharedChannels,
	}
}
