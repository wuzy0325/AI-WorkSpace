package recording

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"daq-p1604/core"
)

// makeChannels 构造 18 通道配置，按 enabledMask 关闭指定通道。
// enabledMask[i]=false 表示第 i 通道禁用（CSV 该列应留空）。
func makeChannels(enabledMask [numChannels]bool) []core.ChannelConfig {
	chs := make([]core.ChannelConfig, numChannels)
	for i := 0; i < numChannels; i++ {
		chs[i] = core.ChannelConfig{
			Index:     i,
			Name:      "CH",
			Enabled:   enabledMask[i],
			Precision: 4,
		}
	}
	return chs
}

// readFirstDataRow 读取 CSV 文件第一条数据行（跳过表头）。
// 返回按逗号分隔的字段切片（已去除行尾换行）。
func readFirstDataRow(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read csv %s: %v", path, err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("csv should have header + at least 1 data row, got %d lines", len(lines))
	}
	// 第一条数据行（跳过表头）
	return strings.Split(lines[1], ",")
}

// TestREC006_DisabledChannelEmpty 单设备禁用通道占位回归测试。
//
// 测试前置：
//   - 构造 18 通道配置，CH1（索引 0）禁用，其余启用
//   - 启动 CSVRecorder，DeviceChannels 字段填充该配置
//   - 写入一条全 1.0 的快照
//
// 测试步骤：
//   - Stop 后读取 CSV 第一条数据行
//   - 检查 CH1 列（索引 1，时间戳占索引 0）应为空字符串
//   - 检查 CH2 列（索引 2）应为 "1.0000"
//
// 期待结果：禁用通道列留空，启用通道列正常输出，列数仍为 19（时间戳 + 18 通道）。
func TestREC006_DisabledChannelEmpty(t *testing.T) {
	var mask [numChannels]bool
	for i := range mask {
		mask[i] = true
	}
	mask[0] = false // CH1 禁用
	channels := makeChannels(mask)

	rec := NewCSVRecorder()
	tmpDir := t.TempDir()
	if err := rec.Start(core.RecordingConfig{
		OutputDir:       tmpDir,
		FilePrefix:      "rec006",
		Channels:        channels,
		DeviceChannels:  map[string][]core.ChannelConfig{"devA": channels},
		FlushIntervalMs: 1,
		SyncIntervalSec: 1,
	}); err != nil {
		t.Fatalf("start recording: %v", err)
	}

	values := make([]float64, numChannels)
	for i := range values {
		values[i] = 1.0
	}
	if err := rec.Write(core.PressureSnapshot{
		DeviceID:  "devA",
		Timestamp: 1700000000000,
		Values:    values,
	}); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	if err := rec.Stop(); err != nil {
		t.Fatalf("stop recording: %v", err)
	}

	// 找到生成的 CSV 文件
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var csvPath string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".csv") {
			csvPath = filepath.Join(tmpDir, e.Name())
			break
		}
	}
	if csvPath == "" {
		t.Fatalf("no csv file generated in %s", tmpDir)
	}

	fields := readFirstDataRow(t, csvPath)
	if got := len(fields); got != numChannels+1 {
		t.Errorf("column count = %d, want %d (timestamp + 18 channels)", got, numChannels+1)
	}

	// 索引 0 是时间戳（带单引号前缀），索引 1 是 CH1（应留空）
	if fields[1] != "" {
		t.Errorf("CH1 (disabled) = %q, want empty string", fields[1])
	}
	// 索引 2 是 CH2（应正常输出）
	if fields[2] != "1.0000" {
		t.Errorf("CH2 (enabled) = %q, want %q", fields[2], "1.0000")
	}
}

// TestREC006_MultiDeviceChannelIsolation 多设备通道启用状态隔离回归测试。
//
// 测试前置：
//   - 设备 A：CH1（索引 0）禁用，其余启用
//   - 设备 B：全部通道启用
//   - 两个设备共享同一份 Channels（CH1 禁用），但 DeviceChannels 分别独立配置
//
// 测试步骤：
//   - 启动录制，DeviceChannels 填充 A/B 各自的通道配置
//   - 分别写入 A、B 各一条快照（全 2.0）
//   - Stop 后读取两个 CSV 文件的第一条数据行
//
// 期待结果：
//   - 设备 A 的 CSV：CH1 列留空（按 A 的配置）
//   - 设备 B 的 CSV：CH1 列有值 "2.0000"（按 B 的配置，不受 A 影响）
//
// 这是 REC-006 修复的关键回归点：初版直接读共享 Channels[i].Enabled，
// 会导致设备 A 关闭 CH1 → 设备 B 的 CH1 数据也被置空。
func TestREC006_MultiDeviceChannelIsolation(t *testing.T) {
	// 设备 A：CH1 禁用
	var maskA [numChannels]bool
	for i := range maskA {
		maskA[i] = true
	}
	maskA[0] = false
	channelsA := makeChannels(maskA)

	// 设备 B：全部启用
	var maskB [numChannels]bool
	for i := range maskB {
		maskB[i] = true
	}
	channelsB := makeChannels(maskB)

	// 共享 Channels 故意设为 A 的配置（CH1 禁用），
	// 验证 recorder 优先用 DeviceChannels 而非共享 Channels
	rec := NewCSVRecorder()
	tmpDir := t.TempDir()
	if err := rec.Start(core.RecordingConfig{
		OutputDir:      tmpDir,
		FilePrefix:     "rec006multi",
		Channels:       channelsA,
		DeviceChannels: map[string][]core.ChannelConfig{"devA": channelsA, "devB": channelsB},
		FlushIntervalMs: 1,
		SyncIntervalSec: 1,
	}); err != nil {
		t.Fatalf("start recording: %v", err)
	}

	values := make([]float64, numChannels)
	for i := range values {
		values[i] = 2.0
	}
	if err := rec.Write(core.PressureSnapshot{
		DeviceID:  "devA",
		Timestamp: 1700000000000,
		Values:    values,
	}); err != nil {
		t.Fatalf("write devA snapshot: %v", err)
	}
	if err := rec.Write(core.PressureSnapshot{
		DeviceID:  "devB",
		Timestamp: 1700000000000,
		Values:    values,
	}); err != nil {
		t.Fatalf("write devB snapshot: %v", err)
	}

	if err := rec.Stop(); err != nil {
		t.Fatalf("stop recording: %v", err)
	}

	// 读取两个 CSV 文件
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	csvByDevice := map[string]string{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".csv") {
			continue
		}
		path := filepath.Join(tmpDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		csvByDevice[e.Name()] = string(data)
	}
	if len(csvByDevice) != 2 {
		t.Fatalf("expected 2 csv files, got %d", len(csvByDevice))
	}

	// 定位 devA 和 devB 的文件内容
	var devAContent, devBContent string
	for name, content := range csvByDevice {
		if strings.Contains(name, "devA") {
			devAContent = content
		} else if strings.Contains(name, "devB") {
			devBContent = content
		}
	}
	if devAContent == "" || devBContent == "" {
		t.Fatalf("missing devA or devB csv file, got: %v", csvByDevice)
	}

	// 解析 devA 第一条数据行
	devALines := strings.Split(strings.TrimRight(devAContent, "\n"), "\n")
	if len(devALines) < 2 {
		t.Fatalf("devA csv should have header + data row")
	}
	devAFields := strings.Split(devALines[1], ",")
	if devAFields[1] != "" {
		t.Errorf("devA CH1 (disabled) = %q, want empty", devAFields[1])
	}

	// 解析 devB 第一条数据行
	devBLines := strings.Split(strings.TrimRight(devBContent, "\n"), "\n")
	if len(devBLines) < 2 {
		t.Fatalf("devB csv should have header + data row")
	}
	devBFields := strings.Split(devBLines[1], ",")
	if devBFields[1] != "2.0000" {
		t.Errorf("devB CH1 (enabled, isolated from devA) = %q, want %q", devBFields[1], "2.0000")
	}
}
