// Package types 暴露 WindLabX4 跨模块共用的传输 DTO。
//
// 本文件托管校准配置 DTO（Task 05：从 internal/adapters/config 迁移）。
// 迁移原因：HTTP 和 Wails 都需要同一份 DTO，但 internal/adapters/config 受 Go internal
// 规则约束无法被 desktop-wails/backend 直接 import。原 pkg/types 通过类型别名 facade
// 暴露，但别名只是"贴牌"，类型身份仍属于 adapters/config——这导致：
//  1. Wails binding 生成的 TS 类路径仍指向 internal/adapters/config（不可访问）
//  2. DTO 字段补齐（如 MotionSafety）需要在 adapters/config 改，违反"DTO 属于 transport boundary"的定位
//
// 迁移后 DTO 成为 pkg/types 的一等类型，HTTP/Wails 共用同一身份，MotionSafety 字段
// 与 calibration.Config 完整对齐，由 backend Start 做权威校验。
package types

import (
	"encoding/json"
	"fmt"

	"windlabx4/services/api-go/internal/core/calibration"
	"windlabx4/services/api-go/internal/core/traversal"
)

// CalibrationConfigDTO 是校准配置在框架边界（HTTP / Wails）的传输对象。
//
// 为什么需要 DTO：
// core 层禁止做字节级 I/O（见 CLAUDE.md 零容忍约束），因此 calibration.Config
// 不再自带 UnmarshalJSON。但前端 ProbeChannelConfig 使用嵌套结构
// { channel: { deviceId, channelIndex } }，而后端/旧调用方使用扁平结构
// { deviceId, channelIndex }。DTO 用普通 struct tag 同时声明两种字段，
// 让 encoding/json 的默认解码器就能同时接收两种 shape，无需自定义 UnmarshalJSON。
//
// ProbeChannelDTO 同时携带：
//   - 扁平字段 DeviceID/ChannelIndex（兼容旧后端 shape）
//   - 嵌套字段 Channel *calibration.ChannelRef（兼容前端 shape）
//
// 在 ToCore 中，若 Channel 非空则用嵌套值覆盖扁平值，与原 ProbeChannel.UnmarshalJSON 语义一致。
type CalibrationConfigDTO struct {
	TaskID          string                         `json:"taskId"`
	DeviceID        string                         `json:"deviceId"`
	Type            string                         `json:"type"`
	Channels        []int                          `json:"channels"`
	PressurePoints  []float64                      `json:"pressurePoints"`
	AverageSamples  int                            `json:"averageSamples"`
	ProbeChannels   []ProbeChannelDTO              `json:"probeChannels,omitempty"`
	Points          []calibration.CalPoint         `json:"points,omitempty"`
	SamplesPerPoint int                            `json:"samplesPerPoint,omitempty"`
	DwellTimeMs     int                            `json:"dwellTimeMs,omitempty"`
	StopOnError     bool                           `json:"stopOnError,omitempty"`
	Name            string                         `json:"name"`
	SavePath        string                         `json:"savePath,omitempty"`
	MotionAxes      []calibration.MotionAxisConfig `json:"motionAxes,omitempty"`
	// CoordinateMode 运动坐标模式："absolute"（绝对坐标，默认）| "relative"（相对坐标）。
	// 与 calibration.Config.CoordinateMode 对齐，空值视为 absolute。
	CoordinateMode calibration.CoordinateMode `json:"coordinateMode,omitempty"`
	// MotionSafety 运动安全配置：到位容差、严重偏离阈值、跨样本看门狗等。
	// 修复（Task 05）：原 adapters/config 层 DTO 遗漏此字段，导致前端发送的 motionSafety
	// 在反序列化时被静默丢弃，后端只能拿到 nil 并使用 DefaultMotionSafety，绕过了用户配置。
	// 此处补齐字段，与 calibration.Config.MotionSafety 完整对齐，由 backend Start 通过
	// validateCalibrationMotionSafetyConfig 做权威校验（拒绝非法值而非静默忽略）。
	MotionSafety           *traversal.MotionSafetyConfig          `json:"motionSafety,omitempty"`
	SphereTankGate         *calibration.SphereTankGateConfig      `json:"sphereTankGate,omitempty"`
	AcquisitionSampling    *calibration.AcquisitionSamplingConfig `json:"acquisitionSampling,omitempty"`
	TotalTemperatureConfig *calibration.TotalTemperatureConfig    `json:"totalTemperatureConfig,omitempty"`
}

// ProbeChannelDTO 是探针通道在框架边界的传输对象，同时支持扁平与嵌套两种 JSON shape。
type ProbeChannelDTO struct {
	Role         string                  `json:"role"`
	Name         string                  `json:"name"`
	DeviceID     string                  `json:"deviceId"`
	ChannelIndex int                     `json:"channelIndex"`
	Enabled      bool                    `json:"enabled"`
	Channel      *calibration.ChannelRef `json:"channel,omitempty"`
}

// ToCore 将 DTO 转换为 core 层的 calibration.Config。
// 嵌套 channel 非空时覆盖扁平字段，保证前后端两种 shape 都能正确映射。
// MotionSafety 指针原样传递（共享底层值），nil 语义保留——下游使用 DefaultMotionSafety。
func (d CalibrationConfigDTO) ToCore() calibration.Config {
	cfg := calibration.Config{
		TaskID:                 d.TaskID,
		DeviceID:               d.DeviceID,
		Type:                   d.Type,
		Channels:               d.Channels,
		PressurePoints:         d.PressurePoints,
		AverageSamples:         d.AverageSamples,
		Points:                 d.Points,
		SamplesPerPoint:        d.SamplesPerPoint,
		DwellTimeMs:            d.DwellTimeMs,
		StopOnError:            d.StopOnError,
		Name:                   d.Name,
		SavePath:               d.SavePath,
		MotionAxes:             d.MotionAxes,
		CoordinateMode:         d.CoordinateMode,
		MotionSafety:           d.MotionSafety,
		SphereTankGate:         d.SphereTankGate,
		AcquisitionSampling:    d.AcquisitionSampling,
		TotalTemperatureConfig: d.TotalTemperatureConfig,
	}
	if len(d.ProbeChannels) > 0 {
		cfg.ProbeChannels = make([]calibration.ProbeChannel, len(d.ProbeChannels))
		for i, p := range d.ProbeChannels {
			cfg.ProbeChannels[i] = p.ToCore()
		}
	}
	return cfg
}

// ToCore 将单个探针通道 DTO 转换为 core 层的 calibration.ProbeChannel。
// 嵌套 channel 非空时覆盖扁平字段。
func (p ProbeChannelDTO) ToCore() calibration.ProbeChannel {
	pc := calibration.ProbeChannel{
		Role:         p.Role,
		Name:         p.Name,
		DeviceID:     p.DeviceID,
		ChannelIndex: p.ChannelIndex,
		Enabled:      p.Enabled,
	}
	if p.Channel != nil {
		pc.DeviceID = p.Channel.DeviceID
		pc.ChannelIndex = p.Channel.ChannelIndex
	}
	return pc
}

// DecodeCalibrationConfig 将 JSON 字节流解码为 core 层的 calibration.Config。
//
// 此函数是 core 层 ProbeChannel.UnmarshalJSON 的替代实现：把字节级 I/O 从 core
// 迁移到 transport boundary（pkg/types），使 core 仅定义数据结构，符合六边形架构
// 的零容忍约束。调用方（api/server.go、apps/desktop-wails/backend/app.go）在框架
// 边界调用本函数，而非直接 json.Unmarshal 进 calibration.Config。
//
// Task 05 迁移：原位于 internal/adapters/config，现移至 pkg/types 以便 HTTP 和 Wails
// 共用同一身份的 DTO 类型，同时修复 MotionSafety 字段遗漏。
func DecodeCalibrationConfig(data []byte) (calibration.Config, error) {
	var dto CalibrationConfigDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return calibration.Config{}, fmt.Errorf("decode calibration config: %w", err)
	}
	return dto.ToCore(), nil
}
