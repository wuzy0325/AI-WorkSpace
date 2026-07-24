package recording

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"daq-t1603/core"
)

// makeChannels16 构造 16 通道配置，按 enabledMask 设置每通道启用状态。
// REC-006 测试辅助：mask[i]=false 表示第 i 通道禁用（CSV 该列应留空）。
func makeChannels16(enabledMask [16]bool) []core.ChannelConfig {
	chs := make([]core.ChannelConfig, 16)
	for i := 0; i < 16; i++ {
		chs[i] = core.ChannelConfig{
			Index:   i,
			Name:    "CH",
			Enabled: enabledMask[i],
			Unit:    "°C",
		}
	}
	return chs
}

// readFirstDataRowT1603 读取 CSV 文件第一条数据行（跳过表头）。
// 返回按逗号分隔的字段切片（已去除行尾换行）。
// REC-005 表头共 20 列：DeviceID, Timestamp, Millisecond, Unit, CH01..CH16
func readFirstDataRowT1603(t *testing.T, path string) []string {
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
//   - 构造 16 通道配置，CH1（索引 0）禁用，其余启用
//   - 启动 CSVRecorder，通过 SetDeviceProfile 注入通道掩码
//   - 写入一条全 1.0 的快照
//
// 测试步骤：
//   - Stop 后读取 CSV 第一条数据行
//   - 检查 CH1 列（字段索引 4，前 4 列为 DeviceID/Timestamp/Millisecond/Unit）应为空字符串
//   - 检查 CH2 列（字段索引 5）应为 "1.000"
//
// 期待结果：禁用通道列留空，启用通道列正常输出，列数仍为 20。
func TestREC006_DisabledChannelEmpty(t *testing.T) {
	var mask [16]bool
	for i := range mask {
		mask[i] = true
	}
	mask[0] = false // CH1 禁用
	channels := makeChannels16(mask)

	rec := NewCSVRecorder()
	tmpDir := t.TempDir()
	if err := rec.Start(tmpDir, "rec006"); err != nil {
		t.Fatalf("start recording: %v", err)
	}
	// REC-006：注入通道掩码，CH1 禁用应在 CSV 中留空
	rec.SetDeviceProfile("devA", channels)

	values := make([]float64, 16)
	for i := range values {
		values[i] = 1.0
	}
	if err := rec.Write(core.TemperatureSnapshot{
		DeviceID:  "devA",
		Timestamp: 1700000000000,
		Values:    values,
		Unit:      "°C",
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

	fields := readFirstDataRowT1603(t, csvPath)
	// REC-005：20 列 = DeviceID + Timestamp + Millisecond + Unit + CH01..CH16
	if got := len(fields); got != 20 {
		t.Errorf("column count = %d, want 20 (DeviceID/Timestamp/Millisecond/Unit + 16 channels)", got)
	}

	// 索引 0: DeviceID, 1: Timestamp, 2: Millisecond, 3: Unit, 4: CH01（禁用，应留空）
	if fields[4] != "" {
		t.Errorf("CH01 (disabled) = %q, want empty string", fields[4])
	}
	// 索引 5: CH02（启用，应输出 "1.000"）
	if fields[5] != "1.000" {
		t.Errorf("CH02 (enabled) = %q, want %q", fields[5], "1.000")
	}
}

// TestREC006_MultiDeviceChannelIsolation 多设备通道启用状态隔离回归测试。
//
// 测试前置：
//   - 设备 A：CH1（索引 0）禁用，其余启用
//   - 设备 B：全部通道启用
//   - 两个设备分别通过 SetDeviceProfile 注入各自的通道配置
//
// 测试步骤：
//   - 启动录制，分别对 devA/devB 调用 SetDeviceProfile
//   - 分别写入 A、B 各一条快照（全 2.0）
//   - Stop 后读取两个 CSV 文件的第一条数据行
//
// 期待结果：
//   - 设备 A 的 CSV：CH1 列留空（按 A 的配置）
//   - 设备 B 的 CSV：CH1 列有值 "2.000"（按 B 的配置，不受 A 影响）
//
// 这是 REC-006 修复的关键回归点：channelEnabled 是 per-device 的 atomic.Pointer，
// 若误用全局共享 mask，会导致设备 A 关闭 CH1 → 设备 B 的 CH1 数据也被置空。
func TestREC006_MultiDeviceChannelIsolation(t *testing.T) {
	// 设备 A：CH1 禁用
	var maskA [16]bool
	for i := range maskA {
		maskA[i] = true
	}
	maskA[0] = false
	channelsA := makeChannels16(maskA)

	// 设备 B：全部启用
	var maskB [16]bool
	for i := range maskB {
		maskB[i] = true
	}
	channelsB := makeChannels16(maskB)

	rec := NewCSVRecorder()
	tmpDir := t.TempDir()
	if err := rec.Start(tmpDir, "rec006multi"); err != nil {
		t.Fatalf("start recording: %v", err)
	}
	// 分别注入 A、B 的通道配置，验证 per-device 隔离
	rec.SetDeviceProfile("devA", channelsA)
	rec.SetDeviceProfile("devB", channelsB)

	values := make([]float64, 16)
	for i := range values {
		values[i] = 2.0
	}
	if err := rec.Write(core.TemperatureSnapshot{
		DeviceID:  "devA",
		Timestamp: 1700000000000,
		Values:    values,
		Unit:      "°C",
	}); err != nil {
		t.Fatalf("write devA snapshot: %v", err)
	}
	if err := rec.Write(core.TemperatureSnapshot{
		DeviceID:  "devB",
		Timestamp: 1700000000000,
		Values:    values,
		Unit:      "°C",
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

	// 解析 devA 第一条数据行：CH1（索引 4）应留空
	devALines := strings.Split(strings.TrimRight(devAContent, "\n"), "\n")
	if len(devALines) < 2 {
		t.Fatalf("devA csv should have header + data row")
	}
	devAFields := strings.Split(devALines[1], ",")
	if devAFields[4] != "" {
		t.Errorf("devA CH01 (disabled) = %q, want empty", devAFields[4])
	}

	// 解析 devB 第一条数据行：CH1（索引 4）应有值 "2.000"
	devBLines := strings.Split(strings.TrimRight(devBContent, "\n"), "\n")
	if len(devBLines) < 2 {
		t.Fatalf("devB csv should have header + data row")
	}
	devBFields := strings.Split(devBLines[1], ",")
	if devBFields[4] != "2.000" {
		t.Errorf("devB CH01 (enabled, isolated from devA) = %q, want %q", devBFields[4], "2.000")
	}
}

// TestREC006_ProfileAppliedBeforeStart 验证 SetDeviceProfile 在 Start 前调用时，
// 掩码被缓存到 deviceProfiles，待 newDeviceWriter 创建时自动应用。
//
// 这覆盖 backend 在 Connect 时注入掩码、之后才 Start 录制的场景：
//   - 先 SetDeviceProfile（recorder 未 Start，仅缓存）
//   - 再 Start 录制
//   - 写入快照，验证禁用通道仍留空
//
// 期待结果：缓存路径与实时注入路径行为一致，禁用通道列留空。
func TestREC006_ProfileAppliedBeforeStart(t *testing.T) {
	var mask [16]bool
	for i := range mask {
		mask[i] = true
	}
	mask[0] = false // CH1 禁用
	channels := makeChannels16(mask)

	rec := NewCSVRecorder()
	tmpDir := t.TempDir()

	// 关键顺序：先注入掩码，再 Start
	rec.SetDeviceProfile("devC", channels)
	if err := rec.Start(tmpDir, "rec006cache"); err != nil {
		t.Fatalf("start recording: %v", err)
	}

	values := make([]float64, 16)
	for i := range values {
		values[i] = 3.0
	}
	if err := rec.Write(core.TemperatureSnapshot{
		DeviceID:  "devC",
		Timestamp: 1700000000000,
		Values:    values,
		Unit:      "°C",
	}); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	if err := rec.Stop(); err != nil {
		t.Fatalf("stop recording: %v", err)
	}

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

	fields := readFirstDataRowT1603(t, csvPath)
	// CH1（索引 4）禁用应留空
	if fields[4] != "" {
		t.Errorf("CH01 (disabled, profile injected before Start) = %q, want empty", fields[4])
	}
	// CH2（索引 5）启用应有值 "3.000"
	if fields[5] != "3.000" {
		t.Errorf("CH02 (enabled) = %q, want %q", fields[5], "3.000")
	}
}

// TestREC006_StopClearsProfiles 验证 Stop 清理 deviceProfiles，
// 避免同进程内 Stop → Start 会话间掩码泄漏。
//
// 测试前置：
//   - 第一次会话：注入 CH1 禁用掩码，Start → Write → Stop
//   - 第二次会话：不注入掩码，Start → Write → Stop
//
// 测试步骤：
//   - 读取第二次会话的 CSV 第一条数据行
//   - 检查 CH1 列应有值（未禁用）
//
// 期待结果：第二次会话 CH1 不留空，证明 Stop 清理了上次的掩码。
// 若 Stop 未清理，第二次会话会沿用第一次的 CH1 禁用掩码，CH1 错误地留空。
func TestREC006_StopClearsProfiles(t *testing.T) {
	var maskDisabled [16]bool
	for i := range maskDisabled {
		maskDisabled[i] = true
	}
	maskDisabled[0] = false // CH1 禁用
	channelsDisabled := makeChannels16(maskDisabled)

	rec := NewCSVRecorder()

	// 第一次会话：注入 CH1 禁用
	dir1 := t.TempDir()
	if err := rec.Start(dir1, "session1"); err != nil {
		t.Fatalf("start session1: %v", err)
	}
	rec.SetDeviceProfile("devD", channelsDisabled)
	values := make([]float64, 16)
	for i := range values {
		values[i] = 5.0
	}
	if err := rec.Write(core.TemperatureSnapshot{
		DeviceID: "devD", Timestamp: 1, Values: values, Unit: "°C",
	}); err != nil {
		t.Fatalf("write session1: %v", err)
	}
	if err := rec.Stop(); err != nil {
		t.Fatalf("stop session1: %v", err)
	}

	// 第二次会话：不注入掩码，验证 CH1 不再被禁用
	dir2 := t.TempDir()
	if err := rec.Start(dir2, "session2"); err != nil {
		t.Fatalf("start session2: %v", err)
	}
	if err := rec.Write(core.TemperatureSnapshot{
		DeviceID: "devD", Timestamp: 2, Values: values, Unit: "°C",
	}); err != nil {
		t.Fatalf("write session2: %v", err)
	}
	if err := rec.Stop(); err != nil {
		t.Fatalf("stop session2: %v", err)
	}

	// 读取第二次会话的 CSV
	entries, err := os.ReadDir(dir2)
	if err != nil {
		t.Fatalf("read dir2: %v", err)
	}
	var csvPath string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".csv") {
			csvPath = filepath.Join(dir2, e.Name())
			break
		}
	}
	if csvPath == "" {
		t.Fatalf("no csv file in session2 dir")
	}

	fields := readFirstDataRowT1603(t, csvPath)
	// CH1（索引 4）应正常输出 "5.000"（Stop 清理了上次掩码，无注入 = 全启用）
	if fields[4] != "5.000" {
		t.Errorf("session2 CH01 = %q, want %q (Stop should clear profiles)", fields[4], "5.000")
	}
}

// TestREC006_EmptyChannelsFallback 验证 SetDeviceProfile 传入空 channels 时，
// 退化为"全通道启用"（mask 补 true），而非全禁用。
//
// 这保护"backend 意外传入空 channels"的边界：若退化为全禁用，
// 所有通道列都会留空，用户会误以为采集失败。
//
// 测试前置：调用 SetDeviceProfile 传入空切片
// 测试步骤：写入快照，读取 CSV
// 期待结果：所有通道列都有值（全启用回退）
func TestREC006_EmptyChannelsFallback(t *testing.T) {
	rec := NewCSVRecorder()
	tmpDir := t.TempDir()
	if err := rec.Start(tmpDir, "empty"); err != nil {
		t.Fatalf("start: %v", err)
	}
	// 传入空 channels，应退化为全启用
	rec.SetDeviceProfile("devE", []core.ChannelConfig{})

	values := make([]float64, 16)
	for i := range values {
		values[i] = 7.0
	}
	if err := rec.Write(core.TemperatureSnapshot{
		DeviceID: "devE", Timestamp: 1, Values: values, Unit: "°C",
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := rec.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}

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
		t.Fatalf("no csv file")
	}

	fields := readFirstDataRowT1603(t, csvPath)
	// 所有 16 通道（索引 4-19）都应有值 "7.000"
	for i := 4; i < 20; i++ {
		if fields[i] != "7.000" {
			t.Errorf("CH%02d (empty channels fallback) = %q, want %q", i-3, fields[i], "7.000")
		}
	}
}
