package config

import (
	"strings"
	"testing"

	"wind-daq/services/api-go/internal/core/calibration"
)

// TestDecodeCalibrationConfig_NestedFrontendShape 验证前端发送的嵌套 channel 格式
// 能被正确解码为扁平的 calibration.ProbeChannel 字段。
//
// 前端 ProbeChannelConfig 结构为 { name, role, channel:{deviceId,channelIndex}, enabled }，
// 顶层不带 deviceId/channelIndex，必须通过 DTO 的 Channel 字段接收后再映射到扁平字段。
// 此测试对应从 core 迁移过来的 TestProbeChannelUnmarshalNestedFrontendShape。
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
// （顶层带 deviceId/channelIndex，无 channel 嵌套对象）仍能被正确解码。
// 这保证 HTTP API 与旧调用方不因 DTO 引入而破坏。
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
	// I-1: 补 ProbeChannels 全字段断言——JSON 输入用了嵌套 channel shape，
	// 必须确认 Role/Name/DeviceID/ChannelIndex/Enabled 都正确解码，防止 DTO 嵌套映射回归。
	if len(cfg.ProbeChannels) != 1 {
		t.Fatalf("expected 1 probe channel, got %d", len(cfg.ProbeChannels))
	}
	pc := cfg.ProbeChannels[0]
	if pc.Role != "fiveHole.p1" || pc.Name != "P1" || pc.DeviceID != "dev-1" || pc.ChannelIndex != 1 || !pc.Enabled {
		t.Fatalf("unexpected probeChannel fields: %+v", pc)
	}
	// I-2: MotionAxes 不仅断言 ControllerID，还要覆盖 Axis/Name，避免后续给 MotionAxisConfig
	// 加字段时 DTO 同步遗漏。
	if len(cfg.MotionAxes) != 1 ||
		cfg.MotionAxes[0].ControllerID != "mc-1" ||
		cfg.MotionAxes[0].Axis != "x" ||
		cfg.MotionAxes[0].Name != "X" {
		t.Fatalf("unexpected motionAxes: %+v", cfg.MotionAxes)
	}
	if cfg.SphereTankGate == nil || !cfg.SphereTankGate.Enabled || cfg.SphereTankGate.WaitTimeSec != 2.5 {
		t.Fatalf("unexpected sphereTankGate: %+v", cfg.SphereTankGate)
	}
	if cfg.SphereTankGate.StableTimeChannel.DeviceID != "dev-1" || cfg.SphereTankGate.StableTimeChannel.ChannelIndex != 2 {
		t.Fatalf("unexpected stableTimeChannel: %+v", cfg.SphereTankGate.StableTimeChannel)
	}
	// I-2: AcquisitionSampling 必须覆盖全部三个字段（BatchTimeoutMs/BatchPollIntervalMs/BatchMaxAgeMs），
	// 防止后续给 AcquisitionSamplingConfig 加字段时 DTO 同步遗漏。
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
//
// 这与 ToCore() 中 len(d.ProbeChannels) > 0 的判断语义一致：
// json.Unmarshal 把 "probeChannels": [] 解码为非 nil 的空切片（len==0），
// 但 ToCore 用 len > 0 判断，空切片不会进入赋值分支，cfg.ProbeChannels 保持 nil。
// 此测试防止后续重构（例如改用 != nil 判断或直接赋值）打破 nil 归一化语义。
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

// ==================== 七孔探针校准 DTO 集成测试（Task 8） ====================
//
// 七孔 DTO 无需扩展结构——CalibrationConfigDTO 的 ProbeChannels []ProbeChannelDTO
// 字段是类型无关的通用容器，11 个 sevenHole.* 角色字符串原样保留即可。
// 这些测试验证「DTO 通用解码 + SevenHoleAlgorithm.ValidateConfig 校验」的端到端链路。

// sevenHoleAllRolesJSON 构造 11 角色齐全的七孔 DTO JSON（前端嵌套 channel shape）。
// 复用此常量避免在多个测试中重复 11 行 probeChannels 定义。
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
//
// 测试前置：构造含 11 个 sevenHole.* 角色的 DTO JSON（前端嵌套 channel shape）
// 测试步骤：DecodeCalibrationConfig 解码 → SevenHoleAlgorithm.ValidateConfig 校验
// 期待结果：解码无错，11 个 ProbeChannels 角色字符串原样保留，ValidateConfig 返回 nil
func TestDecodeCalibrationConfig_SevenHoleAllRoles(t *testing.T) {
	cfg, err := DecodeCalibrationConfig([]byte(sevenHoleAllRolesJSON))
	if err != nil {
		t.Fatalf("decode seven-hole config: %v", err)
	}

	// DTO 通用字段断言
	if cfg.Type != string(calibration.TypeSevenHole) {
		t.Fatalf("Type: expected %q, got %q", calibration.TypeSevenHole, cfg.Type)
	}
	if cfg.TaskID != "cal-7h" || cfg.DeviceID != "dev-7h" {
		t.Fatalf("unexpected header: %+v", cfg)
	}
	if cfg.SamplesPerPoint != 10 {
		t.Fatalf("SamplesPerPoint: expected 10, got %d", cfg.SamplesPerPoint)
	}

	// 11 角色齐全断言
	if len(cfg.ProbeChannels) != 11 {
		t.Fatalf("expected 11 probe channels, got %d", len(cfg.ProbeChannels))
	}
	// 抽查 P7（中心孔）和 tAtm（温度）两个关键角色，确保嵌套 channel 解码后角色字符串原样保留
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

	// 端到端：解码后的 Config 必须通过 SevenHoleAlgorithm.ValidateConfig
	algo := calibration.NewSevenHoleAlgorithm()
	if err := algo.ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig should pass with all 11 roles: %v", err)
	}
}

// TestDecodeCalibrationConfig_SevenHoleMissingP7 验证缺少 sevenHole.p7 时 ValidateConfig 返回错误
//
// 测试前置：从 sevenHoleAllRolesJSON 中删除 sevenHole.p7 角色行
// 测试步骤：DecodeCalibrationConfig 解码 → ValidateConfig 校验
// 期待结果：解码无错（DTO 不校验角色），但 ValidateConfig 返回 "缺少必需通道角色" 错误
//
// 此测试对应 spec Task 8 验收标准 "构造缺 sevenHole.p7 的 DTO JSON，解码后 ValidateConfig 返回错误"。
// 选 p7 是因为中心孔是七孔探针的核心角色（内区公式分母 P7-P̄ 依赖 P7），
// 缺失会导致内区所有系数无法计算。
func TestDecodeCalibrationConfig_SevenHoleMissingP7(t *testing.T) {
	// 通过字符串替换删除 sevenHole.p7 行——保留其他 10 个角色
	missingP7JSON := strings.Replace(sevenHoleAllRolesJSON,
		`		{"role":"sevenHole.p7","name":"P7","channel":{"deviceId":"dev-7h","channelIndex":7},"enabled":true},
`, "", 1)

	cfg, err := DecodeCalibrationConfig([]byte(missingP7JSON))
	if err != nil {
		t.Fatalf("decode seven-hole config without p7: %v", err)
	}
	// DTO 解码不校验角色——只保留 10 个 ProbeChannels
	if len(cfg.ProbeChannels) != 10 {
		t.Fatalf("expected 10 probe channels (missing p7), got %d", len(cfg.ProbeChannels))
	}

	// ValidateConfig 必须捕获缺失角色
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
//
// 测试前置：从 sevenHoleAllRolesJSON 中删除 pTotal 和 pAtm 两个角色
// 测试步骤：DecodeCalibrationConfig 解码 → ValidateConfig 校验
// 期待结果：ValidateConfig 返回错误，错误信息同时包含 "sevenHole.pTotal" 和 "sevenHole.pAtm"
//
// 此测试覆盖 spec §3.2 规则之外的另一种缺失场景——多角色缺失时错误信息完整。
// 操作员根据错误信息一次性补齐所有缺失角色，避免逐个补齐多次试错。
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

// TestDecodeCalibrationConfig_SevenHoleDualCoordinates 验证七孔点位的 MotionCoordinates/Region/Sector 字段
// 通过 DTO 原样保留（CalPoint 在 Task 1 已扩展这三个字段，DTO 的 Points []calibration.CalPoint 直接复用）
//
// 测试前置：在 sevenHoleAllRolesJSON 基础上追加两个点位（内区 + 外区各一个），含双坐标
// 测试步骤：DecodeCalibrationConfig 解码 → 检查 Points 字段
// 期待结果：内区点 Coordinates={"α":5,"β":-3}，MotionCoordinates={"α":5,"β":-3}，Region="inner"，Sector=7
//          外区点 Coordinates={"θ":35,"φ":60}，MotionCoordinates={"α":..,"β":..}（按正向公式换算），Region="outer"，Sector=2
//
// 此测试覆盖 spec §3.4 双坐标模型——点位生成阶段一次性填 Coordinates + MotionCoordinates，
// 运行时 moveToPoint 只读 MotionCoordinates，避免运行时换算。
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

	// 内区点
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

	// 外区点
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

	// 端到端：含双坐标点位的 Config 也必须通过 ValidateConfig
	algo := calibration.NewSevenHoleAlgorithm()
	if err := algo.ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig should pass with dual coordinates: %v", err)
	}
}

// TestDecodeCalibrationConfig_SevenHoleSamplesPerPointZero 验证 SamplesPerPoint=0 时 ValidateConfig 返回错误
//
// 测试前置：在 sevenHoleAllRolesJSON 基础上把 samplesPerPoint 改为 0
// 测试步骤：DecodeCalibrationConfig 解码 → ValidateConfig 校验
// 期待结果：ValidateConfig 返回 "samplesPerPoint 必须大于0" 错误
//
// 此测试覆盖 spec Task 5 中 ValidateConfig 的另一条校验路径——
// 即使 11 角色齐全，SamplesPerPoint=0 也无法采集（会导致除零或无限循环）。
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