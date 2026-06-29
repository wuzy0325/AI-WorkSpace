package config

import (
	"testing"
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
	if len(cfg.MotionAxes) != 1 || cfg.MotionAxes[0].ControllerID != "mc-1" {
		t.Fatalf("unexpected motionAxes: %+v", cfg.MotionAxes)
	}
	if cfg.SphereTankGate == nil || !cfg.SphereTankGate.Enabled || cfg.SphereTankGate.WaitTimeSec != 2.5 {
		t.Fatalf("unexpected sphereTankGate: %+v", cfg.SphereTankGate)
	}
	if cfg.SphereTankGate.StableTimeChannel.DeviceID != "dev-1" || cfg.SphereTankGate.StableTimeChannel.ChannelIndex != 2 {
		t.Fatalf("unexpected stableTimeChannel: %+v", cfg.SphereTankGate.StableTimeChannel)
	}
	if cfg.AcquisitionSampling == nil || cfg.AcquisitionSampling.BatchTimeoutMs != 100 {
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