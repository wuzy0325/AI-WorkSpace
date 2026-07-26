package usecase

import (
	"path/filepath"
	"testing"

	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/core/device"
)

// totalTemperatureCsvLatestReader 为总温 CSV 测试提供通道数据。
// 通道布局：
//
//	0: testProbe=30℃      1: standardProbe=25℃   2: (未用)
//	3: totalPressure=200k  4: staticPressure=100k  5: atmosphericPressure=101325
//	6: atmosphericTemperature=25℃
//
// 压力组合满足 CalculateMach 的约束 Pt > Ps > 0，避免算法报错。
type totalTemperatureCsvLatestReader struct{}

func (r totalTemperatureCsvLatestReader) GetLatestData(_ string) (device.DataPayload, bool) {
	return device.DataPayload{
		Channels:       []float64{30, 25, 0, 200000, 100000, 101325, 25},
		ChannelIndices: []int{0, 1, 2, 3, 4, 5, 6},
	}, true
}

func (r totalTemperatureCsvLatestReader) GetLatestTimestamp(_ string) (int64, bool) { return 0, false }

// totalTemperatureCsvTestConfig 构造总温校准测试配置，包含 ValidateConfig 要求的
// 六个必需通道角色（testProbe/standardProbe/totalPressure/staticPressure/
// atmosphericPressure/atmosphericTemperature）和一个 Ma=0.5 的测点。
func totalTemperatureCsvTestConfig(taskID string) calibration.Config {
	return calibration.Config{
		TaskID:          taskID,
		Type:            string(calibration.TypeTotalTemperature),
		SamplesPerPoint: 1,
		Points: []calibration.CalPoint{
			{ID: 1, Coordinates: map[string]float64{"Ma": 0.5}},
		},
		ProbeChannels: []calibration.ProbeChannel{
			{Role: "testProbe", DeviceID: "dev-1", ChannelIndex: 0, Enabled: true},
			{Role: "standardProbe", DeviceID: "dev-1", ChannelIndex: 1, Enabled: true},
			{Role: "totalPressure", DeviceID: "dev-1", ChannelIndex: 3, Enabled: true},
			{Role: "staticPressure", DeviceID: "dev-1", ChannelIndex: 4, Enabled: true},
			{Role: "atmosphericPressure", DeviceID: "dev-1", ChannelIndex: 5, Enabled: true},
			{Role: "atmosphericTemperature", DeviceID: "dev-1", ChannelIndex: 6, Enabled: true},
		},
	}
}

// TestCalibrationManagerTotalTemperatureInitializesCsvWriterOnStart 验证总温校准在
// Start 时若 savePath 非空，会调用 csvWriter.Initialize 打开文件并写表头。
//
// 修复背景：autoTypes 不含总温（总温走手动 CollectCurrentPoint 路径），之前 Start
// 不调 Initialize，导致 CollectCurrentPoint 调 AppendPoint 时报"CSV写入器未初始化"，
// CSV 文件不会被创建。
func TestCalibrationManagerTotalTemperatureInitializesCsvWriterOnStart(t *testing.T) {
	manager := NewCalibrationManager(totalTemperatureCsvLatestReader{}, nil, nil, nil)
	writer := &fakeCalibrationCsvWriter{}
	manager.SetCsvWriter(writer)

	config := totalTemperatureCsvTestConfig("cal-ttotal-csv-init")
	config.SavePath = filepath.Join(t.TempDir(), "total-temperature.csv")

	if err := manager.Start(config); err != nil {
		t.Fatalf("start calibration: %v", err)
	}

	// Start 同步阶段已完成 csvWriter.Initialize，异步循环不影响此断言
	if writer.path != config.SavePath {
		t.Fatalf("expected csvWriter initialized with %q, got %q", config.SavePath, writer.path)
	}

	// 释放后台循环（MotionAxes 为空时 moveToPoint 直接跳过，循环会因总温
	// AcquireData 返回"请使用 AcquireDataWithChannels"而快速结束，不影响 Initialize）
	if err := manager.Stop(); err != nil {
		t.Fatalf("stop calibration: %v", err)
	}
}

// TestCalibrationManagerTotalTemperatureCollectCurrentPointAppendsToCsv 验证总温校准
// 在 csvWriter 已初始化的前提下，CollectCurrentPoint 能成功采集并 AppendPoint 到 CSV。
//
// 此测试绕过 Start 的异步采集循环（循环依赖 motion runtime，与 CSV 写入逻辑无关），
// 直接将 manager 置于 Running 态后调用 CollectCurrentPoint，聚焦验证 CSV 写入路径。
func TestCalibrationManagerTotalTemperatureCollectCurrentPointAppendsToCsv(t *testing.T) {
	manager := NewCalibrationManager(totalTemperatureCsvLatestReader{}, nil, nil, nil)
	writer := &fakeCalibrationCsvWriter{}
	// 模拟 Start 已 Initialize csvWriter（Initialize 仅设置 path，AppendPoint 依赖此状态）
	writer.path = filepath.Join(t.TempDir(), "total-temperature.csv")
	manager.SetCsvWriter(writer)

	config := totalTemperatureCsvTestConfig("cal-ttotal-csv-collect")
	manager.currentConfig = config
	manager.currentStatus = calibration.Status{
		TaskID:      config.TaskID,
		Type:        config.Type,
		State:       calibration.StateRunning,
		TotalPoints: len(config.Points),
	}

	if err := manager.CollectCurrentPoint(); err != nil {
		t.Fatalf("collect current point: %v", err)
	}

	if len(writer.points) != 1 {
		t.Fatalf("expected 1 point appended to csvWriter, got %d", len(writer.points))
	}
}
