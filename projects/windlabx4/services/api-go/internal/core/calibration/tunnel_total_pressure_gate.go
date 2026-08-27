package calibration

import (
	"fmt"
	"math"
)

// fiveHoleTotalPressureRole 五孔探针风洞总压通道角色。
// 风洞总压范围判定复用该已配置通道读取当前总压，不新增独立通道选择。
const fiveHoleTotalPressureRole = "fiveHole.pTotal"

// NormalizeTunnelTotalPressureGate 标准化风洞总压范围判定配置。
// 未配置或未启用时返回 nil（调用方跳过判定）。
func NormalizeTunnelTotalPressureGate(config Config) *TunnelTotalPressureGateConfig {
	if config.TunnelTotalPressureGate == nil || !config.TunnelTotalPressureGate.Enabled {
		return nil
	}
	return normalizeTunnelTotalPressureGate(config.TunnelTotalPressureGate)
}

// normalizeTunnelTotalPressureGate 内部标准化函数：规整超时与范围字段。
func normalizeTunnelTotalPressureGate(gate *TunnelTotalPressureGateConfig) *TunnelTotalPressureGateConfig {
	timeoutSec := gate.TimeoutSec
	if timeoutSec < 0 {
		timeoutSec = 0
	}
	// 范围上下限不做强制交换：非法区间（min > max）由 Validate 显式报错，
	// 避免静默纠正掩盖用户配置错误。
	return &TunnelTotalPressureGateConfig{
		Enabled:          gate.Enabled,
		MinTotalPressure: gate.MinTotalPressure,
		MaxTotalPressure: gate.MaxTotalPressure,
		TimeoutSec:       timeoutSec,
	}
}

// findFiveHoleTotalPressureChannel 在探针通道中查找已启用的 fiveHole.pTotal 通道。
// 未找到或未启用时返回 ok=false。
func findFiveHoleTotalPressureChannel(config Config) (ChannelRef, bool) {
	for _, ch := range config.ProbeChannels {
		if ch.Role == fiveHoleTotalPressureRole && ch.Enabled {
			return ChannelRef{DeviceID: ch.DeviceID, ChannelIndex: ch.ChannelIndex}, true
		}
	}
	return ChannelRef{}, false
}

// IsTotalPressureInRange 判断风洞总压值是否在配置范围内（闭区间 [min, max]）。
// gate 为 nil 或未启用时恒返回 true（等价于跳过判定）。
func IsTotalPressureInRange(gate *TunnelTotalPressureGateConfig, value float64) bool {
	if gate == nil || !gate.Enabled {
		return true
	}
	return value >= gate.MinTotalPressure && value <= gate.MaxTotalPressure
}

// ValidateTunnelTotalPressureGate 验证风洞总压范围判定配置。
// 未启用时返回 nil；启用时校验：
//   - 范围边界为有限数（拒绝 NaN/Inf——JSON 中的 null 解码到非指针 float64 会落为 0）
//   - 范围上下限合法（max >= min），且上下限不同时为 0（null/null 解码头会落到 [0,0]）
//   - fiveHole.pTotal 通道已配置并启用，且 deviceId/通道索引有效
func ValidateTunnelTotalPressureGate(config Config) error {
	if config.TunnelTotalPressureGate == nil || !config.TunnelTotalPressureGate.Enabled {
		return nil
	}
	gate := config.TunnelTotalPressureGate
	if math.IsNaN(gate.MinTotalPressure) || math.IsInf(gate.MinTotalPressure, 0) ||
		math.IsNaN(gate.MaxTotalPressure) || math.IsInf(gate.MaxTotalPressure, 0) {
		return fmt.Errorf("风洞总压范围判定已启用，但范围边界不是有效数字（NaN/Inf）")
	}
	// JSON null 解码到非指针 float64 会落为 0：min/max 均为 0 通常意味着字段缺失/为空，
	// 而非用户有意配置的"恰好 0 Pa"单点范围，显式拒绝以避免 [0,0] 被当作合法范围。
	if gate.MinTotalPressure == 0 && gate.MaxTotalPressure == 0 {
		return fmt.Errorf("风洞总压范围判定已启用，但范围未配置（上下限均为 0，可能来自缺失或空字段）")
	}
	if gate.MaxTotalPressure < gate.MinTotalPressure {
		return fmt.Errorf("风洞总压范围判定已启用，但范围非法（下限 %.2f > 上限 %.2f）",
			gate.MinTotalPressure, gate.MaxTotalPressure)
	}
	ch, ok := findFiveHoleTotalPressureChannel(config)
	if !ok {
		return fmt.Errorf("风洞总压范围判定已启用，但未找到启用的 %s 通道", fiveHoleTotalPressureRole)
	}
	if ch.DeviceID == "" {
		return fmt.Errorf("风洞总压范围判定已启用，但 %s 通道设备ID未配置", fiveHoleTotalPressureRole)
	}
	if ch.ChannelIndex < 0 {
		return fmt.Errorf("风洞总压范围判定已启用，但 %s 通道索引无效", fiveHoleTotalPressureRole)
	}
	return nil
}
