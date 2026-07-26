// Package usecase assembly.go 提供 v2 校零组件 + DataSink 的统一装配入口。
//
// 设计动机（Critical BUG 修复）：
// 三处装配根（bootstrap / appcontext / apiserver）原本各自重复实现校零装配，
// 导致 appcontext（Wails 桌面生产路径）与 apiserver（独立 API 服务器）遗漏了
// calApplier 与 channels 闭包，NewDataSink 收到 (nil, nil) 后校零热路径整段跳过，
// 桌面用户即使点击校零按钮也不会生效。此处统一抽出装配函数，确保任何装配根
// 都无法绕过正确顺序。
package usecase

import (
	"time"

	"wind-daq/services/api-go/internal/core/device"
)

// AssembleDataSinkWithCalibration 装配 v2 校零组件并创建 dataSink。
//
// 装配顺序（避免前向引用未初始化变量，避免 nil 闭包）：
//  1. 创建 unitConverter / applier / sampler
//  2. manager.SetCalibrationComponents 注入 applier/sampler
//     （内部会同步已加载 profile 的存量 CalibrationOffset 到 applier 快照，
//     避免冷启动后已校零设备在下次 Calibrate 前偏移不生效）
//  3. 用 manager.GetChannelsForCalibration 构造 channels 闭包
//     （该方法已实现 RLock + copy，返回深拷贝，热路径并发安全）
//  4. 创建 dataSink 并通过 manager.UpdateDataSink 注入
//
// 调用时机：必须在 manager 创建之后、设备启动采集之前调用。
// 返回 (applier, sampler) 供 caller 暴露 API 或进一步使用。
func AssembleDataSinkWithCalibration(
	hub *AcquisitionHub,
	recorder *StorageRecorder,
	manager *DeviceManager,
	sampleDuration time.Duration,
) (*CalibrationApplier, *CalibrationSampler) {
	if sampleDuration <= 0 {
		sampleDuration = time.Duration(device.CalibrationDurationSec) * time.Second
	}
	unitConverter := device.NewUnitConverter()
	calibrationStream := NewCalibrationStream()
	calApplier := NewCalibrationApplier(unitConverter)
	calSampler := NewCalibrationSampler(calibrationStream, unitConverter, sampleDuration)

	manager.SetCalibrationComponents(calApplier, calSampler)

	// channels 闭包每帧被 dataSink 调用，依赖 manager 读取最新 profile。
	// 不能在 manager 创建前捕获其引用（前向引用 nil panic 风险），
	// 因此本函数必须在 manager 已构造后调用。
	dataSink := NewDataSink(hub, recorder, calibrationStream, calApplier, func(deviceID string) []device.ChannelConfig {
		return manager.GetChannelsForCalibration(deviceID)
	})
	manager.UpdateDataSink(dataSink)
	return calApplier, calSampler
}
