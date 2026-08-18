package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"windlabx4/services/api-go/internal/core/device"
	corestorage "windlabx4/services/api-go/internal/core/storage"
)

func TestCSVRecordingSinkWritesPayloadsToFile(t *testing.T) {
	dir := t.TempDir()
	sink := NewCSVRecordingSink()

	if err := sink.Start(corestorage.RecordingConfig{OutputDir: dir, FilePrefix: "run"}); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if err := sink.Write(device.DataPayload{DeviceID: "sim-1", Timestamp: 123, ChannelIndices: []int{0, 1}, Channels: []float64{1.2, 3.4}}); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if err := sink.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "run-*.csv"))
	if err != nil {
		t.Fatalf("glob recording files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one recording file, got %d", len(files))
	}
	content, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read recording file: %v", err)
	}
	text := string(content)
	// 新格式：动态宽格式，首帧决定列布局，时间戳使用 'YYYY-MM-DD HH:MM:SS 前缀单引号（秒级）
	if !strings.Contains(text, "Timestamp,CH01,CH02") {
		t.Fatalf("expected CSV wide header, got %q", text)
	}
	// 一行应含时间戳 + 两个通道值（值为 6 位小数）
	if !strings.Contains(text, "1.200000") || !strings.Contains(text, "3.400000") {
		t.Fatalf("expected payload row with channel values, got %q", text)
	}
}

func TestCSVRecordingSinkReturnsErrorWhenNotStarted(t *testing.T) {
	sink := NewCSVRecordingSink()

	if err := sink.Write(device.DataPayload{DeviceID: "sim-1"}); err == nil {
		t.Fatal("expected write before start to fail")
	}
}

// TestCSVRecordingSinkDAQP1603HeaderWithSensorTypeSuffix 验证 DAQ-P-1603 在注入通道元数据后
// CSV 表头按 SensorType 生成单位后缀（pressure→_Pa，temperature→_degC）。
//
// 测试前置：
//   - 构造 RecordingConfig.DeviceChannels，包含 3 通道（前 2 压力、第 3 温度）
//   - 设备类型为 DAQ-P-1603
//
// 测试步骤：
//   - Start sink → Write 一帧 → Stop
//
// 期待结果：
//   - CSV 文件表头为 "Timestamp,CH01_Pa,CH02_Pa,CH03_degC"
//   - 数据行包含 3 个通道值
func TestCSVRecordingSinkDAQP1603HeaderWithSensorTypeSuffix(t *testing.T) {
	dir := t.TempDir()
	sink := NewCSVRecordingSink()

	channels := []device.ChannelConfig{
		{Index: 0, Name: "P1", Unit: "Pa", SensorType: device.SensorPressure},
		{Index: 1, Name: "P2", Unit: "Pa", SensorType: device.SensorPressure},
		{Index: 2, Name: "T1", Unit: "℃", SensorType: device.SensorTemperature},
	}
	cfg := corestorage.RecordingConfig{
		OutputDir:      dir,
		FilePrefix:     "p1603",
		DeviceChannels: map[string][]device.ChannelConfig{"p1603-1": channels},
	}
	if err := sink.Start(cfg); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	payload := device.DataPayload{
		DeviceID:       "p1603-1",
		DeviceType:     device.DeviceDAQP1603,
		Timestamp:      123,
		ChannelIndices: []int{0, 1, 2},
		Channels:       []float64{101.2, 202.3, 25.6},
	}
	if err := sink.Write(payload); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if err := sink.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "p1603-*.csv"))
	if err != nil {
		t.Fatalf("glob recording files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one recording file, got %d", len(files))
	}
	content, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read recording file: %v", err)
	}
	text := string(content)
	// 表头应按 SensorType 生成后缀：前两通道压力 _Pa，第三通道温度 _degC
	expectedHeader := "Timestamp,CH01_Pa,CH02_Pa,CH03_degC"
	if !strings.Contains(text, expectedHeader) {
		t.Fatalf("expected DAQ-P-1603 header with sensor type suffix %q, got %q", expectedHeader, text)
	}
	// 数据行应包含 3 个通道值
	for _, want := range []string{"101.200000", "202.300000", "25.600000"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected payload row to contain %q, got %q", want, text)
		}
	}
}

// TestCSVRecordingSinkDAQP1603FallbackToGenericHeader 验证 DAQ-P-1603 在未注入通道元数据时
// 表头退化为通用 CH01..CHnn 格式（兼容录制中后连接的设备）。
//
// 测试前置：
//   - RecordingConfig.DeviceChannels 为空
//   - 设备类型为 DAQ-P-1603
//
// 测试步骤：
//   - Start sink → Write 一帧 → Stop
//
// 期待结果：
//   - CSV 文件表头为 "Timestamp,CH01,CH02"（无单位后缀）
func TestCSVRecordingSinkDAQP1603FallbackToGenericHeader(t *testing.T) {
	dir := t.TempDir()
	sink := NewCSVRecordingSink()

	cfg := corestorage.RecordingConfig{
		OutputDir:  dir,
		FilePrefix: "p1603-nometa",
	}
	if err := sink.Start(cfg); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	payload := device.DataPayload{
		DeviceID:       "p1603-2",
		DeviceType:     device.DeviceDAQP1603,
		Timestamp:      456,
		ChannelIndices: []int{0, 1},
		Channels:       []float64{1.0, 2.0},
	}
	if err := sink.Write(payload); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if err := sink.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "p1603-nometa-*.csv"))
	if err != nil {
		t.Fatalf("glob recording files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one recording file, got %d", len(files))
	}
	content, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read recording file: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "Timestamp,CH01,CH02") {
		t.Fatalf("expected generic header without suffix, got %q", text)
	}
}

// TestCSVRecordingSinkDAQP1604WideFormatHeaderUnchanged 验证 DAQ-P-1604 固定 18 列宽格式表头不受
// channelConfigs 注入影响（保持与 daq-p1604 项目的对齐）。
//
// 测试前置：
//   - 设备类型为 DAQ-P-1604，通道数 18
//   - 同时注入 DeviceChannels（模拟 server.go 总是注入的场景）
//
// 测试步骤：
//   - Start sink → Write 一帧（18 通道）→ Stop
//
// 期待结果：
//   - CSV 文件表头为固定 18 列宽格式（CH01..CH16 + CH17_AtmPressure + CH18_AtmTemp）
//   - 不应出现 _Pa/_degC 后缀
func TestCSVRecordingSinkDAQP1604WideFormatHeaderUnchanged(t *testing.T) {
	dir := t.TempDir()
	sink := NewCSVRecordingSink()

	// 即便 server.go 注入了 DeviceChannels，DAQ-P-1604 走 isWideFormat 分支应忽略
	channels := make([]device.ChannelConfig, 18)
	for i := range channels {
		channels[i] = device.ChannelConfig{Index: i, SensorType: device.SensorPressure}
	}
	cfg := corestorage.RecordingConfig{
		OutputDir:      dir,
		FilePrefix:     "p1604",
		DeviceChannels: map[string][]device.ChannelConfig{"p1604-1": channels},
	}
	if err := sink.Start(cfg); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	// DAQ-P-1604 18 通道
	channelValues := make([]float64, 18)
	for i := range channelValues {
		channelValues[i] = float64(i + 1)
	}
	payload := device.DataPayload{
		DeviceID:   "p1604-1",
		DeviceType: device.DeviceDAQP1604,
		Timestamp:  789,
		Channels:   channelValues,
	}
	if err := sink.Write(payload); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if err := sink.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "p1604-*.csv"))
	if err != nil {
		t.Fatalf("glob recording files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one recording file, got %d", len(files))
	}
	content, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read recording file: %v", err)
	}
	text := string(content)
	expectedHeader := "Timestamp,CH01,CH02,CH03,CH04,CH05,CH06,CH07,CH08,CH09,CH10,CH11,CH12,CH13,CH14,CH15,CH16,CH17_AtmPressure,CH18_AtmTemp"
	if !strings.Contains(text, expectedHeader) {
		t.Fatalf("expected DAQ-P-1604 fixed wide format header, got %q", text)
	}
	// 不应出现 _Pa 后缀（DAQ-P-1604 走 isWideFormat 固定表头分支）
	if strings.Contains(text, "CH01_Pa") {
		t.Fatalf("DAQ-P-1604 header should not contain _Pa suffix, got %q", text)
	}
}
