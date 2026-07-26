// Package types 的校准 DTO 单元测试（Task 05：从 internal/adapters/config 迁移）。
//
// 这些测试覆盖：
//  1. 嵌套/扁平 channel 两种 JSON shape 的解码（与原 adapters/config 测试对齐）
//  2. ToCore 完整字段保留（防止后续给 calibration.Config 加字段时遗漏 DTO 同步）
//  3. 七孔 11 角色齐全/缺失/双坐标等场景
//  4. MotionSafety 字段保留（Task 05 新增——原 DTO 遗漏此字段导致前端配置被静默丢弃）
package types

import (
	"strings"
	"testing"

	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/core/traversal"
)

// TestDecodeCalibrationConfig_NestedFrontendShape 验证前端发送的嵌套 channel 格式
// 能被正确解码为扁平的 calibration.ProbeChannel 字段。
//
// 前端 ProbeChannelConfig 结构为 { name, role, channel:{deviceId,channelIndex}, enabled }，
// 顶层不带 deviceId/channelIndex，必须通过 DTO 的 Channel 字段接收后再映射到扁平字段。
func TestDecodeCalibrationConfig_NestedFrontendShape(t *testing.T) {
	jsonData := []byte(`{
		"taskId": "cal-1",
		"type": "five-hole",
		"name": "五孔校准",
		"probeChannels": [
			{"role":"fiveHole.p1","name":"P1","enabled":true,"channel":{"deviceId":"dev-1","channelIndex":7}}
		]
	}`)

	cfg, err := DecodeCalibrationConfig(jsonData)
	if err != nil {
		t.Fatalf("decode calibration config: %v", err)
	}

	if len(cfg.ProbeChannels) != 1 {
		t.Fatalf("expected 1 probe channel, got %d", len(cfg.ProbeChannels))
	}
	ch := cfg.ProbeChannels[0]
	if ch.DeviceID != "dev-1" || ch.ChannelIndex != 7 {
		t.Fatalf("expected nested channel mapping to dev-1/7, got device=%q channel=%d", ch.DeviceID, ch.ChannelIndex)
	}
	if ch.Role != "fiveHole.p1" || ch.Name != "P1" || !ch.Enabled {
		t.Fatalf("unexpected probe channel fields: %+v", ch)
	}
}

// TestDecodeCalibrationConfig_FlatBackendShape 验证旧后端/直接构造的扁平格式
// 仍能被正确解码，保证 HTTP API 与旧调用方不因 DTO 迁移而破坏。
func TestDecodeCalibrationConfig_FlatBackendShape(t *testing.T) {
	jsonData := []byte(`{
		"taskId": "cal-2",
		"type": "three-hole",
		"probeChannels": [
			{"role":"threeHole.p1","name":"P1","deviceId":"dev-flat","channelIndex":3,"enabled":true}
		]
	}`)

	cfg, err := DecodeCalibrationConfig(jsonData)
	if err != nil {
		t.Fatalf("decode calibration config: %v", err)
	}

	ch := cfg.ProbeChannels[0]
	if ch.DeviceID != "dev-flat" || ch.ChannelIndex != 3 {
		t.Fatalf("expected flat channel mapping to dev-flat/3, got device=%q channel=%d", ch.DeviceID, ch.ChannelIndex)
	}
}

// TestDecodeCalibrationConfig_NestedOverridesFlat 验证当同时存在嵌套 channel 和扁平字段时，
// 嵌套 channel 覆盖扁平字段。与原 ProbeChannel.UnmarshalJSON 语义一致。
func TestDecodeCalibrationConfig_NestedOverridesFlat(t *testing.T) {
	jsonData := []byte(`{
		"taskId": "cal-3",
		"type": "five-hole",
		"probeChannels": [
			{"role":"fiveHole.p1","deviceId":"flat-dev","channelIndex":1,"channel":{"deviceId":"nested-dev","channelIndex":9}}
		]
	}`)

	cfg, err := DecodeCalibrationConfig(jsonData)
	if err != nil {
		t.Fatalf("decode calibration config: %v", err)
	}

	ch := cfg.ProbeChannels[0]
	if ch.DeviceID != "nested-dev" || ch.ChannelIndex != 9 {
		t.Fatalf("expected nested channel to override flat, got device=%q channel=%d", ch.DeviceID, ch.ChannelIndex)
	}
}

// TestDecodeCalibrationConfig_PreservesAllFields 验证 DTO 完整覆盖 Config 的所有字段，
// ToCore 不丢字段。这是防止后续给 calibration.Config 加字段时遗漏 DTO 同步。
func TestDecodeCalibrationConfig_PreservesAllFields(t *testing.T) {
	jsonData := []byte(`{
		"taskId": "cal-full",
		"deviceId": "dev-host",
		"type": "total-temperature",
		"channels": [1, 2, 3],
		"pressurePoints": [100.0, 200.0],
		"averageSamples": 8,
		"probeChannels": [
			{"role":"fiveHole.p1","name":"P1","channel":{"deviceId":"dev-1","channelIndex":1},"enabled":true}
		],
		"points": [{"id": 1, "coordinates": {"x": 1.5}}],
		"samplesPerPoint": 16,
		"dwellTimeMs": 500,
		"stopOnError": true,
		"name": "全字段校准",
		"savePath": "data/out",
		"motionAxes": [{"controllerId":"mc-1","axis":"x","name":"X"}],
		"motionSafety": {
			"arrivalTolerance": 0.05,
			"criticalDeviationLimit": 1.0,
			"noProgressTimeoutMs": 1500,
			"progressEpsilon": 0.002
		},
		"sphereTankGate": {"enabled": true, "waitTimeSec": 2.5, "stableTimeChannel": {"deviceId":"dev-1","channelIndex":2}},
		"acquisitionSampling": {"batchTimeoutMs": 100, "batchPollIntervalMs": 10, "batchMaxAgeMs": 1000},
		"totalTemperatureConfig": {
			"probeChannels": {"testProbe": {"deviceId":"dev-1","channelIndex":5}},
			"targetMachNumbers": [0.3, 0.6],
			"machTolerance": 0.01,
			"stabilityCriteria": {"sampleCount": 10, "sampleInterval": 50, "maxStdDev": 0.1},
			"samplesPerPoint": 20,
			"sampleInterval": 100,
			"enableFitting": true
		}
	}`)

	cfg, err := DecodeCalibrationConfig(jsonData)
	if err != nil {
		t.Fatalf("decode calibration config: %v", err)
	}

	if cfg.TaskID != "cal-full" || cfg.DeviceID != "dev-host" || cfg.Type != "total-temperature" {
		t.Fatalf("unexpected header fields: %+v", cfg)
	}
	if len(cfg.Channels) != 3 || cfg.Channels[0] != 1 {
		t.Fatalf("unexpected channels: %v", cfg.Channels)
	}
	if len(cfg.PressurePoints) != 2 || cfg.PressurePoints[1] != 200.0 {
		t.Fatalf("unexpected pressurePoints: %v", cfg.PressurePoints)
	}
	if cfg.AverageSamples != 8 {
		t.Fatalf("unexpected averageSamples: %d", cfg.AverageSamples)
	}
	if cfg.SamplesPerPoint != 16 || cfg.DwellTimeMs != 500 || !cfg.StopOnError {
		t.Fatalf("unexpected numeric/bool fields: samples=%d dwell=%d stop=%v", cfg.SamplesPerPoint, cfg.DwellTimeMs, cfg.StopOnError)
	}
	if cfg.Name != "全字段校准" || cfg.SavePath != "data/out" {
		t.Fatalf("unexpected name/savePath: %q %q", cfg.Name, cfg.SavePath)
	}
	if len(cfg.Points) != 1 || cfg.Points[0].ID != 1 {
		t.Fatalf("unexpected points: %+v", cfg.Points)
	}
	if cfg.Points[0].Coordinates["x"] != 1.5 {
		t.Fatalf("unexpected point coordinates: %v", cfg.Points[0].Coordinates)
	}
	if len(cfg.ProbeChannels) != 1 {
		t.Fatalf("expected 1 probe channel, got %d", len(cfg.ProbeChannels))
	}
	pc := cfg.ProbeChannels[0]
	if pc.Role != "fiveHole.p1" || pc.Name != "P1" || pc.DeviceID != "dev-1" || pc.ChannelIndex != 1 || !pc.Enabled {
		t.Fatalf("unexpected probeChannel fields: %+v", pc)
	}
	if len(cfg.MotionAxes) != 1 ||
		cfg.MotionAxes[0].ControllerID != "mc-1" ||
		cfg.MotionAxes[0].Axis != "x" ||
		cfg.MotionAxes[0].Name != "X" {
		t.Fatalf("unexpected motionAxes: %+v", cfg.MotionAxes)
	}
	// MotionSafety 必须完整保留——这是 Task 05 修复的核心字段。
	// 原 adapters/config DTO 遗漏此字段导致前端配置被静默丢弃。
	if cfg.MotionSafety == nil {
		t.Fatalf("expected motionSafety to be set, got nil (Task 05 regression)")
	}
	if cfg.MotionSafety.ArrivalTolerance == nil || *cfg.MotionSafety.ArrivalTolerance != 0.05 {
		t.Fatalf("unexpected motionSafety.arrivalTolerance: %+v", cfg.MotionSafety.ArrivalTolerance)
	}
	if cfg.MotionSafety.CriticalDeviationLimit == nil || *cfg.MotionSafety.CriticalDeviationLimit != 1.0 {
		t.Fatalf("unexpected motionSafety.criticalDeviationLimit: %+v", cfg.MotionSafety.CriticalDeviationLimit)
	}
	if cfg.MotionSafety.NoProgressTimeoutMs == nil || *cfg.MotionSafety.NoProgressTimeoutMs != 1500 {
		t.Fatalf("unexpected motionSafety.noProgressTimeoutMs: %+v", cfg.MotionSafety.NoProgressTimeoutMs)
	}
	if cfg.MotionSafety.ProgressEpsilon == nil || *cfg.MotionSafety.ProgressEpsilon != 0.002 {
		t.Fatalf("unexpected motionSafety.progressEpsilon: %+v", cfg.MotionSafety.ProgressEpsilon)
	}
	if cfg.SphereTankGate == nil || !cfg.SphereTankGate.Enabled || cfg.SphereTankGate.WaitTimeSec != 2.5 {
		t.Fatalf("unexpected sphereTankGate: %+v", cfg.SphereTankGate)
	}
	if cfg.SphereTankGate.StableTimeChannel.DeviceID != "dev-1" || cfg.SphereTankGate.StableTimeChannel.ChannelIndex != 2 {
		t.Fatalf("unexpected stableTimeChannel: %+v", cfg.SphereTankGate.StableTimeChannel)
	}
	if cfg.AcquisitionSampling == nil ||
		cfg.AcquisitionSampling.BatchTimeoutMs != 100 ||
		cfg.AcquisitionSampling.BatchPollIntervalMs != 10 ||
		cfg.AcquisitionSampling.BatchMaxAgeMs != 1000 {
		t.Fatalf("unexpected acquisitionSampling: %+v", cfg.AcquisitionSampling)
	}
	if cfg.TotalTemperatureConfig == nil {
		t.Fatalf("expected totalTemperatureConfig to be set")
	}
	ttc := cfg.TotalTemperatureConfig
	if ttc.ProbeChannels["testProbe"].DeviceID != "dev-1" || ttc.ProbeChannels["testProbe"].ChannelIndex != 5 {
		t.Fatalf("unexpected totalTemperature probe channel: %+v", ttc.ProbeChannels["testProbe"])
	}
	if len(ttc.TargetMachNumbers) != 2 || ttc.TargetMachNumbers[0] != 0.3 {
		t.Fatalf("unexpected targetMachNumbers: %v", ttc.TargetMachNumbers)
	}
	if ttc.MachTolerance != 0.01 {
		t.Fatalf("unexpected machTolerance: %v", ttc.MachTolerance)
	}
	if ttc.StabilityCriteria.SampleCount != 10 || ttc.StabilityCriteria.SampleInterval != 50 || ttc.StabilityCriteria.MaxStdDev != 0.1 {
		t.Fatalf("unexpected stabilityCriteria: %+v", ttc.StabilityCriteria)
	}
	if ttc.SamplesPerPoint != 20 || ttc.SampleInterval != 100 || !ttc.EnableFitting {
		t.Fatalf("unexpected totalTemperature numeric/bool fields: %+v", ttc)
	}
}

// TestDecodeCalibrationConfig_InvalidJSON 验证非法 JSON 返回错误而非 panic。
func TestDecodeCalibrationConfig_InvalidJSON(t *testing.T) {
	if _, err := DecodeCalibrationConfig([]byte(`{not-json`)); err == nil {
		t.Fatal("expected error for invalid json, got nil")
	}
}

// TestCalibrationConfigDTO_ToCore_EmptyProbeChannels 验证空 ProbeChannels 不 panic 且不产生 nil 切片歧义。
func TestCalibrationConfigDTO_ToCore_EmptyProbeChannels(t *testing.T) {
	dto := CalibrationConfigDTO{
		TaskID: "cal-empty",
		Type:   "five-hole",
	}
	cfg := dto.ToCore()
	if cfg.TaskID != "cal-empty" || cfg.Type != "five-hole" {
		t.Fatalf("unexpected ToCore result: %+v", cfg)
	}
	if cfg.ProbeChannels != nil {
		t.Fatalf("expected nil ProbeChannels for empty input, got %v", cfg.ProbeChannels)
	}
}

// TestDecodeCalibrationConfig_EmptyProbeChannelsArray 验证 probeChannels 为空数组 [] 时，
// DecodeCalibrationConfig 返回的 cfg.ProbeChannels 为 nil。
func TestDecodeCalibrationConfig_EmptyProbeChannelsArray(t *testing.T) {
	jsonData := []byte(`{
		"taskId": "cal-empty-array",
		"type": "five-hole",
		"probeChannels": []
	}`)

	cfg, err := DecodeCalibrationConfig(jsonData)
	if err != nil {
		t.Fatalf("decode calibration config: %v", err)
	}
	if cfg.ProbeChannels != nil {
		t.Fatalf("expected nil ProbeChannels for empty array, got %v", cfg.ProbeChannels)
	}
}

// TestDecodeCalibrationConfig_MotionSafetyNil 验证 JSON 不含 motionSafety 字段时
// cfg.MotionSafety 为 nil（下游使用 DefaultMotionSafety）。
//
// 这是向后兼容场景：旧前端/旧配置不发送 motionSafety，DTO 不应构造默认值，
// 而是原样保留 nil，由后端 Start 走 validateCalibrationMotionSafetyConfig 的 nil 路径。
func TestDecodeCalibrationConfig_MotionSafetyNil(t *testing.T) {
	jsonData := []byte(`{
		"taskId": "cal-no-safety",
		"type": "five-hole"
	}`)

	cfg, err := DecodeCalibrationConfig(jsonData)
	if err != nil {
		t.Fatalf("decode calibration config: %v", err)
	}
	if cfg.MotionSafety != nil {
		t.Fatalf("expected nil MotionSafety when JSON omits field, got %+v", cfg.MotionSafety)
	}
}

// TestDecodeCalibrationConfig_MotionSafetyWithAxisOverrides 验证 motionSafety.axisOverrides
// 子字段能通过 DTO 完整保留——这是 R-5 后端权威校验的关键场景。
//
// 测试前置：构造含 axisOverrides 的 motionSafety JSON
// 测试步骤：DecodeCalibrationConfig 解码
// 期待结果：cfg.MotionSafety.AxisOverrides 非空，"x" 轴覆盖项的 ArrivalTolerance 为 0.01
func TestDecodeCalibrationConfig_MotionSafetyWithAxisOverrides(t *testing.T) {
	jsonData := []byte(`{
		"taskId": "cal-override",
		"type": "five-hole",
		"motionSafety": {
			"arrivalTolerance": 0.1,
			"axisOverrides": {
				"x": {"arrivalTolerance": 0.01}
			}
		}
	}`)

	cfg, err := DecodeCalibrationConfig(jsonData)
	if err != nil {
		t.Fatalf("decode calibration config: %v", err)
	}
	if cfg.MotionSafety == nil {
		t.Fatalf("expected motionSafety to be set")
	}
	if cfg.MotionSafety.AxisOverrides == nil {
		t.Fatalf("expected axisOverrides to be set")
	}
	xOverride, ok := cfg.MotionSafety.AxisOverrides["x"]
	if !ok {
		t.Fatalf("expected axisOverrides to contain 'x'")
	}
	if xOverride == nil || xOverride.ArrivalTolerance == nil || *xOverride.ArrivalTolerance != 0.01 {
		t.Fatalf("unexpected x axis override: %+v", xOverride)
	}
	// 顶层 ArrivalTolerance 也必须保留——axisOverrides 不影响顶层默认值
	if cfg.MotionSafety.ArrivalTolerance == nil || *cfg.MotionSafety.ArrivalTolerance != 0.1 {
		t.Fatalf("unexpected top-level arrivalTolerance: %+v", cfg.MotionSafety.ArrivalTolerance)
	}
}

// TestCalibrationConfigDTO_ToCore_MotionSafetyPointer 验证 ToCore 保留 MotionSafety 指针语义——
// 指针字段必须原样传递（共享底层值），让后端 Start 能拿到非 nil 配置并通过校验。
//
// 这防止后续重构时把 MotionSafety 改成值类型导致 nil 语义丢失。
func TestCalibrationConfigDTO_ToCore_MotionSafetyPointer(t *testing.T) {
	tol := 0.05
	dto := CalibrationConfigDTO{
		TaskID: "cal-ptr",
		Type:   "five-hole",
		MotionSafety: &traversal.MotionSafetyConfig{
			ArrivalTolerance: &tol,
		},
	}
	cfg := dto.ToCore()
	if cfg.MotionSafety == nil {
		t.Fatalf("expected MotionSafety pointer to be preserved")
	}
	if cfg.MotionSafety.ArrivalTolerance == nil || *cfg.MotionSafety.ArrivalTolerance != 0.05 {
		t.Fatalf("expected ArrivalTolerance 0.05, got %+v", cfg.MotionSafety.ArrivalTolerance)
	}
}

// ==================== 七孔探针校准 DTO 集成测试 ====================

const sevenHoleAllRolesJSON = `{
	"taskId": "cal-7h",
	"deviceId": "dev-7h",
	"type": "seven-hole",
	"name": "七孔校准-全角色",
	"samplesPerPoint": 10,
	"probeChannels": [
		{"role":"sevenHole.p1","name":"P1","channel":{"deviceId":"dev-7h","channelIndex":1},"enabled":true},
		{"role":"sevenHole.p2","name":"P2","channel":{"deviceId":"dev-7h","channelIndex":2},"enabled":true},
		{"role":"sevenHole.p3","name":"P3","channel":{"deviceId":"dev-7h","channelIndex":3},"enabled":true},
		{"role":"sevenHole.p4","name":"P4","channel":{"deviceId":"dev-7h","channelIndex":4},"enabled":true},
		{"role":"sevenHole.p5","name":"P5","channel":{"deviceId":"dev-7h","channelIndex":5},"enabled":true},
		{"role":"sevenHole.p6","name":"P6","channel":{"deviceId":"dev-7h","channelIndex":6},"enabled":true},
		{"role":"sevenHole.p7","name":"P7","channel":{"deviceId":"dev-7h","channelIndex":7},"enabled":true},
		{"role":"sevenHole.pTotal","name":"Pt","channel":{"deviceId":"dev-7h","channelIndex":8},"enabled":true},
		{"role":"sevenHole.pTunnelStatic","name":"Ps","channel":{"deviceId":"dev-7h","channelIndex":9},"enabled":true},
		{"role":"sevenHole.pAtm","name":"P∞","channel":{"deviceId":"dev-7h","channelIndex":10},"enabled":true},
		{"role":"sevenHole.tAtm","name":"T∞","channel":{"deviceId":"dev-7h","channelIndex":11},"enabled":true}
	]
}`

// TestDecodeCalibrationConfig_SevenHoleAllRoles 验证 11 角色齐全的七孔 DTO 解码后通过 ValidateConfig
func TestDecodeCalibrationConfig_SevenHoleAllRoles(t *testing.T) {
	cfg, err := DecodeCalibrationConfig([]byte(sevenHoleAllRolesJSON))
	if err != nil {
		t.Fatalf("decode seven-hole config: %v", err)
	}

	if cfg.Type != string(calibration.TypeSevenHole) {
		t.Fatalf("Type: expected %q, got %q", calibration.TypeSevenHole, cfg.Type)
	}
	if cfg.TaskID != "cal-7h" || cfg.DeviceID != "dev-7h" {
		t.Fatalf("unexpected header: %+v", cfg)
	}
	if cfg.SamplesPerPoint != 10 {
		t.Fatalf("SamplesPerPoint: expected 10, got %d", cfg.SamplesPerPoint)
	}
	if len(cfg.ProbeChannels) != 11 {
		t.Fatalf("expected 11 probe channels, got %d", len(cfg.ProbeChannels))
	}
	roleSet := make(map[string]calibration.ProbeChannel, len(cfg.ProbeChannels))
	for _, ch := range cfg.ProbeChannels {
		roleSet[ch.Role] = ch
	}
	if pc, ok := roleSet["sevenHole.p7"]; !ok {
		t.Fatalf("missing role sevenHole.p7")
	} else if pc.DeviceID != "dev-7h" || pc.ChannelIndex != 7 || !pc.Enabled {
		t.Fatalf("sevenHole.p7 fields not preserved: %+v", pc)
	}
	if pc, ok := roleSet["sevenHole.tAtm"]; !ok {
		t.Fatalf("missing role sevenHole.tAtm")
	} else if pc.ChannelIndex != 11 {
		t.Fatalf("sevenHole.tAtm channelIndex: expected 11, got %d", pc.ChannelIndex)
	}

	algo := calibration.NewSevenHoleAlgorithm()
	if err := algo.ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig should pass with all 11 roles: %v", err)
	}
}

// TestDecodeCalibrationConfig_SevenHoleMissingP7 验证缺少 sevenHole.p7 时 ValidateConfig 返回错误
func TestDecodeCalibrationConfig_SevenHoleMissingP7(t *testing.T) {
	missingP7JSON := strings.Replace(sevenHoleAllRolesJSON,
		`		{"role":"sevenHole.p7","name":"P7","channel":{"deviceId":"dev-7h","channelIndex":7},"enabled":true},
`, "", 1)

	cfg, err := DecodeCalibrationConfig([]byte(missingP7JSON))
	if err != nil {
		t.Fatalf("decode seven-hole config without p7: %v", err)
	}
	if len(cfg.ProbeChannels) != 10 {
		t.Fatalf("expected 10 probe channels (missing p7), got %d", len(cfg.ProbeChannels))
	}

	algo := calibration.NewSevenHoleAlgorithm()
	err = algo.ValidateConfig(cfg)
	if err == nil {
		t.Fatal("ValidateConfig should return error when sevenHole.p7 is missing")
	}
	if !strings.Contains(err.Error(), "sevenHole.p7") {
		t.Fatalf("error should mention sevenHole.p7, got: %v", err)
	}
}

// TestDecodeCalibrationConfig_SevenHoleMissingMultipleRoles 验证缺多个角色时错误信息列出全部缺失角色
func TestDecodeCalibrationConfig_SevenHoleMissingMultipleRoles(t *testing.T) {
	missingMultipleJSON := strings.Replace(sevenHoleAllRolesJSON,
		`		{"role":"sevenHole.pTotal","name":"Pt","channel":{"deviceId":"dev-7h","channelIndex":8},"enabled":true},
`, "", 1)
	missingMultipleJSON = strings.Replace(missingMultipleJSON,
		`		{"role":"sevenHole.pAtm","name":"P∞","channel":{"deviceId":"dev-7h","channelIndex":10},"enabled":true},
`, "", 1)

	cfg, err := DecodeCalibrationConfig([]byte(missingMultipleJSON))
	if err != nil {
		t.Fatalf("decode seven-hole config without pTotal/pAtm: %v", err)
	}
	if len(cfg.ProbeChannels) != 9 {
		t.Fatalf("expected 9 probe channels (missing pTotal+pAtm), got %d", len(cfg.ProbeChannels))
	}

	algo := calibration.NewSevenHoleAlgorithm()
	err = algo.ValidateConfig(cfg)
	if err == nil {
		t.Fatal("ValidateConfig should return error when multiple roles are missing")
	}
	if !strings.Contains(err.Error(), "sevenHole.pTotal") {
		t.Fatalf("error should mention sevenHole.pTotal, got: %v", err)
	}
	if !strings.Contains(err.Error(), "sevenHole.pAtm") {
		t.Fatalf("error should mention sevenHole.pAtm, got: %v", err)
	}
}

// TestDecodeCalibrationConfig_SevenHoleDualCoordinates 验证七孔点位的双坐标字段通过 DTO 原样保留
func TestDecodeCalibrationConfig_SevenHoleDualCoordinates(t *testing.T) {
	jsonData := sevenHoleAllRolesJSON[:len(sevenHoleAllRolesJSON)-2] + `,
	"points": [
		{
			"id": 1,
			"coordinates": {"α": 5.0, "β": -3.0},
			"motionCoordinates": {"α": 5.0, "β": -3.0},
			"region": "inner",
			"sector": 7
		},
		{
			"id": 170,
			"coordinates": {"θ": 35.0, "φ": 60.0},
			"motionCoordinates": {"α": -20.0, "β": 17.5},
			"region": "outer",
			"sector": 2
		}
	]
}`

	cfg, err := DecodeCalibrationConfig([]byte(jsonData))
	if err != nil {
		t.Fatalf("decode seven-hole config with dual coordinates: %v", err)
	}
	if len(cfg.Points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(cfg.Points))
	}
	inner := cfg.Points[0]
	if inner.ID != 1 {
		t.Fatalf("inner point ID: expected 1, got %d", inner.ID)
	}
	if inner.Coordinates["α"] != 5.0 || inner.Coordinates["β"] != -3.0 {
		t.Fatalf("inner Coordinates: expected α=5 β=-3, got %+v", inner.Coordinates)
	}
	if inner.MotionCoordinates["α"] != 5.0 || inner.MotionCoordinates["β"] != -3.0 {
		t.Fatalf("inner MotionCoordinates: expected α=5 β=-3, got %+v", inner.MotionCoordinates)
	}
	if inner.Region != "inner" || inner.Sector != 7 {
		t.Fatalf("inner Region/Sector: expected inner/7, got %q/%d", inner.Region, inner.Sector)
	}
	outer := cfg.Points[1]
	if outer.ID != 170 {
		t.Fatalf("outer point ID: expected 170, got %d", outer.ID)
	}
	if outer.Coordinates["θ"] != 35.0 || outer.Coordinates["φ"] != 60.0 {
		t.Fatalf("outer Coordinates: expected θ=35 φ=60, got %+v", outer.Coordinates)
	}
	if outer.MotionCoordinates["α"] != -20.0 || outer.MotionCoordinates["β"] != 17.5 {
		t.Fatalf("outer MotionCoordinates: expected α=-20 β=17.5, got %+v", outer.MotionCoordinates)
	}
	if outer.Region != "outer" || outer.Sector != 2 {
		t.Fatalf("outer Region/Sector: expected outer/2, got %q/%d", outer.Region, outer.Sector)
	}

	algo := calibration.NewSevenHoleAlgorithm()
	if err := algo.ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig should pass with dual coordinates: %v", err)
	}
}

// TestDecodeCalibrationConfig_SevenHoleSamplesPerPointZero 验证 SamplesPerPoint=0 时 ValidateConfig 返回错误
func TestDecodeCalibrationConfig_SevenHoleSamplesPerPointZero(t *testing.T) {
	zeroSamplesJSON := strings.Replace(sevenHoleAllRolesJSON,
		`"samplesPerPoint": 10`, `"samplesPerPoint": 0`, 1)

	cfg, err := DecodeCalibrationConfig([]byte(zeroSamplesJSON))
	if err != nil {
		t.Fatalf("decode seven-hole config with samplesPerPoint=0: %v", err)
	}

	algo := calibration.NewSevenHoleAlgorithm()
	err = algo.ValidateConfig(cfg)
	if err == nil {
		t.Fatal("ValidateConfig should return error when samplesPerPoint=0")
	}
	if !strings.Contains(err.Error(), "samplesPerPoint") {
		t.Fatalf("error should mention samplesPerPoint, got: %v", err)
	}
}
