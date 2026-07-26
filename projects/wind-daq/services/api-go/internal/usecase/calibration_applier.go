package usecase

import (
	"context"
	"fmt"
	"shared.local/device-sdk/go/pkg/slog"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/device"
)

// P2-18：chanMap 池化复用，避免每帧 Apply 都分配 map[int]*ChannelConfig。
// 热路径（500-1000Hz × 多设备）下每秒减少数千次 map 分配，降低 GC 压力。
//
// 安全性：map 中存放的 *ChannelConfig 指针仅在本次 Apply 调用内有效（指向 channels 切片），
// 归还前通过 clearChanMap 清空所有 entry，避免悬垂指针跨调用泄漏。
// sync.Pool 本身线程安全，多设备 read loop 并发 Get/Put 无需额外加锁。
var chanMapPool = sync.Pool{
	New: func() interface{} {
		// 初始容量 16 覆盖常见 16 通道设备，避免运行时 rehash
		return make(map[int]*device.ChannelConfig, 16)
	},
}

// clearChanMap 清空 map 所有 entry（保留底层数组，避免重新分配）。
// Go 1.21+ 可直接用 clear(m)，此处手动 delete 兼容旧版本。
func clearChanMap(m map[int]*device.ChannelConfig) {
	for k := range m {
		delete(m, k)
	}
}

// CalibrationApplier 在 raw DataPayload 投递前逐通道减去校零偏移。
//
// 设计原则：
//   - 校零值内部全走基单位（Pa/℃），落盘时已由 CalibrationSampler 统一转基单位
//   - Apply 前需将基单位偏移换算为当前通道单位（如 Pa→kPa）
//   - CalibrationEnabled=false 的通道跳过偏移
//
// 线程安全：offsets 由 LockOffsets/Unlock 保护，写入路径（DeviceManager.Calibrate）持写锁，
// 读取路径（Apply）持读锁。在 500Hz×10设备 场景下读锁开销极低。
type CalibrationApplier struct {
	mu      sync.RWMutex
	offsets map[string]device.CalibrationOffsets // deviceID → {channelIndex → offsetBaseUnit}
	uc      *device.UnitConverter
}

// NewCalibrationApplier 创建校零应用器。
func NewCalibrationApplier(uc *device.UnitConverter) *CalibrationApplier {
	return &CalibrationApplier{
		offsets: make(map[string]device.CalibrationOffsets),
		uc:      uc,
	}
}

// UpdateOffsets 更新指定设备的校零偏移快照（从 profile load 或 Calibrate 后调用）。
// 传入的值应为基单位（Pa/℃）。
func (ca *CalibrationApplier) UpdateOffsets(deviceID string, offsets device.CalibrationOffsets) {
	ca.mu.Lock()
	ca.offsets[deviceID] = offsets
	ca.mu.Unlock()
}

// RemoveDevice 移除设备的校零偏移。
func (ca *CalibrationApplier) RemoveDevice(deviceID string) {
	ca.mu.Lock()
	delete(ca.offsets, deviceID)
	ca.mu.Unlock()
}

// Apply 对 payload 的每通道值减去校零偏移。
// 传入的 profileChannels 用于获取单位（将基单位偏移换算为当前单位后再减）。
// 注意：校零偏移已在落库时统一转为基单位，此处按当前通道单位换算。
//
// 性能：channels 每次 payload 都不同（来自 GetChannelsForCalibration 深拷贝），
// 无法跨 payload 缓存。但可在单次 Apply 内构建 map[int]*ChannelConfig，
// 将 O(N²) 的逐通道线性查找降为 O(N) 构建 + O(1) 查找，
// 16 通道 × 1000Hz 场景下每秒减少 ~24 万次比较。
//
// P2-18：chanMap 通过 sync.Pool 跨调用复用，避免每帧分配 map 结构。
func (ca *CalibrationApplier) Apply(payload *device.DataPayload, channels []device.ChannelConfig) {
	// 防御：上游 adapter 可能产生长度不一致的 Channels/ChannelIndices，避免 for 循环越界 panic
	if len(payload.Channels) != len(payload.ChannelIndices) {
		slog.Error("calibration applier: channel/indices length mismatch",
			"device", payload.DeviceID,
			"channels", len(payload.Channels),
			"indices", len(payload.ChannelIndices))
		return
	}

	ca.mu.RLock()
	offsets, ok := ca.offsets[payload.DeviceID]
	ca.mu.RUnlock()
	if !ok || len(offsets) == 0 {
		return
	}

	// P2-18：从池中借用 chanMap，用完归还。
	// 借用时可能有残留 entry（来自上次调用），先清空再填充。
	chanMap := chanMapPool.Get().(map[int]*device.ChannelConfig)
	clearChanMap(chanMap)
	defer chanMapPool.Put(chanMap)

	// 构建通道索引 → 配置指针的 map，供后续 O(1) 查找。
	// 注意：必须取 &channels[i] 的地址而非值拷贝，避免大结构体复制。
	// channels 为本次 Apply 独有的深拷贝，不存在并发修改，无需加锁。
	for i := range channels {
		chanMap[channels[i].Index] = &channels[i]
	}

	for i := range payload.Channels {
		idx := payload.ChannelIndices[i]
		offsetBase, exists := offsets[idx]
		if !exists || offsetBase == 0 {
			continue
		}
		// O(1) 查找替代原先的 O(N) 线性扫描
		ch, found := chanMap[idx]
		if !found {
			continue
		}
		if !ch.CalibrationEnabled || ch.Unit == "" {
			continue
		}
		if !ca.uc.SupportsZeroCalibration(ch.Unit) {
			continue
		}
		// 基单位 → 当前单位（e.g. 1000 Pa → 1 kPa）
		offsetCurrent, err := ca.uc.FromBaseUnit(offsetBase, ch.Unit)
		if err != nil {
			slog.Error("calibration applier unit conversion", "device", payload.DeviceID, "channel", idx, "unit", ch.Unit, "err", err)
			continue
		}
		payload.Channels[i] -= offsetCurrent
	}
}

// CalibrationSampler 订阅 AcquisitionHub 5 秒，计算每通道均值。
//
// 使用方式：
//
//	sampler := NewCalibrationSampler(hub, unitConverter, 5*time.Second)
//	results, err := sampler.Sample(ctx, deviceID, channels)
//	if err != nil { /* 超时或被取消 */ }
type CalibrationSampler struct {
	stream   *CalibrationStream
	uc       *device.UnitConverter
	duration time.Duration
	bufSize  int
}

// NewCalibrationSampler 创建校零采样器。
// duration 默认 CalibrationDurationSec 秒（见 core/device/types.go）。
func NewCalibrationSampler(stream *CalibrationStream, uc *device.UnitConverter, duration time.Duration) *CalibrationSampler {
	// 1024 ≈ 2 秒弹性缓冲（500Hz × 2s），非全量容纳 5 秒数据。
	// 设计意图：正常消费速率追得上生产速率，缓冲区仅应对 GC/CPU 争抢等短暂落后。
	// 若消费者持续落后导致 buffer 满，生产者阻塞会被 Subscription 的 goroutine 带崩，
	// 届时 5 秒 deadline 触发 → unsub → Sample 返回，不会无限期卡死。
	bufSize := 1024
	if duration <= 0 {
		duration = time.Duration(device.CalibrationDurationSec) * time.Second
	}
	return &CalibrationSampler{
		stream:   stream,
		uc:       uc,
		duration: duration,
		bufSize:  bufSize,
	}
}

// Sample 订阅指定设备 5 秒实时数据流，按通道索引分桶取算术平均。
// ctx 用于支持取消（5 秒内用户可取消校零）。
// channels 用于获取每通道的单位（均值需转为基单位存储）。
// 返回 []CalibrationResult，已转为基单位值。
func (cs *CalibrationSampler) Sample(ctx context.Context, deviceID string, channels []device.ChannelConfig, targetChannel *int, onProgress func(int)) ([]device.CalibrationResult, error) {
	sub, unsub := cs.stream.Subscribe(deviceID, cs.bufSize)
	defer unsub()

	// channelBins: channelIndex → []float64（按通道分桶，存原始工程量）
	channelBins := make(map[int][]float64)
	// channelUnits: channelIndex → unit（从 channels 配置中获取）
	channelUnits := make(map[int]string)
	totalSamples := 0
	for _, ch := range channels {
		if (targetChannel == nil || ch.Index == *targetChannel) && cs.uc.SupportsZeroCalibration(ch.Unit) {
			channelUnits[ch.Index] = ch.Unit
		}
	}
	if targetChannel != nil {
		if _, ok := channelUnits[*targetChannel]; !ok {
			return nil, fmt.Errorf("channel %d does not support zero calibration", *targetChannel)
		}
	}
	if len(channelUnits) == 0 {
		return nil, fmt.Errorf("device has no pressure channels for zero calibration")
	}

	deadline := time.After(cs.duration)

	// 收集阶段：持续接收 5 秒
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline:
			goto compute
		case payload := <-sub:
			if len(payload.Channels) != len(payload.ChannelIndices) {
				continue
			}
			// 按 ChannelIndices 分桶
			for i := range payload.Channels {
				idx := payload.ChannelIndices[i]
				if _, wanted := channelUnits[idx]; !wanted {
					continue
				}
				channelBins[idx] = append(channelBins[idx], payload.Channels[i])
				totalSamples++
			}
			if onProgress != nil {
				onProgress(totalSamples)
			}
		}
	}

compute:
	// 防御：5 秒 deadline 触发后跳出 select，此时不再监听 ctx.Done()。
	// 若用户在 4.99 秒点击取消，deadline 已 fire 但 ctx 也已 cancel，
	// 不检查会导致后端继续落库偏移而前端标 cancelled —— 状态分裂。
	// 此处显式检查 ctx.Err() 保证取消语义在 compute 阶段也生效。
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// 采样完成时刻的时间戳（非采样开始时刻），用于 UI 展示"上次校零于 xxx"
	now := time.Now().UnixMilli()
	results := make([]device.CalibrationResult, 0, len(channelBins))
	for idx, values := range channelBins {
		if len(values) == 0 {
			continue
		}
		mean := meanFloat64(values)
		// 查通道单位
		unit, ok := channelUnits[idx]
		if !ok {
			continue
		}
		// 工程量 → 基单位
		baseValue, err := cs.uc.ToBaseUnit(mean, unit)
		if err != nil {
			return nil, fmt.Errorf("calibration channel %d: %w", idx, err)
		}
		results = append(results, device.CalibrationResult{
			ChannelIndex: idx,
			Offset:       baseValue,
			Unit:         unit,
			At:           now,
			SampleCount:  len(values),
		})
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no calibration samples received")
	}
	return results, nil
}

// meanFloat64 计算浮点数切片的算术平均。
func meanFloat64(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}
