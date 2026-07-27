package usecase

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	seveninterp "ai-workspace/shared/algorithms/go/sevenhole/interpolation"
	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
)

// stubAppConfigStore 是最小 AppConfigStore 实现（供 loadPersistedConfig 测试）。
type stubAppConfigStore struct {
	data map[string][]byte
}

func (s *stubAppConfigStore) LoadConfig(key string) ([]byte, error) {
	if s.data == nil {
		return nil, nil
	}
	return s.data[key], nil
}
func (s *stubAppConfigStore) SaveConfig(key string, data []byte) error {
	if s.data == nil {
		s.data = make(map[string][]byte)
	}
	s.data[key] = data
	return nil
}

// TestLoadPersistedConfigSyncsProbeType 重启恢复路径：probeType 必须从持久化
// 配置同步到 m.config（否则探针感知前置检查按空值五孔判定，误报未加载）。
func TestLoadPersistedConfigSyncsProbeType(t *testing.T) {
	store := &stubAppConfigStore{data: map[string][]byte{
		traversalConfigKey: []byte(`{"name":"t","probeType":"seven-hole"}`),
	}}
	mgr := NewTraversalManager(nil, nil, nil, nil, nil, store)
	if mgr.config.ProbeType != traversal.ProbeTypeSevenHole {
		t.Errorf("config.ProbeType = %q, want seven-hole after persisted load", mgr.config.ProbeType)
	}
	if !mgr.config.IsSevenHole() {
		t.Error("IsSevenHole must be true after persisted load")
	}

	// 五孔持久化配置同样同步
	store2 := &stubAppConfigStore{data: map[string][]byte{
		traversalConfigKey: []byte(`{"name":"t","probeType":"five-hole"}`),
	}}
	mgr2 := NewTraversalManager(nil, nil, nil, nil, nil, store2)
	if mgr2.config.ProbeType != traversal.ProbeTypeFiveHole {
		t.Errorf("config.ProbeType = %q, want five-hole", mgr2.config.ProbeType)
	}

	// 旧配置（无 probeType）不改变零值
	store3 := &stubAppConfigStore{data: map[string][]byte{
		traversalConfigKey: []byte(`{"name":"t"}`),
	}}
	mgr3 := NewTraversalManager(nil, nil, nil, nil, nil, store3)
	if mgr3.config.ProbeType != "" {
		t.Errorf("legacy config without probeType must keep zero value, got %q", mgr3.config.ProbeType)
	}
}

// recordingSevenHoleLoader 实现 ports.InterpolatorLoader：所有方法记录调用次数并可注入错误。
//
// 双变体恢复语义下，五孔与七孔方法可能同时被触达——loader 不再"拒绝另一类型调用"，
// 改为记录所有调用次数让测试自行断言期望路径。七孔专用测试可通过断言
// prbCalls/csvFiveCalls/multiPrbCalls == 0 表达"该场景不应触达五孔 loader"。
type recordingSevenHoleLoader struct {
	calls         int
	csvCalls      int
	prbCalls      int
	csvFiveCalls  int
	multiPrbCalls int
	innerPath     string
	outerPaths    [6]string
	prbPath       string
	fiveCsvPath   string
	interp        seveninterp.Interpolator
	fiveInterp    coreinterp.Interpolator
	err           error
}

func (l *recordingSevenHoleLoader) LoadPRB(path string) (coreinterp.Interpolator, error) {
	l.prbCalls++
	l.prbPath = path
	if l.err != nil {
		return nil, l.err
	}
	return l.fiveInterp, nil
}
func (l *recordingSevenHoleLoader) LoadFiveHoleCSV(path string) (coreinterp.Interpolator, error) {
	l.csvFiveCalls++
	l.fiveCsvPath = path
	if l.err != nil {
		return nil, l.err
	}
	return l.fiveInterp, nil
}
func (l *recordingSevenHoleLoader) LoadMultiPRB(paths []string, _ []float64, _ coreinterp.MultiPrbInterpolationMode) (coreinterp.Interpolator, *ports.MultiPrbLoadMetadata, error) {
	l.multiPrbCalls++
	if l.err != nil {
		return nil, nil, l.err
	}
	return l.fiveInterp, nil, nil
}

// LoadSevenHolePRB Task 07 新签名：返回中立 metadata；pointCount 不暴露
// （兼容约定值 169/52 不应伪装为 loader 真值）。
func (l *recordingSevenHoleLoader) LoadSevenHolePRB(innerPath string, outerPaths [6]string) (seveninterp.Interpolator, *ports.SevenHoleLoadMetadata, error) {
	l.calls++
	l.innerPath = innerPath
	l.outerPaths = outerPaths
	if l.err != nil {
		return nil, nil, l.err
	}
	return l.interp, &ports.SevenHoleLoadMetadata{}, nil
}
func (l *recordingSevenHoleLoader) LoadSevenHoleCalibrationCSV(innerPath string, outerPaths [6]string) (seveninterp.Interpolator, *ports.SevenHoleLoadMetadata, error) {
	l.csvCalls++
	l.innerPath = innerPath
	l.outerPaths = outerPaths
	if l.err != nil {
		return nil, nil, l.err
	}
	return l.interp, &ports.SevenHoleLoadMetadata{}, nil
}

// sevenHoleConfigJSON 构造一份合法的七孔持久化配置 JSON。
func sevenHoleConfigJSON() []byte {
	return []byte(`{
		"probeType": "seven-hole",
		"sevenHolePrb": {
			"kind": "seven-hole-prb-set",
			"innerFile": {"filePath": "D:/cal/7.prb"},
			"outerFiles": [
				{"filePath": "D:/cal/1.prb"},
				{"filePath": "D:/cal/2.prb"},
				{"filePath": "D:/cal/3.prb"},
				{"filePath": "D:/cal/4.prb"},
				{"filePath": "D:/cal/5.prb"},
				{"filePath": "D:/cal/6.prb"}
			]
		}
	}`)
}

func newRestoreManager(probeType string) *TraversalManager {
	mgr := NewTraversalManager(nil, nil, nil, nil, nil)
	mgr.config = traversal.Config{ProbeType: probeType}
	return mgr
}

// TestRestoreSevenHole 七孔配置持久化→恢复→七孔 probeType 下 HasLoadedInterpolator()==true，
// 且 loader 收到 1 内区 + 6 扇区路径（spec §5.4 唯一七孔恢复路径）。
func TestRestoreSevenHole(t *testing.T) {
	loader := &recordingSevenHoleLoader{interp: &mockSevenInterpolator{loaded: true}}
	mgr := newRestoreManager(traversal.ProbeTypeSevenHole)
	mgr.restoreInterpolatorFromConfig(sevenHoleConfigJSON(), loader, 0, 0)

	if loader.calls != 1 {
		t.Fatalf("LoadSevenHolePRB calls = %d, want 1", loader.calls)
	}
	if loader.innerPath != "D:/cal/7.prb" {
		t.Errorf("innerPath = %q, want D:/cal/7.prb", loader.innerPath)
	}
	for i, p := range loader.outerPaths {
		want := fmt.Sprintf("D:/cal/%d.prb", i+1)
		if p != want {
			t.Errorf("outerPaths[%d] = %q, want %q", i, p, want)
		}
	}
	if !mgr.HasLoadedInterpolator() {
		t.Error("seven-hole probeType must report loaded after restore")
	}
	if got := mgr.InterpolatorRestoreErr(); got != "" {
		t.Errorf("restore err = %q, want empty", got)
	}
}

func TestRestoreDualVariantEachGetsFullTimeout(t *testing.T) {
	loader := &mockInterpolatorLoader{
		blockFor:          40 * time.Millisecond,
		sevenHoleBlockFor: 40 * time.Millisecond,
	}
	loader.sevenHoleValidRange = seveninterp.PrbValidRange{}
	mgr := newRestoreManager(traversal.ProbeTypeFiveHole)
	cfg := []byte(`{
		"probeType":"five-hole",
		"prbFile":{"filePath":"D:/cal/five.prb"},
		"sevenHolePrb":{
			"kind":"seven-hole-prb-set",
			"innerFile":{"filePath":"D:/cal/7.prb"},
			"outerFiles":[
				{"filePath":"D:/cal/1.prb"},{"filePath":"D:/cal/2.prb"},
				{"filePath":"D:/cal/3.prb"},{"filePath":"D:/cal/4.prb"},
				{"filePath":"D:/cal/5.prb"},{"filePath":"D:/cal/6.prb"}
			]
		}
	}`)

	mgr.restoreInterpolatorFromConfigWithTimeout(cfg, loader, 0, 0, 60*time.Millisecond)

	if got := mgr.InterpolatorRestoreErrFor(traversal.ProbeTypeFiveHole); got != "" {
		t.Fatalf("five-hole restore error = %q", got)
	}
	if got := mgr.InterpolatorRestoreErrFor(traversal.ProbeTypeSevenHole); got != "" {
		t.Fatalf("seven-hole restore error = %q", got)
	}
	if loader.prbCalls != 1 || loader.sevenHolePRBCalls != 1 {
		t.Fatalf("loader calls: five=%d seven=%d, want 1 each", loader.prbCalls, loader.sevenHolePRBCalls)
	}
}

// TestRestoreSevenHoleIncomplete 七孔字段缺失或文件集不完整时视为该变体未配置，
// 跳过且不报错（双变体语义：缺失不等于失败，仅意味着该侧未配置，前端按需引导导入）。
//
// 与旧设计的差异：旧设计将"文件集不完整"视为恢复失败并写入错误。
// 新设计下，缺失字段表示该变体未启用，与五孔字段缺失的处理保持对称；
// 不再调用 loader，错误字段保持空。
func TestRestoreSevenHoleIncomplete(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{"missing sevenHolePrb", `{"probeType":"seven-hole"}`},
		{"5 outer files", `{"probeType":"seven-hole","sevenHolePrb":{"innerFile":{"filePath":"7.prb"},"outerFiles":[{"filePath":"1.prb"},{"filePath":"2.prb"},{"filePath":"3.prb"},{"filePath":"4.prb"},{"filePath":"5.prb"}]}}`},
		{"missing innerFile", `{"probeType":"seven-hole","sevenHolePrb":{"outerFiles":[{"filePath":"1.prb"},{"filePath":"2.prb"},{"filePath":"3.prb"},{"filePath":"4.prb"},{"filePath":"5.prb"},{"filePath":"6.prb"}]}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loader := &recordingSevenHoleLoader{}
			mgr := newRestoreManager(traversal.ProbeTypeSevenHole)
			mgr.restoreInterpolatorFromConfig([]byte(tc.json), loader, 0, 0)
			if loader.calls != 0 {
				t.Error("incomplete seven-hole config must not start the loader")
			}
			if got := mgr.InterpolatorRestoreErrFor(traversal.ProbeTypeSevenHole); got != "" {
				t.Errorf("seven-hole restore err = %q, want empty (missing fields treated as unconfigured)", got)
			}
			// 七孔字段缺失时五孔侧也不应被触达（五孔字段同样缺失）
			if loader.prbCalls != 0 || loader.csvFiveCalls != 0 || loader.multiPrbCalls != 0 {
				t.Errorf("five-hole loaders must not be called when five-hole fields absent, got prb=%d csvFive=%d multi=%d",
					loader.prbCalls, loader.csvFiveCalls, loader.multiPrbCalls)
			}
		})
	}
}

// TestRestoreDualVariantAccepted 双变体并行恢复语义：
//   - 五孔字段与 sevenHolePrb 并存时，两侧 loader 均被调用，互不阻塞；
//   - 七孔加载失败不影响五孔加载（反之亦然）——错误按类型分桶；
//   - 未知 probeType 两侧同时报错且不启动任何 loader。
func TestRestoreDualVariantAccepted(t *testing.T) {
	t.Run("seven-hole active with five-hole fields present", func(t *testing.T) {
		// 七孔激活 + 五孔字段并存：两侧 loader 均被调用一次，五孔/七孔错误都为空。
		cfg := `{"probeType":"seven-hole","prbFile":{"filePath":"a.prb"},` +
			`"sevenHolePrb":{"innerFile":{"filePath":"7.prb"},"outerFiles":[{"filePath":"1.prb"},{"filePath":"2.prb"},{"filePath":"3.prb"},{"filePath":"4.prb"},{"filePath":"5.prb"},{"filePath":"6.prb"}]}}`
		// mockInterpolator.IsLoaded() 永远返回 true，无需 loaded 字段。
		loader := &recordingSevenHoleLoader{
			interp:     &mockSevenInterpolator{loaded: true},
			fiveInterp: &mockInterpolator{},
		}
		mgr := newRestoreManager(traversal.ProbeTypeSevenHole)
		mgr.restoreInterpolatorFromConfig([]byte(cfg), loader, 0, 0)
		if loader.calls != 1 {
			t.Fatalf("seven-hole loader calls = %d, want 1", loader.calls)
		}
		if loader.prbCalls != 1 {
			t.Errorf("five-hole PRB loader calls = %d, want 1 (dual-variant parallel restore)", loader.prbCalls)
		}
		if got := mgr.InterpolatorRestoreErrFor(traversal.ProbeTypeSevenHole); got != "" {
			t.Errorf("seven-hole restore err = %q, want empty", got)
		}
		if got := mgr.InterpolatorRestoreErrFor(traversal.ProbeTypeFiveHole); got != "" {
			t.Errorf("five-hole restore err = %q, want empty", got)
		}
		if !mgr.HasLoadedInterpolatorFor(traversal.ProbeTypeSevenHole) {
			t.Error("seven-hole must be loaded after restore")
		}
		if !mgr.HasLoadedInterpolatorFor(traversal.ProbeTypeFiveHole) {
			t.Error("five-hole must be loaded after restore (dual-variant)")
		}
	})

	t.Run("five-hole active with sevenHolePrb present", func(t *testing.T) {
		// 五孔激活 + sevenHolePrb 并存：两侧 loader 均被调用，与激活类型无关。
		// 这是双变体恢复的核心——切换激活类型后未恢复侧不再报"未加载"。
		cfg := `{"probeType":"five-hole","prbFile":{"filePath":"a.prb"},` +
			`"sevenHolePrb":{"innerFile":{"filePath":"7.prb"},"outerFiles":[{"filePath":"1.prb"},{"filePath":"2.prb"},{"filePath":"3.prb"},{"filePath":"4.prb"},{"filePath":"5.prb"},{"filePath":"6.prb"}]}}`
		// mockInterpolator.IsLoaded() 永远返回 true，无需 loaded 字段。
		loader := &recordingSevenHoleLoader{
			interp:     &mockSevenInterpolator{loaded: true},
			fiveInterp: &mockInterpolator{},
		}
		mgr := newRestoreManager(traversal.ProbeTypeFiveHole)
		mgr.restoreInterpolatorFromConfig([]byte(cfg), loader, 0, 0)
		if loader.calls != 1 {
			t.Errorf("seven-hole loader calls = %d, want 1 (independent of active probeType)", loader.calls)
		}
		if loader.prbCalls != 1 {
			t.Errorf("five-hole PRB loader calls = %d, want 1", loader.prbCalls)
		}
		if got := mgr.InterpolatorRestoreErrFor(traversal.ProbeTypeSevenHole); got != "" {
			t.Errorf("seven-hole restore err = %q, want empty", got)
		}
		if got := mgr.InterpolatorRestoreErrFor(traversal.ProbeTypeFiveHole); got != "" {
			t.Errorf("five-hole restore err = %q, want empty", got)
		}
	})

	t.Run("seven-hole loader failure does not affect five-hole", func(t *testing.T) {
		// 七孔 loader 失败仅写入七孔错误桶，五孔侧仍成功加载。
		cfg := `{"probeType":"five-hole","prbFile":{"filePath":"a.prb"},` +
			`"sevenHolePrb":{"innerFile":{"filePath":"7.prb"},"outerFiles":[{"filePath":"1.prb"},{"filePath":"2.prb"},{"filePath":"3.prb"},{"filePath":"4.prb"},{"filePath":"5.prb"},{"filePath":"6.prb"}]}}`
		// 当前 recordingSevenHoleLoader 用单一 err 字段同时影响五孔和七孔。
		// 这里需要更细粒度的注入：临时让 LoadSevenHolePRB 通过 interp=nil 模拟失败。
		// mockInterpolator.IsLoaded() 永远返回 true，无需 loaded 字段。
		loader := &recordingSevenHoleLoader{
			interp:     nil, // 七孔 interp 为 nil，loader 仍返回 nil, nil
			fiveInterp: &mockInterpolator{},
		}
		mgr := newRestoreManager(traversal.ProbeTypeFiveHole)
		mgr.restoreInterpolatorFromConfig([]byte(cfg), loader, 0, 0)
		// 五孔侧正常加载，错误为空
		if got := mgr.InterpolatorRestoreErrFor(traversal.ProbeTypeFiveHole); got != "" {
			t.Errorf("five-hole restore err = %q, want empty (independent of seven-hole failure)", got)
		}
		if !mgr.HasLoadedInterpolatorFor(traversal.ProbeTypeFiveHole) {
			t.Error("five-hole must remain loaded despite seven-hole nil interpolator")
		}
	})

	t.Run("unknown probeType still rejected", func(t *testing.T) {
		loader := &recordingSevenHoleLoader{}
		mgr := newRestoreManager("")
		mgr.restoreInterpolatorFromConfig([]byte(`{"probeType":"nine-hole"}`), loader, 0, 0)
		if loader.calls != 0 {
			t.Error("unknown probeType must not start any loader")
		}
		if got := mgr.InterpolatorRestoreErrFor(traversal.ProbeTypeSevenHole); !strings.Contains(got, "未知探针类型") {
			t.Errorf("seven-hole restore err = %q, want 未知探针类型", got)
		}
		if got := mgr.InterpolatorRestoreErrFor(traversal.ProbeTypeFiveHole); !strings.Contains(got, "未知探针类型") {
			t.Errorf("five-hole restore err = %q, want 未知探针类型", got)
		}
	})
}

// TestRestoreSevenHoleCalibrationCsv 校准 CSV 变体恢复：
// kind=seven-hole-calibration-csv 时走 LoadSevenHoleCalibrationCSV（不走 PRB loader）。
func TestRestoreSevenHoleCalibrationCsv(t *testing.T) {
	cfg := `{"probeType":"seven-hole","sevenHolePrb":{"kind":"seven-hole-calibration-csv","innerFile":{"filePath":"inner.csv"},"outerFiles":[{"filePath":"1.csv"},{"filePath":"2.csv"},{"filePath":"3.csv"},{"filePath":"4.csv"},{"filePath":"5.csv"},{"filePath":"6.csv"}]}}`
	loader := &recordingSevenHoleLoader{interp: &mockSevenInterpolator{loaded: true}}
	mgr := newRestoreManager(traversal.ProbeTypeSevenHole)
	mgr.restoreInterpolatorFromConfig([]byte(cfg), loader, 0, 0)

	if loader.csvCalls != 1 {
		t.Fatalf("LoadSevenHoleCalibrationCSV calls = %d, want 1", loader.csvCalls)
	}
	if loader.calls != 0 {
		t.Error("csv kind must not start the PRB loader")
	}
	if loader.innerPath != "inner.csv" {
		t.Errorf("innerPath = %q, want inner.csv", loader.innerPath)
	}
	for i, p := range loader.outerPaths {
		want := fmt.Sprintf("%d.csv", i+1)
		if p != want {
			t.Errorf("outerPaths[%d] = %q, want %q", i, p, want)
		}
	}
	if !mgr.HasLoadedInterpolator() {
		t.Error("seven-hole calibration-csv restore must report loaded")
	}
}

// TestRestoreSevenHoleBadKind 非法 kind 在恢复侧按不完整文件集处理前被规范化拦截（ParseConfig），
// 恢复路径遇到未知 kind 时按 PRB 集处理——此处锁定 ParseConfig 的 kind 校验。
func TestParseConfigSevenHoleBadKind(t *testing.T) {
	mgr := NewTraversalManager(nil, nil, nil, nil, nil)
	body := `{
		"name": "t",
		"layout": {"pattern": "line", "line": {"startX": 0, "endX": 10, "xStepSegments": [{"start": 0, "end": 10, "step": 5}]}},
		"channels": {"probeChannels": [
			{"name":"P1","role":"sevenHole.p1","channel":{"deviceId":"dev","channelIndex":0},"enabled":true},
			{"name":"P2","role":"sevenHole.p2","channel":{"deviceId":"dev","channelIndex":1},"enabled":true},
			{"name":"P3","role":"sevenHole.p3","channel":{"deviceId":"dev","channelIndex":2},"enabled":true},
			{"name":"P4","role":"sevenHole.p4","channel":{"deviceId":"dev","channelIndex":3},"enabled":true},
			{"name":"P5","role":"sevenHole.p5","channel":{"deviceId":"dev","channelIndex":4},"enabled":true},
			{"name":"P6","role":"sevenHole.p6","channel":{"deviceId":"dev","channelIndex":5},"enabled":true},
			{"name":"P7","role":"sevenHole.p7","channel":{"deviceId":"dev","channelIndex":6},"enabled":true},
			{"name":"Patm","role":"sevenHole.pAtm","channel":{"deviceId":"dev","channelIndex":16},"enabled":true},
			{"name":"Tatm","role":"sevenHole.tAtm","channel":{"deviceId":"dev","channelIndex":17},"enabled":true}
		]},
		"dwellTimeMs": 100, "samplesPerPoint": 10,
		"probeType": "seven-hole",
		"sevenHolePrb": {"kind": "wrong-kind", "innerFile": {"filePath": "7.prb"}, "outerFiles": [{"filePath":"1.prb"},{"filePath":"2.prb"},{"filePath":"3.prb"},{"filePath":"4.prb"},{"filePath":"5.prb"},{"filePath":"6.prb"}]}
	}`
	if _, err := mgr.ParseConfig([]byte(body)); err == nil {
		t.Fatal("unknown seven-hole kind must error")
	} else if !strings.Contains(err.Error(), "kind") {
		t.Errorf("error %q must name kind", err.Error())
	}
}

// TestRestoreSevenHoleLoaderError loader 失败时错误透传到 restore err。
func TestRestoreSevenHoleLoaderError(t *testing.T) {
	loader := &recordingSevenHoleLoader{err: errors.New("disk gone")}
	mgr := newRestoreManager(traversal.ProbeTypeSevenHole)
	mgr.restoreInterpolatorFromConfig(sevenHoleConfigJSON(), loader, 0, 0)
	if loader.calls != 1 {
		t.Fatalf("calls = %d, want 1", loader.calls)
	}
	if got := mgr.InterpolatorRestoreErr(); !strings.Contains(got, "disk gone") {
		t.Errorf("restore err = %q, want loader error propagated", got)
	}
	if mgr.HasLoadedInterpolator() {
		t.Error("failed restore must not leave a loaded interpolator")
	}
}

// TestRoleToLabelSevenHole 七孔 9 角色映射逐项核对；五孔映射不变。
func TestRoleToLabelSevenHole(t *testing.T) {
	seven := map[string]string{
		"sevenHole.p1": "P1", "sevenHole.p2": "P2", "sevenHole.p3": "P3",
		"sevenHole.p4": "P4", "sevenHole.p5": "P5", "sevenHole.p6": "P6",
		"sevenHole.p7": "P7", "sevenHole.pAtm": "Patm", "sevenHole.tAtm": "Tatm",
	}
	for role, want := range seven {
		if got := roleToLabel(role, "fallback"); got != want {
			t.Errorf("roleToLabel(%q) = %q, want %q", role, got, want)
		}
	}
	five := map[string]string{
		"fiveHole.p1": "P1", "fiveHole.p2": "P2", "fiveHole.p3": "P3",
		"fiveHole.p4": "P4", "fiveHole.p5": "P5", "fiveHole.pAtm": "Patm", "fiveHole.tAtm": "Tatm",
	}
	for role, want := range five {
		if got := roleToLabel(role, "fallback"); got != want {
			t.Errorf("roleToLabel(%q) = %q, want %q (five-hole unchanged)", role, got, want)
		}
	}
	if got := roleToLabel("unknown.role", "customName"); got != "customName" {
		t.Errorf("unknown role must fall back to name, got %q", got)
	}
}

// TestParseConfigProbeType ParseConfig 边界：旧配置归一化为五孔；未知类型、
// 五孔携带 sevenHolePrb、七孔文件集不齐均报错。
func TestParseConfigProbeType(t *testing.T) {
	// 最小合法配置体（line 布局 + 通道），探针相关字段按用例注入。
	base := func(probeFields string) string {
		return `{
			"name": "t",
			"layout": {"pattern": "line", "line": {"startX": 0, "endX": 10, "xStepSegments": [{"start": 0, "end": 10, "step": 5}]}},
			"channels": {"probeChannels": [
				{"name":"P1","role":"sevenHole.p1","channel":{"deviceId":"dev","channelIndex":0},"enabled":true},
				{"name":"P2","role":"sevenHole.p2","channel":{"deviceId":"dev","channelIndex":1},"enabled":true},
				{"name":"P3","role":"sevenHole.p3","channel":{"deviceId":"dev","channelIndex":2},"enabled":true},
				{"name":"P4","role":"sevenHole.p4","channel":{"deviceId":"dev","channelIndex":3},"enabled":true},
				{"name":"P5","role":"sevenHole.p5","channel":{"deviceId":"dev","channelIndex":4},"enabled":true},
				{"name":"P6","role":"sevenHole.p6","channel":{"deviceId":"dev","channelIndex":5},"enabled":true},
				{"name":"P7","role":"sevenHole.p7","channel":{"deviceId":"dev","channelIndex":6},"enabled":true},
				{"name":"Patm","role":"sevenHole.pAtm","channel":{"deviceId":"dev","channelIndex":16},"enabled":true},
				{"name":"Tatm","role":"sevenHole.tAtm","channel":{"deviceId":"dev","channelIndex":17},"enabled":true}
			]},
			"dwellTimeMs": 100,
			"samplesPerPoint": 10` + probeFields + `}`
	}
	sevenPrb := `,
			"sevenHolePrb": {"kind":"seven-hole-prb-set","innerFile":{"filePath":"7.prb"},"outerFiles":[{"filePath":"1.prb"},{"filePath":"2.prb"},{"filePath":"3.prb"},{"filePath":"4.prb"},{"filePath":"5.prb"},{"filePath":"6.prb"}]}`

	mgr := NewTraversalManager(nil, nil, nil, nil, nil)

	t.Run("legacy defaults to five-hole", func(t *testing.T) {
		cfg, err := mgr.ParseConfig([]byte(base("")))
		if err != nil {
			t.Fatalf("legacy config must parse: %v", err)
		}
		if cfg.ProbeType != traversal.ProbeTypeFiveHole {
			t.Errorf("ProbeType = %q, want five-hole (legacy normalization)", cfg.ProbeType)
		}
		if cfg.IsSevenHole() {
			t.Error("legacy config must not be seven-hole")
		}
	})

	t.Run("seven-hole accepted", func(t *testing.T) {
		cfg, err := mgr.ParseConfig([]byte(base(`,"probeType":"seven-hole"` + sevenPrb)))
		if err != nil {
			t.Fatalf("seven-hole config must parse: %v", err)
		}
		if !cfg.IsSevenHole() {
			t.Error("ProbeType must pass through as seven-hole")
		}
		// 七孔 9 角色经 roleToLabel 装配到 ChannelLabels
		// 注意：ChannelLabels 是 map[int]string，Go map 遍历顺序不固定，
		// 必须按"标签集合"比较，不能按 index 比较顺序（避免 flaky）
		labelSet := make(map[string]struct{}, len(cfg.ChannelLabels))
		for _, v := range cfg.ChannelLabels {
			labelSet[v] = struct{}{}
		}
		wantLabels := []string{"P1", "P2", "P3", "P4", "P5", "P6", "P7", "Patm", "Tatm"}
		if len(labelSet) != len(wantLabels) {
			t.Fatalf("ChannelLabels 数量 = %d, want %d", len(labelSet), len(wantLabels))
		}
		for _, want := range wantLabels {
			if _, ok := labelSet[want]; !ok {
				t.Errorf("ChannelLabels 缺少标签 %q（实际集合: %v）", want, labelSet)
			}
		}
	})

	t.Run("unknown probeType rejected", func(t *testing.T) {
		if _, err := mgr.ParseConfig([]byte(base(`,"probeType":"nine-hole"`))); err == nil {
			t.Fatal("unknown probeType must error")
		}
	})

	t.Run("five-hole with sevenHolePrb accepted (dual-variant)", func(t *testing.T) {
		cfg, err := mgr.ParseConfig([]byte(base(`,"probeType":"five-hole"` + sevenPrb)))
		if err != nil {
			t.Fatalf("five-hole + sevenHolePrb must parse (dual-variant): %v", err)
		}
		if cfg.ProbeType != traversal.ProbeTypeFiveHole {
			t.Errorf("ProbeType = %q, want five-hole", cfg.ProbeType)
		}
	})

	t.Run("empty probeType with sevenHolePrb accepted (dual-variant)", func(t *testing.T) {
		cfg, err := mgr.ParseConfig([]byte(base(sevenPrb)))
		if err != nil {
			t.Fatalf("empty probeType + sevenHolePrb must parse (dual-variant, normalizes to five-hole): %v", err)
		}
		if cfg.ProbeType != traversal.ProbeTypeFiveHole {
			t.Errorf("ProbeType = %q, want five-hole (legacy normalization)", cfg.ProbeType)
		}
	})

	t.Run("seven-hole incomplete set rejected", func(t *testing.T) {
		if _, err := mgr.ParseConfig([]byte(base(`,"probeType":"seven-hole"`))); err == nil {
			t.Fatal("seven-hole without sevenHolePrb must error")
		}
	})
}
