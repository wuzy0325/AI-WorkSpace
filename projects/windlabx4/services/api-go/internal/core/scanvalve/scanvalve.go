// Package scanvalve 扫描阀适配器：根据扫描速率推荐采集参数、按 ProbeChannelRole
// 映射五孔压力、并对最终压力做物理范围合理性校验。
//
// 对应 Cursor DAQ ScanValveAdapter.ts 的 Go 实现。
package scanvalve

import (
	"errors"
	"fmt"
	"math"

	"windlabx4/services/api-go/internal/core/traversal"
)

// Config 扫描阀采集参数
type Config struct {
	// ScanRateHz 扫描速率（Hz），如 100 表示 100 帧/秒
	ScanRateHz float64
	// ChannelCount 通道数（典型 7：P1..P5,Patm,Tatm）
	ChannelCount int
	// Enabled 是否启用扫描阀模式（false 时其他参数无意义）
	Enabled bool
}

// RecommendedAcquisitionParams 推荐采集参数（与 Cursor DAQ 公式一致）
//   - DwellTimeMs ≥ 2 个扫描周期（让数据稳定）
//   - SamplesPerPoint：≥100Hz 用 5；<100Hz 用 10
//   - BatchTimeoutMs = 2 个扫描周期
type RecommendedAcquisitionParams struct {
	DwellTimeMs     int
	SamplesPerPoint int
	BatchTimeoutMs  int
}

// PressureValidationRange 物理合理性范围
type PressureValidationRange struct {
	Min, Max float64
}

// 默认物理合理性范围（Pa / ℃）
//   - Patm 正常海平面区间 80~110 kPa
//   - Tatm 标准范围 -50~100 ℃
//   - 五孔压力（差压或绝压）：±200 kPa 兜底范围
var (
	defaultPatmRange  = PressureValidationRange{Min: 80000, Max: 110000}
	defaultTatmRange  = PressureValidationRange{Min: -50, Max: 100}
	defaultProbeRange = PressureValidationRange{Min: -200000, Max: 200000}
)

// ScanIntervalMs 单次扫描周期毫秒
func (c Config) ScanIntervalMs() float64 {
	if c.ScanRateHz <= 0 {
		return 0
	}
	return 1000.0 / c.ScanRateHz
}

// RecommendParams 根据 scanRateHz 给出推荐采集参数
func (c Config) RecommendParams() RecommendedAcquisitionParams {
	interval := c.ScanIntervalMs()
	if interval <= 0 {
		// 未配置时给一个保守默认
		return RecommendedAcquisitionParams{
			DwellTimeMs:     200,
			SamplesPerPoint: 5,
			BatchTimeoutMs:  2000,
		}
	}
	dwell := int(math.Ceil(2 * interval))
	if dwell < 1 {
		dwell = 1
	}
	samples := 5
	if c.ScanRateHz < 100 {
		samples = 10
	}
	batchTimeout := int(math.Ceil(2 * interval))
	if batchTimeout < 100 {
		batchTimeout = 100
	}
	return RecommendedAcquisitionParams{
		DwellTimeMs:     dwell,
		SamplesPerPoint: samples,
		BatchTimeoutMs:  batchTimeout,
	}
}

// DetectConfig 默认探测：返回启用 + 100Hz + 7 通道
// 真实环境应从设备元数据读取，这里给出与 Cursor DAQ 默认一致的兜底值
func DetectConfig() Config {
	return Config{ScanRateHz: 100, ChannelCount: 7, Enabled: true}
}

// ExtractPressures 按 channelLabels 把多通道值映射为五孔压力 map
// 参数：
//   - values:        通道索引 → 原始值
//   - channelLabels: 通道索引 → 标签（"P1".."Tatm"）
//
// 返回的 map 至少包含可识别到的标签；缺失项不写入。
func ExtractPressures(values map[int]float64, channelLabels map[int]string) map[string]float64 {
	out := make(map[string]float64, 7)
	for ch, v := range values {
		if label, ok := channelLabels[ch]; ok && label != "" {
			out[label] = v
		}
	}
	return out
}

// ValidatePressures 在物理合理性范围内校验五孔压力
// 自定义范围通过 traversal.DataValidationConfig.PressureRange 提供（覆盖默认值）
// 返回首个不在范围内的字段错误；全部通过返回 nil
func ValidatePressures(p map[string]float64, custom *traversal.DataValidationConfig) error {
	check := func(label string, v float64, defaultR PressureValidationRange) error {
		r := defaultR
		if custom != nil && custom.Enabled {
			if cr, ok := custom.PressureRange[label]; ok && cr != nil {
				r = PressureValidationRange{Min: cr.Min, Max: cr.Max}
			}
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("%s is not finite: %v", label, v)
		}
		if v < r.Min || v > r.Max {
			return fmt.Errorf("%s out of range [%.2f, %.2f]: %.2f", label, r.Min, r.Max, v)
		}
		return nil
	}
	if v, ok := p["Patm"]; ok {
		if err := check("Patm", v, defaultPatmRange); err != nil {
			return err
		}
	}
	if v, ok := p["Tatm"]; ok {
		if err := check("Tatm", v, defaultTatmRange); err != nil {
			return err
		}
	}
	for _, label := range []string{"P1", "P2", "P3", "P4", "P5"} {
		if v, ok := p[label]; ok {
			if err := check(label, v, defaultProbeRange); err != nil {
				return err
			}
		}
	}
	return nil
}

// ErrIncompletePressures 表示无法从输入中获取完整五孔压力组
var ErrIncompletePressures = errors.New("incomplete five-hole pressures")

// CompletePressures 检查 P1..Patm,Tatm 是否齐全；不齐全返回 ErrIncompletePressures
func CompletePressures(p map[string]float64) error {
	required := []string{"P1", "P2", "P3", "P4", "P5", "Patm", "Tatm"}
	for _, k := range required {
		if _, ok := p[k]; !ok {
			return fmt.Errorf("%w: missing %s", ErrIncompletePressures, k)
		}
	}
	return nil
}
