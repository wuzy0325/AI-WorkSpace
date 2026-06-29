package config

import (
	"encoding/json"
	"fmt"

	"wind-daq/services/api-go/internal/core/calibration"
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
	TaskID                 string                              `json:"taskId"`
	DeviceID               string                              `json:"deviceId"`
	Type                   string                              `json:"type"`
	Channels               []int                               `json:"channels"`
	PressurePoints         []float64                           `json:"pressurePoints"`
	AverageSamples         int                                 `json:"averageSamples"`
	ProbeChannels          []ProbeChannelDTO                   `json:"probeChannels,omitempty"`
	Points                 []calibration.CalPoint              `json:"points,omitempty"`
	SamplesPerPoint        int                                 `json:"samplesPerPoint,omitempty"`
	DwellTimeMs            int                                 `json:"dwellTimeMs,omitempty"`
	StopOnError            bool                                `json:"stopOnError,omitempty"`
	Name                   string                              `json:"name"`
	SavePath               string                              `json:"savePath,omitempty"`
	MotionAxes             []calibration.MotionAxisConfig      `json:"motionAxes,omitempty"`
	SphereTankGate         *calibration.SphereTankGateConfig   `json:"sphereTankGate,omitempty"`
	AcquisitionSampling    *calibration.AcquisitionSamplingConfig `json:"acquisitionSampling,omitempty"`
	TotalTemperatureConfig *calibration.TotalTemperatureConfig `json:"totalTemperatureConfig,omitempty"`
}

// ProbeChannelDTO 是探针通道在框架边界的传输对象，同时支持扁平与嵌套两种 JSON shape。
type ProbeChannelDTO struct {
	Role         string                   `json:"role"`
	Name         string                   `json:"name"`
	DeviceID     string                   `json:"deviceId"`
	ChannelIndex int                      `json:"channelIndex"`
	Enabled      bool                     `json:"enabled"`
	Channel      *calibration.ChannelRef `json:"channel,omitempty"`
}

// ToCore 将 DTO 转换为 core 层的 calibration.Config。
// 嵌套 channel 非空时覆盖扁平字段，保证前后端两种 shape 都能正确映射。
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
// 迁移到 adapters/config 层，使 core 仅定义数据结构，符合六边形架构的零容忍约束。
// 调用方（api/server.go、apps/desktop-wails/backend/app.go）在框架边界调用本函数，
// 而非直接 json.Unmarshal 进 calibration.Config。
func DecodeCalibrationConfig(data []byte) (calibration.Config, error) {
	var dto CalibrationConfigDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return calibration.Config{}, fmt.Errorf("decode calibration config: %w", err)
	}
	return dto.ToCore(), nil
}
