// data_sink.go 数据汇工厂：把设备数据流的共同入口提取为可复用函数。
//
// 本函数仅依赖 core/device 与本包（usecase），不导入 adapters，
// 所有装配根（appcontext、apiserver、bootstrap）和测试均可直接调用，
// 避免 wiring 包因导入 usecase 而产生的 test 层 import cycle。
package usecase

import "windlabx4/services/api-go/internal/core/device"

// NewDataSink 创建数据汇闭包，承担三项职责：
//  1. 防御性拷贝：生产者（SimulatedDevice 等）可能通过 sync.Pool 复用底层切片，
//     必须在交给 hub 和 recorder 之前拷贝出独立副本，否则 recorder 异步路径
//     会把 payload 引用存入 channel，writer goroutine 读出时切片可能已被覆盖。
//     在此统一拷贝，让 hub 和 recorder 共享同一份独立数据，避免重复拷贝。
//  2. 校零应用：若 calApplier 非 nil 且 device 有校准偏移，先减去偏移
//  3. hub.OnData：同步更新内存索引（分片锁，<1μs）
//  4. recorder.HandlePayload：内部 sink.Write 已异步化（投递到 channel 立即返回）
//
// 整体不阻塞设备 read loop，支撑 1kHz × 10 设备全量保存。
//
// channels 为设备通道配置（用于校零时的单位换算），通常由 caller 从 deviceManager 获取。
func NewDataSink(hub *AcquisitionHub, recorder *StorageRecorder, calibrationStream *CalibrationStream, calApplier *CalibrationApplier, channels func(deviceID string) []device.ChannelConfig) func(device.DataPayload) {
	return func(payload device.DataPayload) {
		payload.EnsureNonNilSlices()
		if len(payload.Channels) > 0 {
			channelsCopy := make([]float64, len(payload.Channels))
			copy(channelsCopy, payload.Channels)
			payload.Channels = channelsCopy
		}
		if len(payload.ChannelIndices) > 0 {
			indicesCopy := make([]int, len(payload.ChannelIndices))
			copy(indicesCopy, payload.ChannelIndices)
			payload.ChannelIndices = indicesCopy
		}
		if calibrationStream != nil {
			calibrationStream.Publish(payload)
		}
		// Apply calibration only after the raw sampling tap.
		if calApplier != nil && channels != nil {
			ch := channels(payload.DeviceID)
			calApplier.Apply(&payload, ch)
		}
		hub.OnData(payload)
		if recorder != nil {
			_ = recorder.HandlePayload(payload)
		}
	}
}
