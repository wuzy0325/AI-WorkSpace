package usecase

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"windlabx4/services/api-go/internal/core/device"
	"windlabx4/services/api-go/internal/core/traversal"
)

// ===== 多设备通道绑定：ParseConfig 测试 =====

// multiDeviceConfigJSON 构造五孔在 devA（ch0-4）、大气压/温度在 devB（ch0-1）的配置。
// devB 的通道序号与 devA 重复（各设备独立编号），验证跨设备序号冲突的别名分配。
const multiDeviceConfigJSON = `{
	"name": "multi-device-test",
	"layout": {
		"pattern": "line",
		"line": { "startX": 0, "endX": 1, "xStepSegments": [{"start":0,"end":1,"step":1}] }
	},
	"channels": {
		"probeChannels": [
			{"name":"P1","role":"fiveHole.p1","channel":{"deviceId":"devA","channelIndex":0},"enabled":true},
			{"name":"P2","role":"fiveHole.p2","channel":{"deviceId":"devA","channelIndex":1},"enabled":true},
			{"name":"P3","role":"fiveHole.p3","channel":{"deviceId":"devA","channelIndex":2},"enabled":true},
			{"name":"P4","role":"fiveHole.p4","channel":{"deviceId":"devA","channelIndex":3},"enabled":true},
			{"name":"P5","role":"fiveHole.p5","channel":{"deviceId":"devA","channelIndex":4},"enabled":true},
			{"name":"Patm","role":"fiveHole.pAtm","channel":{"deviceId":"devB","channelIndex":0},"enabled":true},
			{"name":"Tatm","role":"fiveHole.tAtm","channel":{"deviceId":"devB","channelIndex":1},"enabled":true}
		]
	},
	"dwellTimeMs": 100,
	"samplesPerPoint": 1
}`

// TestParseConfig_MultiDeviceDuplicateIndices 跨设备序号冲突：内部键唯一且 ChannelRefs 还原真实物理通道。
func TestParseConfig_MultiDeviceDuplicateIndices(t *testing.T) {
	mgr := newConfigTestManager(t)
	config, err := mgr.ParseConfig(json.RawMessage(multiDeviceConfigJSON))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	if config.DeviceID != "devA" {
		t.Errorf("DeviceID = %q, want devA（首探针设备）", config.DeviceID)
	}

	// 7 个通道键全局唯一：devA 沿用硬件索引 0-4，devB 冲突序号分配空闲键 5、6
	if len(config.Channels) != 7 {
		t.Fatalf("channels = %v, want 7 entries", config.Channels)
	}
	wantRefs := map[int]traversal.ChannelRef{
		0: {DeviceID: "devA", Index: 0},
		1: {DeviceID: "devA", Index: 1},
		2: {DeviceID: "devA", Index: 2},
		3: {DeviceID: "devA", Index: 3},
		4: {DeviceID: "devA", Index: 4},
		5: {DeviceID: "devB", Index: 0},
		6: {DeviceID: "devB", Index: 1},
	}
	for key, want := range wantRefs {
		got, ok := config.ChannelRefs[key]
		if !ok {
			t.Fatalf("ChannelRefs missing key %d (refs=%v)", key, config.ChannelRefs)
		}
		if got != want {
			t.Errorf("ChannelRefs[%d] = %+v, want %+v", key, got, want)
		}
	}

	// 标签挂在内部键上：Patm/Tatm 必须在别名键 5、6 而非硬件序号 0、1
	if config.ChannelLabels[5] != "Patm" || config.ChannelLabels[6] != "Tatm" {
		t.Errorf("labels on aliased keys wrong: %v", config.ChannelLabels)
	}
	if _, clash := config.ChannelLabels[0]; config.ChannelLabels[0] != "P1" || !clash {
		t.Errorf("label for key 0 = %q, want P1", config.ChannelLabels[0])
	}
}

// TestParseConfig_RejectsDuplicatePhysicalBinding 同一设备同一通道被两个探针绑定 → 明确报错。
func TestParseConfig_RejectsDuplicatePhysicalBinding(t *testing.T) {
	mgr := newConfigTestManager(t)
	raw := json.RawMessage(`{
		"name": "dup-physical",
		"layout": {
			"pattern": "line",
			"line": { "startX": 0, "endX": 1, "xStepSegments": [{"start":0,"end":1,"step":1}] }
		},
		"channels": {
			"probeChannels": [
				{"name":"P1","role":"fiveHole.p1","channel":{"deviceId":"devA","channelIndex":0},"enabled":true},
				{"name":"Patm","role":"fiveHole.pAtm","channel":{"deviceId":"devA","channelIndex":0},"enabled":true}
			]
		},
		"dwellTimeMs": 100,
		"samplesPerPoint": 1
	}`)

	_, err := mgr.ParseConfig(raw)
	if err == nil || !strings.Contains(err.Error(), "duplicate channel 0 on device devA") {
		t.Fatalf("expected duplicate physical binding error, got %v", err)
	}
}

// TestParseConfig_SingleDeviceKeepsHardwareIndices 单设备无冲突：内部键 == 硬件索引（历史行为）。
func TestParseConfig_SingleDeviceKeepsHardwareIndices(t *testing.T) {
	mgr := newConfigTestManager(t)
	raw := json.RawMessage(`{
		"name": "single-device",
		"layout": {
			"pattern": "line",
			"line": { "startX": 0, "endX": 1, "xStepSegments": [{"start":0,"end":1,"step":1}] }
		},
		"channels": {
			"probeChannels": [
				{"name":"P1","role":"fiveHole.p1","channel":{"deviceId":"devA","channelIndex":0},"enabled":true},
				{"name":"Patm","role":"fiveHole.pAtm","channel":{"deviceId":"devA","channelIndex":16},"enabled":true},
				{"name":"Tatm","role":"fiveHole.tAtm","channel":{"deviceId":"devA","channelIndex":17},"enabled":true}
			]
		},
		"dwellTimeMs": 100,
		"samplesPerPoint": 1
	}`)

	config, err := mgr.ParseConfig(raw)
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}
	for _, ch := range []int{0, 16, 17} {
		ref, ok := config.ChannelRefs[ch]
		if !ok || ref.DeviceID != "devA" || ref.Index != ch {
			t.Errorf("ChannelRefs[%d] = %+v (ok=%v), want {devA %d}", ch, ref, ok, ch)
		}
	}
	if config.ChannelLabels[16] != "Patm" || config.ChannelLabels[17] != "Tatm" {
		t.Errorf("labels wrong: %v", config.ChannelLabels)
	}
}

// TestResolvedChannelRefs_LegacyFallback 旧配置无 ChannelRefs：回退合成单设备语义。
func TestResolvedChannelRefs_LegacyFallback(t *testing.T) {
	cfg := traversal.Config{DeviceID: "dev-legacy", Channels: []int{0, 5, 16}}
	refs := cfg.ResolvedChannelRefs()
	for _, ch := range []int{0, 5, 16} {
		ref, ok := refs[ch]
		if !ok || ref.DeviceID != "dev-legacy" || ref.Index != ch {
			t.Errorf("refs[%d] = %+v (ok=%v), want {dev-legacy %d}", ch, ref, ok, ch)
		}
	}
}

// ===== 多设备采样：collectAveragedSamples 测试 =====

// multiDeviceReader 按设备脚本化返回数据帧：每次 GetLatestData 消费该设备序列中的
// 下一帧；帧用完后重复返回最后一帧（模拟设备刷新周期低于轮询时的同帧重复）。
// seq 中不存在的设备返回 ok=false（模拟设备未采集/未接入）。
type multiDeviceReader struct {
	mu      sync.Mutex
	seq     map[string][]device.DataPayload
	current map[string]device.DataPayload
	calls   map[string]int
}

func newMultiDeviceReader(seq map[string][]device.DataPayload) *multiDeviceReader {
	return &multiDeviceReader{
		seq:     seq,
		current: make(map[string]device.DataPayload),
		calls:   make(map[string]int),
	}
}

func (r *multiDeviceReader) GetLatestData(deviceID string) (device.DataPayload, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls[deviceID]++
	frames, ok := r.seq[deviceID]
	if !ok {
		return device.DataPayload{}, false
	}
	if len(frames) > 0 {
		r.current[deviceID] = frames[0]
		r.seq[deviceID] = frames[1:]
	}
	cur, ok := r.current[deviceID]
	return cur, ok
}

func (r *multiDeviceReader) GetLatestTimestamp(_ string) (int64, bool) { return 0, false }

func frame(dev string, ts int64, values map[int]float64) device.DataPayload {
	channels := make([]float64, 0, len(values))
	indices := make([]int, 0, len(values))
	// 固定顺序：按键升序，保证脚本帧可复现
	for i := 0; i < 64; i++ {
		if v, ok := values[i]; ok {
			channels = append(channels, v)
			indices = append(indices, i)
		}
	}
	return device.DataPayload{DeviceID: dev, Timestamp: ts, Channels: channels, ChannelIndices: indices}
}

// TestCollectAveragedSamples_MultiDeviceMergesFreshFrames 两台设备各自出新帧，
// 合并为同一有效样本并按内部键取平均。
func TestCollectAveragedSamples_MultiDeviceMergesFreshFrames(t *testing.T) {
	reader := newMultiDeviceReader(map[string][]device.DataPayload{
		"devA": {
			frame("devA", 100, map[int]float64{0: 1.0}),
			frame("devA", 200, map[int]float64{0: 2.0}),
			frame("devA", 300, map[int]float64{0: 3.0}),
		},
		"devB": {
			frame("devB", 100, map[int]float64{0: 10.0}),
			frame("devB", 200, map[int]float64{0: 20.0}),
			frame("devB", 300, map[int]float64{0: 30.0}),
		},
	})
	manager := NewTraversalManager(reader, nil, nil, nil, nil)
	manager.status = traversal.Status{TaskID: "trav-multi"}

	groups := []deviceChannelGroup{
		{deviceID: "devA", keys: []int{0}, hwIndices: []int{0}},
		{deviceID: "devB", keys: []int{5}, hwIndices: []int{0}}, // devB ch0 别名到内部键 5
	}
	values, err := manager.collectAveragedSamples("trav-multi", groups, 3)
	if err != nil {
		t.Fatalf("collectAveragedSamples returned error: %v", err)
	}
	if got := values[0]; got != 2.0 {
		t.Errorf("devA averaged key 0 = %v, want 2.0", got)
	}
	if got := values[5]; got != 20.0 {
		t.Errorf("devB averaged key 5 = %v, want 20.0", got)
	}
}

// TestCollectAveragedSamples_SkipsStaleFramesPerDevice 慢设备同帧重复不计入样本：
// devB 前几次轮询返回同一时间戳的旧帧，只有新时间戳的帧才推进样本计数。
func TestCollectAveragedSamples_SkipsStaleFramesPerDevice(t *testing.T) {
	reader := newMultiDeviceReader(map[string][]device.DataPayload{
		"devA": {
			frame("devA", 100, map[int]float64{0: 1.0}),
			frame("devA", 200, map[int]float64{0: 3.0}),
			frame("devA", 300, map[int]float64{0: 99.0}), // 不应被消费（只需 2 个样本）
		},
		"devB": {
			frame("devB", 100, map[int]float64{0: 10.0}),
			frame("devB", 100, map[int]float64{0: 10.0}), // 同帧重复，跳过
			frame("devB", 100, map[int]float64{0: 10.0}), // 同帧重复，跳过
			frame("devB", 200, map[int]float64{0: 30.0}),
		},
	})
	manager := NewTraversalManager(reader, nil, nil, nil, nil)
	manager.status = traversal.Status{TaskID: "trav-stale"}

	groups := []deviceChannelGroup{
		{deviceID: "devA", keys: []int{0}, hwIndices: []int{0}},
		{deviceID: "devB", keys: []int{5}, hwIndices: []int{0}},
	}
	values, err := manager.collectAveragedSamples("trav-stale", groups, 2)
	if err != nil {
		t.Fatalf("collectAveragedSamples returned error: %v", err)
	}
	// devA 恰好消费 2 帧（等新设备期间不重复消费快设备帧）
	if got := values[0]; got != 2.0 {
		t.Errorf("devA averaged = %v, want 2.0（(1+3)/2，第三帧 99 不应计入）", got)
	}
	if got := values[5]; got != 20.0 {
		t.Errorf("devB averaged = %v, want 20.0（同帧 10 不重复计）", got)
	}
	if reader.calls["devA"] != 2 {
		t.Errorf("devA GetLatestData calls = %d, want 2（等待 devB 期间快设备不应被重复轮询）", reader.calls["devA"])
	}
}

// TestCollectAveragedSamples_OneDeviceSilentTimesOut 一台设备始终无数据：
// 有效样本为 0，超时报 fresh samples 错误（即用户现场的失败形态）。
func TestCollectAveragedSamples_OneDeviceSilentTimesOut(t *testing.T) {
	reader := newMultiDeviceReader(map[string][]device.DataPayload{
		"devA": {
			frame("devA", 100, map[int]float64{0: 1.0}),
			frame("devA", 200, map[int]float64{0: 2.0}),
		},
		// devB 不在 seq 中 → GetLatestData 恒 ok=false
	})
	manager := NewTraversalManager(reader, nil, nil, nil, nil)
	manager.status = traversal.Status{TaskID: "trav-silent"}

	groups := []deviceChannelGroup{
		{deviceID: "devA", keys: []int{0}, hwIndices: []int{0}},
		{deviceID: "devB", keys: []int{5}, hwIndices: []int{0}},
	}
	_, err := manager.collectAveragedSamples("trav-silent", groups, 2)
	if err == nil || !strings.Contains(err.Error(), "collected 0/2 fresh samples") {
		t.Fatalf("expected fresh samples stall timeout error, got %v", err)
	}
}

// TestGroupChannelsByDevice 分组语义：设备按通道首次出现排序，键与硬件索引一一对应；
// 缺失 ref 的键被跳过（由采样长度校验兜底暴露）。
func TestGroupChannelsByDevice(t *testing.T) {
	refs := map[int]traversal.ChannelRef{
		0: {DeviceID: "devA", Index: 0},
		1: {DeviceID: "devA", Index: 1},
		5: {DeviceID: "devB", Index: 0},
		6: {DeviceID: "devB", Index: 1},
		7: {DeviceID: "devA", Index: 2},
	}
	groups := groupChannelsByDevice([]int{0, 1, 5, 6, 7, 99}, refs)

	if len(groups) != 2 {
		t.Fatalf("groups = %v, want 2 devices", groups)
	}
	if groups[0].deviceID != "devA" || groups[1].deviceID != "devB" {
		t.Fatalf("device order = %q, %q, want devA, devB（首次出现序）", groups[0].deviceID, groups[1].deviceID)
	}
	if len(groups[0].keys) != 3 || len(groups[1].keys) != 2 {
		t.Fatalf("group sizes = %d/%d, want 3/2", len(groups[0].keys), len(groups[1].keys))
	}
	// devA: keys [0 1 7] ↔ hw [0 1 2]；devB: keys [5 6] ↔ hw [0 1]
	for i, want := range []int{0, 1, 7} {
		if groups[0].keys[i] != want || groups[0].hwIndices[i] != i {
			t.Errorf("devA binding[%d] = key %d hw %d, want key %d hw %d",
				i, groups[0].keys[i], groups[0].hwIndices[i], want, i)
		}
	}
	for i, want := range []int{5, 6} {
		if groups[1].keys[i] != want || groups[1].hwIndices[i] != i {
			t.Errorf("devB binding[%d] = key %d hw %d, want key %d hw %d",
				i, groups[1].keys[i], groups[1].hwIndices[i], want, i)
		}
	}
}
