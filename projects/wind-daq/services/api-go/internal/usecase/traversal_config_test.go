package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	seveninterp "ai-workspace/shared/algorithms/go/sevenhole/interpolation"
	"wind-daq/services/api-go/internal/adapters/storage"
	"wind-daq/services/api-go/internal/core/motion"
)

// ===== InterpolatorLoader mock 实现 =====

// mockInterpolator 是测试用的最小 Interpolator 实现，只断言是否被注入到 manager 中。
type mockInterpolator struct {
	tag string
}

func (m *mockInterpolator) IsLoaded() bool { return true }
func (m *mockInterpolator) GetValidRange() coreinterp.PrbValidRange {
	return coreinterp.PrbValidRange{}
}
func (m *mockInterpolator) Calculate(_ coreinterp.InterpolationInput) (coreinterp.InterpolationResult, error) {
	return coreinterp.InterpolationResult{}, nil
}

// mockSevenHoleInterpolator 是 seveninterp.Interpolator 的最小实现，
// 仅用于满足 ports.InterpolatorLoader.LoadSevenHolePRB 的接口契约；
// 当前 traversal_config_test 不覆盖七孔加载的实际数据流，所以 Calculate 永远返回零值。
// 若后续新增七孔路径测试，应在此扩展为可控 mock（带输入快照、错误开关等）。
type mockSevenHoleInterpolator struct {
	tag string
}

func (m *mockSevenHoleInterpolator) IsLoaded() bool { return true }
func (m *mockSevenHoleInterpolator) GetValidRange() seveninterp.PrbValidRange {
	return seveninterp.PrbValidRange{}
}
func (m *mockSevenHoleInterpolator) Calculate(_ seveninterp.InterpolationInput) (seveninterp.InterpolationResult, error) {
	return seveninterp.InterpolationResult{}, nil
}

// mockInterpolatorLoader 是 ports.InterpolatorLoader 的可控 mock，
// 用于覆盖 CSV / 单 PRB / 多 PRB 三条加载分支以及成功 / 失败 / 阻塞三种场景。
type mockInterpolatorLoader struct {
	mu sync.Mutex

	// 触发条件：调用入参回填
	lastPRB       string
	lastCSV       string
	lastMultiPRB  []string
	lastMode      coreinterp.MultiPrbInterpolationMode
	lastMachs     []float64
	prbCalls      int
	csvCalls      int
	multiPRBCalls int

	// 行为开关
	prbErr      error
	csvErr      error
	multiPRBErr error
	// blockFor 模拟磁盘阻塞，常用于测试 ctx 超时分支
	blockFor time.Duration
}

func (l *mockInterpolatorLoader) LoadPRB(filePath string) (coreinterp.Interpolator, error) {
	l.mu.Lock()
	l.prbCalls++
	l.lastPRB = filePath
	blockFor := l.blockFor
	prbErr := l.prbErr
	l.mu.Unlock()
	if blockFor > 0 {
		time.Sleep(blockFor)
	}
	if prbErr != nil {
		return nil, prbErr
	}
	return &mockInterpolator{tag: "prb:" + filePath}, nil
}

func (l *mockInterpolatorLoader) LoadFiveHoleCSV(filePath string) (coreinterp.Interpolator, error) {
	l.mu.Lock()
	l.csvCalls++
	l.lastCSV = filePath
	blockFor := l.blockFor
	csvErr := l.csvErr
	l.mu.Unlock()
	if blockFor > 0 {
		time.Sleep(blockFor)
	}
	if csvErr != nil {
		return nil, csvErr
	}
	return &mockInterpolator{tag: "csv:" + filePath}, nil
}

func (l *mockInterpolatorLoader) LoadMultiPRB(filePaths []string, machNumbers []float64, mode coreinterp.MultiPrbInterpolationMode) (coreinterp.Interpolator, error) {
	l.mu.Lock()
	l.multiPRBCalls++
	l.lastMultiPRB = append([]string(nil), filePaths...)
	l.lastMachs = append([]float64(nil), machNumbers...)
	l.lastMode = mode
	blockFor := l.blockFor
	multiPRBErr := l.multiPRBErr
	l.mu.Unlock()
	if blockFor > 0 {
		time.Sleep(blockFor)
	}
	if multiPRBErr != nil {
		return nil, multiPRBErr
	}
	return &mockInterpolator{tag: "multi"}, nil
}

// LoadSevenHolePRB 当前测试不覆盖七孔加载分支，仅满足接口契约。
// 返回空 mock + nil 错误；若后续需要测试七孔路径，可在此扩展为
// 带 lastSevenHoleInner/lastSevenHoleOuter/sevenHolePRBErr 字段的可控 mock。
func (l *mockInterpolatorLoader) LoadSevenHolePRB(innerPath string, outerPaths [6]string) (seveninterp.Interpolator, error) {
	return &mockSevenHoleInterpolator{tag: "seven-hole:" + innerPath}, nil
}

// LoadSevenHoleCalibrationCSV 满足接口契约（校准 CSV 加载分支）。
func (l *mockInterpolatorLoader) LoadSevenHoleCalibrationCSV(innerPath string, outerPaths [6]string) (seveninterp.Interpolator, error) {
	return &mockSevenHoleInterpolator{tag: "seven-hole-csv:" + innerPath}, nil
}

func (l *mockInterpolatorLoader) snapshot() (lastPRB, lastCSV string, lastMultiPRB []string, lastMachs []float64, lastMode coreinterp.MultiPrbInterpolationMode, prbCalls, csvCalls, multiPRBCalls int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lastPRB, l.lastCSV, append([]string(nil), l.lastMultiPRB...), append([]float64(nil), l.lastMachs...), l.lastMode, l.prbCalls, l.csvCalls, l.multiPRBCalls
}

func (l *mockInterpolatorLoader) totalCalls() int {
	_, _, _, _, _, prbCalls, csvCalls, multiPRBCalls := l.snapshot()
	return prbCalls + csvCalls + multiPRBCalls
}

// newConfigTestManager 构造一个最小可用的 TraversalManager，用于 traversal_config 相关测试。
// 不依赖真实硬件 / 文件系统，所有外部端口均为 nil 或测试专用实现。
func newConfigTestManager(t *testing.T) *TraversalManager {
	t.Helper()
	reader := &mockLatestDataReader{}
	motionAccess := &mockMotionAccess{}
	sink := &mockTraversalPointSink{}
	store := newMockTraversalResultStore()
	return NewTraversalManager(reader, motionAccess, sink, store, storage.NewFileCheckpointStore())
}

// waitForRestoreSettled 等待异步恢复 goroutine 写入完成。
// 测试侧通过轮询 InterpolatorRestoreErr / Interpolator() 的可观察状态判断结束，
// 避免依赖 sleep 而引起 flaky 测试。
func waitForRestoreSettled(t *testing.T, mgr *TraversalManager, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("等待恢复结果超时；err=%q, interpolator=%v", mgr.InterpolatorRestoreErr(), mgr.Interpolator())
}

// ===== 单元测试 =====

// 当未注入 loader 时，应记录"未注入插值器加载端口"错误并跳过加载。
func TestRestoreInterpolator_NoLoader(t *testing.T) {
	mgr := newConfigTestManager(t)
	mgr.SaveConfigRaw(json.RawMessage(`{"prbFile":{"filePath":"any.prb"}}`))
	mgr.RestoreInterpolatorFromPersistedConfig()
	if mgr.InterpolatorRestoreErr() == "" {
		t.Fatalf("期望记录加载器缺失错误，实际为空")
	}
	if mgr.Interpolator() != nil {
		t.Fatalf("未注入 loader 时不应注入插值器")
	}
}

// configRaw 为空时，恢复函数应静默返回，既不调用 loader 也不写错误。
func TestRestoreInterpolator_EmptyConfig(t *testing.T) {
	mgr := newConfigTestManager(t)
	loader := &mockInterpolatorLoader{}
	mgr.SetInterpolatorLoader(loader)
	mgr.RestoreInterpolatorFromPersistedConfig()
	// configRaw 为空 → 直接返回，loader 不会被调用
	time.Sleep(50 * time.Millisecond)
	_, _, _, _, _, prbCalls, csvCalls, multiPRBCalls := loader.snapshot()
	if prbCalls+csvCalls+multiPRBCalls != 0 {
		t.Fatalf("空配置不应触发任何 loader 调用，实际 prb=%d csv=%d multi=%d", prbCalls, csvCalls, multiPRBCalls)
	}
	if mgr.InterpolatorRestoreErr() != "" {
		t.Fatalf("空配置不应写错误，实际: %q", mgr.InterpolatorRestoreErr())
	}
}

// 单 PRB 分支：成功路径应调用 LoadPRB 并注入插值器。
func TestRestoreInterpolator_PRB_Success(t *testing.T) {
	mgr := newConfigTestManager(t)
	loader := &mockInterpolatorLoader{}
	mgr.SetInterpolatorLoader(loader)
	mgr.SaveConfigRaw(json.RawMessage(`{"prbFile":{"filePath":"/tmp/test.prb"}}`))
	mgr.RestoreInterpolatorFromPersistedConfig()

	waitForRestoreSettled(t, mgr, func() bool { return mgr.Interpolator() != nil })
	lastPRB, _, _, _, _, _, _, _ := loader.snapshot()
	if lastPRB != "/tmp/test.prb" {
		t.Fatalf("LoadPRB 应传入 /tmp/test.prb，实际 %q", lastPRB)
	}
	if mgr.InterpolatorRestoreErr() != "" {
		t.Fatalf("成功路径不应留下错误，实际: %q", mgr.InterpolatorRestoreErr())
	}
}

// 单 PRB 分支：失败路径应写入错误且不注入插值器。
func TestRestoreInterpolator_PRB_Failure(t *testing.T) {
	mgr := newConfigTestManager(t)
	loader := &mockInterpolatorLoader{prbErr: errors.New("disk gone")}
	mgr.SetInterpolatorLoader(loader)
	mgr.SaveConfigRaw(json.RawMessage(`{"prbFile":{"filePath":"/tmp/missing.prb"}}`))
	mgr.RestoreInterpolatorFromPersistedConfig()
	waitForRestoreSettled(t, mgr, func() bool { return mgr.InterpolatorRestoreErr() != "" })
	if mgr.Interpolator() != nil {
		t.Fatalf("失败路径不应注入插值器")
	}
}

// 新算法 CSV 分支：interpolationAlgorithm=new + calibrationCsvFile.filePath 时走 CSV 加载。
func TestRestoreInterpolator_CSV_Success(t *testing.T) {
	mgr := newConfigTestManager(t)
	loader := &mockInterpolatorLoader{}
	mgr.SetInterpolatorLoader(loader)
	mgr.SaveConfigRaw(json.RawMessage(`{"interpolationAlgorithm":"new","calibrationCsvFile":{"filePath":"/tmp/cal.csv"}}`))
	mgr.RestoreInterpolatorFromPersistedConfig()
	waitForRestoreSettled(t, mgr, func() bool { return loader.totalCalls() > 0 })
	_, lastCSV, _, _, _, _, _, _ := loader.snapshot()
	if lastCSV != "/tmp/cal.csv" {
		t.Fatalf("LoadFiveHoleCSV 应传入 /tmp/cal.csv，实际 %q", lastCSV)
	}
	if mgr.Interpolator() == nil {
		t.Fatalf("CSV 成功路径应注入插值器")
	}
}

// CSV 分支失败应记录错误。
func TestRestoreInterpolator_CSV_Failure(t *testing.T) {
	mgr := newConfigTestManager(t)
	loader := &mockInterpolatorLoader{csvErr: errors.New("parse error")}
	mgr.SetInterpolatorLoader(loader)
	mgr.SaveConfigRaw(json.RawMessage(`{"interpolationAlgorithm":"new","calibrationCsvFile":{"filePath":"/tmp/bad.csv"}}`))
	mgr.RestoreInterpolatorFromPersistedConfig()
	waitForRestoreSettled(t, mgr, func() bool { return mgr.InterpolatorRestoreErr() != "" })
	if mgr.Interpolator() != nil {
		t.Fatalf("CSV 失败路径不应注入插值器")
	}
}

// 多 PRB 分支：成功时应把 machNumbers / mode 透传到 loader。
func TestRestoreInterpolator_MultiPRB_Success(t *testing.T) {
	mgr := newConfigTestManager(t)
	loader := &mockInterpolatorLoader{}
	mgr.SetInterpolatorLoader(loader)
	mgr.SaveConfigRaw(json.RawMessage(`{
		"useMultiPrb": true,
		"multiPrb": {
			"files": [{"filePath":"a.prb"},{"filePath":"b.prb"}],
			"machNumbers": [0.3, 0.6],
			"interpolationMode": "linear"
		}
	}`))
	mgr.RestoreInterpolatorFromPersistedConfig()
	waitForRestoreSettled(t, mgr, func() bool { return loader.totalCalls() > 0 })

	_, _, lastMultiPRB, lastMachs, lastMode, _, _, _ := loader.snapshot()
	if len(lastMultiPRB) != 2 || lastMultiPRB[0] != "a.prb" || lastMultiPRB[1] != "b.prb" {
		t.Fatalf("LoadMultiPRB filePaths 透传错误: %#v", lastMultiPRB)
	}
	if len(lastMachs) != 2 || lastMachs[0] != 0.3 || lastMachs[1] != 0.6 {
		t.Fatalf("LoadMultiPRB machNumbers 透传错误: %#v", lastMachs)
	}
	if lastMode != coreinterp.ModeLinear {
		t.Fatalf("LoadMultiPRB mode 透传错误: %q", lastMode)
	}
}

// 多 PRB 分支：所有 filePath 为空时应记录"无有效文件路径"错误。
func TestRestoreInterpolator_MultiPRB_AllEmptyPaths(t *testing.T) {
	mgr := newConfigTestManager(t)
	loader := &mockInterpolatorLoader{}
	mgr.SetInterpolatorLoader(loader)
	mgr.SaveConfigRaw(json.RawMessage(`{
		"useMultiPrb": true,
		"multiPrb": {"files":[{"filePath":""},{"filePath":""}], "machNumbers":[0.3,0.6]}
	}`))
	mgr.RestoreInterpolatorFromPersistedConfig()
	waitForRestoreSettled(t, mgr, func() bool { return mgr.InterpolatorRestoreErr() != "" })
	_, _, _, _, _, _, _, multiPRBCalls := loader.snapshot()
	if multiPRBCalls != 0 {
		t.Fatalf("空 filePath 列表不应调用 LoadMultiPRB")
	}
}

// 多 PRB 分支：底层 loader 报错应记录错误且不注入。
func TestRestoreInterpolator_MultiPRB_Failure(t *testing.T) {
	mgr := newConfigTestManager(t)
	loader := &mockInterpolatorLoader{multiPRBErr: errors.New("bad prb")}
	mgr.SetInterpolatorLoader(loader)
	mgr.SaveConfigRaw(json.RawMessage(`{
		"useMultiPrb": true,
		"multiPrb": {"files":[{"filePath":"a.prb"}], "machNumbers":[0.5]}
	}`))
	mgr.RestoreInterpolatorFromPersistedConfig()
	waitForRestoreSettled(t, mgr, func() bool { return mgr.InterpolatorRestoreErr() != "" })
	if mgr.Interpolator() != nil {
		t.Fatalf("多 PRB 失败路径不应注入插值器")
	}
}

// 优先级：interpolationAlgorithm="new" + CSV 应优先于 prbFile。
func TestRestoreInterpolator_Priority_CSVOverPRB(t *testing.T) {
	mgr := newConfigTestManager(t)
	loader := &mockInterpolatorLoader{}
	mgr.SetInterpolatorLoader(loader)
	mgr.SaveConfigRaw(json.RawMessage(`{
		"interpolationAlgorithm":"new",
		"calibrationCsvFile":{"filePath":"/tmp/cal.csv"},
		"prbFile":{"filePath":"/tmp/legacy.prb"}
	}`))
	mgr.RestoreInterpolatorFromPersistedConfig()
	waitForRestoreSettled(t, mgr, func() bool { return mgr.Interpolator() != nil })
	_, _, _, _, _, prbCalls, csvCalls, _ := loader.snapshot()
	if prbCalls != 0 {
		t.Fatalf("CSV 优先时不应调用 LoadPRB")
	}
	if csvCalls != 1 {
		t.Fatalf("CSV 优先时应调用 LoadFiveHoleCSV 一次，实际 %d", csvCalls)
	}
}

// 优先级：useMultiPrb=true 应优先于单 prbFile。
func TestRestoreInterpolator_Priority_MultiOverSingle(t *testing.T) {
	mgr := newConfigTestManager(t)
	loader := &mockInterpolatorLoader{}
	mgr.SetInterpolatorLoader(loader)
	mgr.SaveConfigRaw(json.RawMessage(`{
		"useMultiPrb": true,
		"multiPrb": {"files":[{"filePath":"a.prb"}], "machNumbers":[0.5]},
		"prbFile":{"filePath":"/tmp/legacy.prb"}
	}`))
	mgr.RestoreInterpolatorFromPersistedConfig()
	waitForRestoreSettled(t, mgr, func() bool { return loader.totalCalls() > 0 })
	_, _, _, _, _, prbCalls, _, _ := loader.snapshot()
	if prbCalls != 0 {
		t.Fatalf("多 PRB 优先时不应调用 LoadPRB")
	}
}

// 损坏的 JSON 应记录解析错误且不调用 loader。
func TestRestoreInterpolator_InvalidJSON(t *testing.T) {
	mgr := newConfigTestManager(t)
	loader := &mockInterpolatorLoader{}
	mgr.SetInterpolatorLoader(loader)
	mgr.SaveConfigRaw(json.RawMessage(`{ this is not json`))
	mgr.RestoreInterpolatorFromPersistedConfig()
	waitForRestoreSettled(t, mgr, func() bool { return mgr.InterpolatorRestoreErr() != "" })
	if loader.totalCalls() != 0 {
		t.Fatalf("JSON 解析失败不应触发 loader 调用")
	}
}

// ===== runLoaderWithTimeout 单元测试 =====

func TestRunLoaderWithTimeout_Success(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	interp, err, timedOut := runLoaderWithTimeout(ctx, func() (coreinterp.Interpolator, error) {
		return &mockInterpolator{tag: "ok"}, nil
	})
	if timedOut {
		t.Fatalf("快速完成不应被标记为超时")
	}
	if err != nil {
		t.Fatalf("不应返回错误，实际 %v", err)
	}
	if interp == nil {
		t.Fatalf("应返回插值器")
	}
}

func TestRunLoaderWithTimeout_ErrorPassthrough(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	want := errors.New("io fail")
	interp, err, timedOut := runLoaderWithTimeout(ctx, func() (coreinterp.Interpolator, error) {
		return nil, want
	})
	if timedOut {
		t.Fatalf("立即错误返回不应标记超时")
	}
	if !errors.Is(err, want) {
		t.Fatalf("应透传原始错误，实际 %v", err)
	}
	if interp != nil {
		t.Fatalf("失败路径不应返回插值器")
	}
}

func TestRunLoaderWithTimeout_TimeoutTriggersFlag(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	interp, err, timedOut := runLoaderWithTimeout(ctx, func() (coreinterp.Interpolator, error) {
		time.Sleep(200 * time.Millisecond)
		return &mockInterpolator{tag: "late"}, nil
	})
	if !timedOut {
		t.Fatalf("阻塞超过 ctx 截止时间应标记为 timedOut")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("应返回 ctx.Err()=DeadlineExceeded，实际 %v", err)
	}
	if interp != nil {
		t.Fatalf("超时时不应返回插值器，避免主流程误用陈旧结果")
	}
}

// ParseAndStartTraversal 矩形布局字段映射测试：验证 YMin/YMax 被正确传递。
// 此前因遗漏 YMin/YMax 导致矩形遍历只生成 X 轴单排点，总点数仅为 len(xs)。
func TestParseAndStartTraversal_RectangleLayout(t *testing.T) {
	mgr := newConfigTestManager(t)

	// 构造包含完整 rectangle 布局配置的 JSON
	// X: 0~2 步长 1 → 3 个点；Y: 0~4 步长 1 → 5 个点；期望总点数 3*5=15
	raw := json.RawMessage(`{
		"name": "rect-test",
		"layout": {
			"pattern": "rectangle",
			"snakeOrder": false,
			"rectangle": {
				"xMin": 0, "xMax": 2, "xStepSegments": [{"start":0,"end":2,"step":1}],
				"yMin": 0, "yMax": 4, "yStepSegments": [{"start":0,"end":4,"step":1}]
			}
		},
		"channels": {
			"probeChannels": [
				{"name":"P1","role":"fiveHole.p1","channel":{"deviceId":"sim-1","channelIndex":0},"enabled":true}
			]
		},
		"dwellTimeMs": 100,
		"samplesPerPoint": 1,
		"savePath": "/tmp/test.csv"
	}`)

	taskID, err := mgr.ParseAndStartTraversal(raw)
	if err != nil {
		t.Fatalf("ParseAndStartTraversal 失败: %v", err)
	}
	if taskID == "" {
		t.Fatalf("taskID 不应为空")
	}

	// 通过 Status() 获取当前状态，验证总点数
	status := mgr.Status()
	wantTotal := 15 // 3 * 5
	if status.TotalPoints != wantTotal {
		t.Fatalf("矩形布局总点数错误: 期望 %d, 实际 %d (遗漏 YMin/YMax 时会是 %d)",
			wantTotal, status.TotalPoints, 3)
	}

	// 进一步验证 config.Path 长度与 TotalPoints 一致
	// config 是私有字段，通过 Status 中的 TotalPoints 已足够验证
	// 额外验证：停止遍历以清理资源
	if err := mgr.Stop(); err != nil {
		t.Logf("Stop 返回错误 (可忽略): %v", err)
	}
}

func TestParseConfig_PreservesLogicalMotionTargetNames(t *testing.T) {
	mgr := newConfigTestManager(t)
	raw := json.RawMessage(`{
		"name": "sector-axis-binding-test",
		"layout": {
			"pattern": "sector",
			"sector": {
				"radiusMin": 100, "radiusMax": 100,
				"radialStepSegments": [{"start":100,"end":100,"step":10}],
				"angleStart": 0, "angleEnd": 10,
				"angularStepSegments": [{"start":0,"end":10,"step":10}]
			}
		},
		"channels": {
			"probeChannels": [
				{"name":"P1","role":"fiveHole.p1","channel":{"deviceId":"sim-1","channelIndex":0},"enabled":true}
			],
			"motionAxes": [
				{"name":"X","controllerId":"mc-1","axis":"Z"},
				{"name":"Y","controllerId":"mc-1","axis":"U"}
			]
		},
		"dwellTimeMs": 100,
		"samplesPerPoint": 1
	}`)

	config, err := mgr.ParseConfig(raw)
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}
	if len(config.MotionAxes) != 2 {
		t.Fatalf("expected 2 motion bindings, got %#v", config.MotionAxes)
	}
	if config.MotionAxes[0].Name != "X" || config.MotionAxes[0].Axis != "Z" ||
		config.MotionAxes[1].Name != "Y" || config.MotionAxes[1].Axis != "U" {
		t.Fatalf("logical target names were not preserved: %#v", config.MotionAxes)
	}
	if config.LayoutPattern != "sector" {
		t.Fatalf("layout pattern was not preserved: %q", config.LayoutPattern)
	}
}

func TestParseAndStartTraversal_RejectsSectorAxesThatAreNotZeroed(t *testing.T) {
	motionAccess := &mockMotionAccess{statuses: []motion.ControllerStatus{{
		ID: "mc-1", Connected: true,
		Axes: []motion.AxisStatus{
			{Name: motion.AxisX, Position: 5},
			{Name: motion.AxisY, Position: 0},
		},
	}}}
	mgr := NewTraversalManager(&mockLatestDataReader{}, motionAccess, &mockTraversalPointSink{}, newMockTraversalResultStore(), storage.NewFileCheckpointStore())
	raw := json.RawMessage(`{
		"name": "sector-origin-test",
		"layout": {
			"pattern": "sector",
			"sector": {
				"radiusMin": 100, "radiusMax": 100,
				"radialStepSegments": [{"start":100,"end":100,"step":10}],
				"angleStart": 0, "angleEnd": 10,
				"angularStepSegments": [{"start":0,"end":10,"step":10}]
			}
		},
		"channels": {
			"probeChannels": [
				{"name":"P1","role":"fiveHole.p1","channel":{"deviceId":"sim-1","channelIndex":0},"enabled":true}
			],
			"motionAxes": [
				{"name":"X","controllerId":"mc-1","axis":"X"},
				{"name":"Y","controllerId":"mc-1","axis":"Y"}
			]
		},
		"dwellTimeMs": 100,
		"samplesPerPoint": 1
	}`)

	_, err := mgr.ParseAndStartTraversal(raw)
	if err == nil || !strings.Contains(err.Error(), "axis X current position 5") {
		t.Fatalf("expected hard start rejection for non-zero sector axis, got %v", err)
	}
	if len(motionAccess.moveToCalls) != 0 {
		t.Fatalf("origin validation failure must not send motion commands: %#v", motionAccess.moveToCalls)
	}
}

// ParseAndStartTraversal 线型布局字段映射测试：验证简化后的 line 模式
// 仅消费 startX/endX/xStepSegments 三个字段，Y 恒为 0，不再消费 startY/endY/yStepSegments。
// 同时验证残留的旧字段（startY/endY/yStepSegments）会被 JSON 反序列化静默忽略，不报错。
// X: -10~10 步长 5 → 5 个点（-10, -5, 0, 5, 10）；Y 固定 0 → 单行 5 点
func TestParseAndStartTraversal_LineLayout(t *testing.T) {
	mgr := newConfigTestManager(t)

	// 故意在 JSON 中携带旧字段 startY/endY/yStepSegments，验证它们被静默忽略
	raw := json.RawMessage(`{
		"name": "line-test",
		"layout": {
			"pattern": "line",
			"snakeOrder": true,
			"primaryAxis": "y",
			"line": {
				"startX": -10, "endX": 10,
				"xStepSegments": [{"start":-10,"end":10,"step":5}],
				"startY": 99, "endY": 99, "yStepSegments": [{"start":0,"end":1,"step":1}]
			}
		},
		"channels": {
			"probeChannels": [
				{"name":"P1","role":"fiveHole.p1","channel":{"deviceId":"sim-1","channelIndex":0},"enabled":true}
			]
		},
		"dwellTimeMs": 100,
		"samplesPerPoint": 1,
		"savePath": "/tmp/test.csv"
	}`)

	taskID, err := mgr.ParseAndStartTraversal(raw)
	if err != nil {
		t.Fatalf("ParseAndStartTraversal 失败: %v", err)
	}
	if taskID == "" {
		t.Fatalf("taskID 不应为空")
	}

	// 验证总点数：5 个 X 步进点 × 1 行（Y=0）= 5
	status := mgr.Status()
	wantTotal := 5
	if status.TotalPoints != wantTotal {
		t.Fatalf("线型布局总点数错误: 期望 %d, 实际 %d (若仍消费 Y 字段会是 10)",
			wantTotal, status.TotalPoints)
	}

	// 清理
	if err := mgr.Stop(); err != nil {
		t.Logf("Stop 返回错误 (可忽略): %v", err)
	}
}

// buildAutoStartTestRaw 构造 ParseAndStartTraversal 测试用 JSON。
// 复用 line 布局最小配置：deviceId=sim-1，5 个点，dwell 100ms。
// 避免每个测试重复构造大段 JSON。
func buildAutoStartTestRaw() json.RawMessage {
	return json.RawMessage(`{
		"name": "auto-start-test",
		"layout": {
			"pattern": "line",
			"line": {
				"startX": 0, "endX": 4,
				"xStepSegments": [{"start":0,"end":4,"step":1}]
			}
		},
		"channels": {
			"probeChannels": [
				{"name":"P1","role":"fiveHole.p1","channel":{"deviceId":"sim-1","channelIndex":0},"enabled":true}
			]
		},
		"dwellTimeMs": 100,
		"samplesPerPoint": 1,
		"savePath": "/tmp/test.csv"
	}`)
}

// TestParseAndStartTraversal_NilAcquisitionController 端口未注入时不调用 StartAcquisition。
//
// 测试前置：
//   - newConfigTestManager 创建 mgr，未调用 SetAcquisitionController
//
// 测试步骤：
//   - 调用 mgr.ParseAndStartTraversal(raw)
//
// 期待结果：
//   - 不返回 error（向后兼容，loop 正常启动）
//   - taskID 非空
//   - 未触发任何采集启动调用（无端口可调）
func TestParseAndStartTraversal_NilAcquisitionController(t *testing.T) {
	mgr := newConfigTestManager(t)

	taskID, err := mgr.ParseAndStartTraversal(buildAutoStartTestRaw())
	if err != nil {
		t.Fatalf("ParseAndStartTraversal 失败（端口 nil 应保持向后兼容）: %v", err)
	}
	if taskID == "" {
		t.Fatalf("taskID 不应为空")
	}

	if err := mgr.Stop(); err != nil {
		t.Logf("Stop 返回错误 (可忽略): %v", err)
	}
}

// TestParseAndStartTraversal_AlreadyAcquiring 设备已采集时跳过 StartAcquisition。
//
// 测试前置：
//   - mgr 注入 mockAcquisitionController，acquiring["sim-1"]=true
//
// 测试步骤：
//   - 调用 mgr.ParseAndStartTraversal(raw)
//
// 期待结果：
//   - 不返回 error
//   - mock.startCalls 长度 = 0（已采集，跳过主动启动）
func TestParseAndStartTraversal_AlreadyAcquiring(t *testing.T) {
	mgr := newConfigTestManager(t)
	ctrl := &mockAcquisitionController{
		connected: map[string]bool{"sim-1": true},
		acquiring: map[string]bool{"sim-1": true},
	}
	mgr.SetAcquisitionController(ctrl)

	if _, err := mgr.ParseAndStartTraversal(buildAutoStartTestRaw()); err != nil {
		t.Fatalf("ParseAndStartTraversal 失败: %v", err)
	}

	if len(ctrl.startCalls) != 0 {
		t.Errorf("StartAcquisition 不应被调用（设备已采集），实际调用 %d 次: %v",
			len(ctrl.startCalls), ctrl.startCalls)
	}

	if err := mgr.Stop(); err != nil {
		t.Logf("Stop 返回错误 (可忽略): %v", err)
	}
}

// TestParseAndStartTraversal_NotAcquiringRejectsStart 设备未采集时拒绝启动。
//
// 测试前置：
//   - mgr 注入 mockAcquisitionController，acquiring["sim-1"]=false，startErr=nil
//
// 测试步骤：
//   - 调用 mgr.ParseAndStartTraversal(raw)
//
// 期待结果：
//   - 返回 error，提示先开始采集
//   - 不发送 StartAcquisition
func TestParseAndStartTraversal_NotAcquiringRejectsStart(t *testing.T) {
	mgr := newConfigTestManager(t)
	ctrl := &mockAcquisitionController{
		connected: map[string]bool{"sim-1": true},
		acquiring: map[string]bool{"sim-1": false},
		startErr:  nil,
	}
	mgr.SetAcquisitionController(ctrl)

	_, err := mgr.ParseAndStartTraversal(buildAutoStartTestRaw())
	if err == nil || !contains(err.Error(), "not acquiring") {
		t.Fatalf("expected not acquiring error, got %v", err)
	}

	if len(ctrl.startCalls) != 0 {
		t.Errorf("StartAcquisition must not be called, got %v", ctrl.startCalls)
	}
}

// TestParseAndStartTraversal_DoesNotSendAcquisitionCommand 设备未采集时不发送启动命令。
//
// 测试前置：
//   - mgr 注入 mockAcquisitionController，acquiring["sim-1"]=false，startErr=设备拒绝
//
// 测试步骤：
//   - 调用 mgr.ParseAndStartTraversal(raw)
//
// 期待结果：
//   - 返回 error，提示设备未采集
//   - mock.startCalls 为空
//   - mgr.Status().State != "running"（Start 未执行，状态保持 Idle）
func TestParseAndStartTraversal_DoesNotSendAcquisitionCommand(t *testing.T) {
	mgr := newConfigTestManager(t)
	ctrl := &mockAcquisitionController{
		connected: map[string]bool{"sim-1": true},
		acquiring: map[string]bool{"sim-1": false},
		startErr:  errors.New("device rejected start command"),
	}
	mgr.SetAcquisitionController(ctrl)

	_, err := mgr.ParseAndStartTraversal(buildAutoStartTestRaw())
	if err == nil {
		t.Fatalf("期望返回 error（启动采集失败），实际 nil")
	}
	if !contains(err.Error(), "not acquiring") {
		t.Errorf("error should report not acquiring, got: %v", err)
	}

	if len(ctrl.startCalls) != 0 {
		t.Errorf("StartAcquisition must not be called, got %v", ctrl.startCalls)
	}

	// 状态验证：采集态校验在 m.Start 前失败，状态应保持 Idle。
	status := mgr.Status()
	if status.State == "running" {
		t.Errorf("失败后状态不应为 running（Start 未执行），实际: %v", status.State)
	}
}

// TestMotionCompletePollMsForValidation_MatchesRuntimePoll 校验
// traversal_config.go 中 motionCompletePollMsForValidation 与 traversal.go 中
// motionCompletePoll 两个常量保持一致。
//
// 设计动机：motionCompletePollMsForValidation 是 validateMotionSafetyConfig
// 校验 NoProgressTimeoutMs 下限的硬编码常量（避免循环引用不能直接引用 motionCompletePoll），
// 如果未来修改了运行时轮询间隔却忘记同步校验常量，校验规则会与运行时去抖逻辑脱钩
// （例如看门狗比轮询还快误触发，或校验通过但运行时仍频繁触发）。
//
// 测试前置：两个常量分别定义在 traversal_config.go / traversal.go，package usecase 内可直接访问。
// 测试步骤：将 motionCompletePoll 转换为毫秒，与 motionCompletePollMsForValidation 比较。
// 期待结果：两者相等，确保校验侧与运行时侧语义一致。
func TestMotionCompletePollMsForValidation_MatchesRuntimePoll(t *testing.T) {
	runtimePollMs := int(motionCompletePoll / time.Millisecond)
	if runtimePollMs != motionCompletePollMsForValidation {
		t.Errorf("motionCompletePollMsForValidation 与运行时常量脱钩: 校验侧=%dms, 运行时侧=%dms"+
			"（请同步修改 traversal_config.go 中的 motionCompletePollMsForValidation）",
			motionCompletePollMsForValidation, runtimePollMs)
	}
}
