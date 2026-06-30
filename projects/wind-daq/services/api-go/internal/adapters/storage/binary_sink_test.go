// binary_sink_test.go 二进制存储 sink 的单元测试。
//
// 覆盖：
//   - Start/Write/Stop 生命周期（与 csv_sink_test 对齐）
//   - 二进制文件格式 round-trip：写入 payload → 读取文件 → 校验 magic/version/帧头/通道值
//   - 多设备多通道场景
//   - 未 Start 时 Write 应返回错误
//   - DeviceTimestamp 字段优先于 Timestamp
package storage

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wind-daq/services/api-go/internal/core/device"
	corestorage "wind-daq/services/api-go/internal/core/storage"
)

// readBinaryFile 读取二进制 sink 产生的文件并返回 magic、version、所有帧。
// 用于在测试中验证写入的数据是否可被正确读回（round-trip）。
type binaryFrame struct {
	Timestamp     int64
	DeviceTs      int64
	DeviceID      string
	ChannelIdx    []int32
	ChannelValues []float32
}

func readBinaryFile(t *testing.T, path string) (magic string, version uint16, frames []binaryFrame) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read binary file: %v", err)
	}
	if len(data) < binaryHeaderSize {
		t.Fatalf("file too small: %d bytes (need at least %d for header)", len(data), binaryHeaderSize)
	}
	magic = string(data[0:4])
	version = binary.LittleEndian.Uint16(data[4:6])
	// data[6:16] 是保留字段，跳过

	offset := binaryHeaderSize
	for offset < len(data) {
		if offset+binaryFrameHeader > len(data) {
			t.Fatalf("incomplete frame header at offset %d", offset)
		}
		ts := int64(binary.LittleEndian.Uint64(data[offset : offset+8]))
		deviceTs := int64(binary.LittleEndian.Uint64(data[offset+8 : offset+16]))
		deviceIDLen := int(binary.LittleEndian.Uint16(data[offset+16 : offset+18]))
		channelCount := int(binary.LittleEndian.Uint16(data[offset+18 : offset+20]))
		offset += binaryFrameHeader

		if offset+deviceIDLen > len(data) {
			t.Fatalf("incomplete deviceId at offset %d", offset)
		}
		deviceID := string(data[offset : offset+deviceIDLen])
		offset += deviceIDLen

		if offset+channelCount*4 > len(data) {
			t.Fatalf("incomplete channelIndices at offset %d", offset)
		}
		idx := make([]int32, channelCount)
		for i := 0; i < channelCount; i++ {
			idx[i] = int32(binary.LittleEndian.Uint32(data[offset+i*4 : offset+i*4+4]))
		}
		offset += channelCount * 4

		if offset+channelCount*4 > len(data) {
			t.Fatalf("incomplete channelValues at offset %d", offset)
		}
		vals := make([]float32, channelCount)
		for i := 0; i < channelCount; i++ {
			bits := binary.LittleEndian.Uint32(data[offset+i*4 : offset+i*4+4])
			vals[i] = math.Float32frombits(bits)
		}
		offset += channelCount * 4

		frames = append(frames, binaryFrame{
			Timestamp:     ts,
			DeviceTs:      deviceTs,
			DeviceID:      deviceID,
			ChannelIdx:    idx,
			ChannelValues: vals,
		})
	}
	return magic, version, frames
}

// TestBinaryRecordingSinkWritesPayloadsToFile 验证文件格式 round-trip：
// Start → Write 多帧 → Stop，读取文件后校验 magic、version、帧数、字段值。
func TestBinaryRecordingSinkWritesPayloadsToFile(t *testing.T) {
	dir := t.TempDir()
	sink := NewBinaryRecordingSink()

	if err := sink.Start(corestorage.RecordingConfig{OutputDir: dir, FilePrefix: "run"}); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	// 写入两台设备各两帧，覆盖多设备 + 多通道场景
	payloads := []device.DataPayload{
		{DeviceID: "sim-1", Timestamp: 100, Channels: []float64{1.5, -2.5, 3.14}, ChannelIndices: []int{0, 1, 2}},
		{DeviceID: "sim-2", Timestamp: 101, Channels: []float64{10.0, 20.0}, ChannelIndices: []int{5, 6}},
		{DeviceID: "sim-1", Timestamp: 102, Channels: []float64{4.5, -5.5, 6.6}, ChannelIndices: []int{0, 1, 2}},
	}
	for _, p := range payloads {
		if err := sink.Write(p); err != nil {
			t.Fatalf("Write returned error: %v", err)
		}
	}
	if err := sink.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "run-*.bin"))
	if err != nil {
		t.Fatalf("glob recording files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one recording file, got %d", len(files))
	}

	magic, version, frames := readBinaryFile(t, files[0])
	if magic != binaryMagic {
		t.Fatalf("expected magic %q, got %q", binaryMagic, magic)
	}
	if version != binaryVersion {
		t.Fatalf("expected version %d, got %d", binaryVersion, version)
	}
	if len(frames) != len(payloads) {
		t.Fatalf("expected %d frames, got %d", len(payloads), len(frames))
	}

	// 逐帧校验字段
	for i, f := range frames {
		expected := payloads[i]
		if f.Timestamp != expected.Timestamp {
			t.Fatalf("frame %d: expected timestamp %d, got %d", i, expected.Timestamp, f.Timestamp)
		}
		if f.DeviceID != expected.DeviceID {
			t.Fatalf("frame %d: expected deviceId %q, got %q", i, expected.DeviceID, f.DeviceID)
		}
		if len(f.ChannelIdx) != len(expected.ChannelIndices) {
			t.Fatalf("frame %d: expected %d channel indices, got %d", i, len(expected.ChannelIndices), len(f.ChannelIdx))
		}
		for j, idx := range f.ChannelIdx {
			if int(idx) != expected.ChannelIndices[j] {
				t.Fatalf("frame %d channel %d: expected idx %d, got %d", i, j, expected.ChannelIndices[j], idx)
			}
		}
		if len(f.ChannelValues) != len(expected.Channels) {
			t.Fatalf("frame %d: expected %d channel values, got %d", i, len(expected.Channels), len(f.ChannelValues))
		}
		for j, v := range f.ChannelValues {
			// float32 精度损失可接受 1e-5 误差
			got := float64(v)
			want := float32(expected.Channels[j])
			if math.Abs(float64(want)-got) > 1e-5 {
				t.Fatalf("frame %d channel %d: expected %f, got %f", i, j, expected.Channels[j], v)
			}
		}
	}
}

// TestBinaryRecordingSinkPrefersDeviceTimestamp 验证 DeviceTimestamp > 0 时优先使用设备时间戳。
func TestBinaryRecordingSinkPrefersDeviceTimestamp(t *testing.T) {
	dir := t.TempDir()
	sink := NewBinaryRecordingSink()
	if err := sink.Start(corestorage.RecordingConfig{OutputDir: dir, FilePrefix: "run"}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// DeviceTimestamp > 0，应优先写入 deviceTimestamp
	if err := sink.Write(device.DataPayload{
		DeviceID:        "dev-1",
		Timestamp:       100,
		DeviceTimestamp: 9999,
		Channels:        []float64{1.0},
		ChannelIndices:  []int{0},
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := sink.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	files, _ := filepath.Glob(filepath.Join(dir, "run-*.bin"))
	_, _, frames := readBinaryFile(t, files[0])
	if len(frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(frames))
	}
	if frames[0].Timestamp != 9999 {
		t.Fatalf("expected frame timestamp 9999 (deviceTimestamp), got %d", frames[0].Timestamp)
	}
}

// TestBinaryRecordingSinkHandlesEmptyChannelIndices 验证 ChannelIndices 为空时使用 0..N-1 兜底。
func TestBinaryRecordingSinkHandlesEmptyChannelIndices(t *testing.T) {
	dir := t.TempDir()
	sink := NewBinaryRecordingSink()
	if err := sink.Start(corestorage.RecordingConfig{OutputDir: dir, FilePrefix: "run"}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := sink.Write(device.DataPayload{
		DeviceID:  "dev-1",
		Timestamp: 1,
		Channels:  []float64{1.0, 2.0, 3.0},
		// ChannelIndices 留空，writePayload 应填充 [0, 1, 2]
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := sink.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	files, _ := filepath.Glob(filepath.Join(dir, "run-*.bin"))
	_, _, frames := readBinaryFile(t, files[0])
	if len(frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(frames))
	}
	if len(frames[0].ChannelIdx) != 3 {
		t.Fatalf("expected 3 channel indices, got %d", len(frames[0].ChannelIdx))
	}
	for i, idx := range frames[0].ChannelIdx {
		if int(idx) != i {
			t.Fatalf("expected channel index %d, got %d", i, idx)
		}
	}
}

// TestBinaryRecordingSinkReturnsErrorWhenNotStarted 验证未 Start 时 Write 返回错误。
func TestBinaryRecordingSinkReturnsErrorWhenNotStarted(t *testing.T) {
	sink := NewBinaryRecordingSink()
	if err := sink.Write(device.DataPayload{DeviceID: "sim-1"}); err == nil {
		t.Fatal("expected write before start to fail")
	}
}

// TestBinaryRecordingSinkRejectsMissingOutputDir 验证 OutputDir/FilePrefix 为空时 Start 返回错误。
func TestBinaryRecordingSinkRejectsMissingOutputDir(t *testing.T) {
	sink := NewBinaryRecordingSink()
	if err := sink.Start(corestorage.RecordingConfig{FilePrefix: "run"}); err == nil {
		t.Fatal("expected Start with empty OutputDir to fail")
	}
	sink2 := NewBinaryRecordingSink()
	if err := sink2.Start(corestorage.RecordingConfig{OutputDir: t.TempDir()}); err == nil {
		t.Fatal("expected Start with empty FilePrefix to fail")
	}
}

// TestBinaryRecordingSinkStopIsIdempotent 验证重复 Stop 是幂等的。
func TestBinaryRecordingSinkStopIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	sink := NewBinaryRecordingSink()
	if err := sink.Start(corestorage.RecordingConfig{OutputDir: dir, FilePrefix: "run"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := sink.Stop(); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	// 第二次 Stop 应返回 nil（CAS 已 false，直接返回）
	if err := sink.Stop(); err != nil {
		t.Fatalf("second Stop should be idempotent, got: %v", err)
	}
}

// TestBinaryRecordingSinkWriteBeforeStartFailsWithReadableError 验证错误信息可读。
func TestBinaryRecordingSinkWriteBeforeStartFailsWithReadableError(t *testing.T) {
	sink := NewBinaryRecordingSink()
	err := sink.Write(device.DataPayload{DeviceID: "sim-1"})
	if err == nil {
		t.Fatal("expected error before Start")
	}
	if !strings.Contains(err.Error(), "not started") {
		t.Fatalf("expected error to mention 'not started', got: %v", err)
	}
}
