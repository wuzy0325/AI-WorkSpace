package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	"wind-daq/services/api-go/internal/adapters/storage"
)

// ===== InterpolatorLoader mock 实现 =====

// mockInterpolator 是测试用的最小 Interpolator 实现，只断言是否被注入到 manager 中。
type mockInterpolator struct {
	tag string
}

func (m *mockInterpolator) IsLoaded() bool                          { return true }
func (m *mockInterpolator) GetValidRange() coreinterp.PrbValidRange { return coreinterp.PrbValidRange{} }
func (m *mockInterpolator) Calculate(_ coreinterp.InterpolationInput) (coreinterp.InterpolationResult, error) {
	return coreinterp.InterpolationResult{}, nil
}

// mockInterpolatorLoader 是 ports.InterpolatorLoader 的可控 mock，
// 用于覆盖 CSV / 单 PRB / 多 PRB 三条加载分支以及成功 / 失败 / 阻塞三种场景。
type mockInterpolatorLoader struct {
	// 触发条件：调用入参回填
	lastPRB        string
	lastCSV        string
	lastMultiPRB   []string
	lastMode       coreinterp.MultiPrbInterpolationMode
	lastMachs      []float64
	prbCalls       int
	csvCalls       int
	multiPRBCalls  int

	// 行为开关
	prbErr      error
	csvErr      error
	multiPRBErr error
	// blockFor 模拟磁盘阻塞，常用于测试 ctx 超时分支
	blockFor time.Duration
}

func (l *mockInterpolatorLoader) LoadPRB(filePath string) (coreinterp.Interpolator, error) {
	l.prbCalls++
	l.lastPRB = filePath
	if l.blockFor > 0 {
		time.Sleep(l.blockFor)
	}
	if l.prbErr != nil {
		return nil, l.prbErr
	}
	return &mockInterpolator{tag: "prb:" + filePath}, nil
}

func (l *mockInterpolatorLoader) LoadFiveHoleCSV(filePath string) (coreinterp.Interpolator, error) {
	l.csvCalls++
	l.lastCSV = filePath
	if l.blockFor > 0 {
		time.Sleep(l.blockFor)
	}
	if l.csvErr != nil {
		return nil, l.csvErr
	}
	return &mockInterpolator{tag: "csv:" + filePath}, nil
}

func (l *mockInterpolatorLoader) LoadMultiPRB(filePaths []string, machNumbers []float64, mode coreinterp.MultiPrbInterpolationMode) (coreinterp.Interpolator, error) {
	l.multiPRBCalls++
	l.lastMultiPRB = filePaths
	l.lastMachs = machNumbers
	l.lastMode = mode
	if l.blockFor > 0 {
		time.Sleep(l.blockFor)
	}
	if l.multiPRBErr != nil {
		return nil, l.multiPRBErr
	}
	return &mockInterpolator{tag: "multi"}, nil
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
	if loader.prbCalls+loader.csvCalls+loader.multiPRBCalls != 0 {
		t.Fatalf("空配置不应触发任何 loader 调用，实际 prb=%d csv=%d multi=%d", loader.prbCalls, loader.csvCalls, loader.multiPRBCalls)
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
	if loader.lastPRB != "/tmp/test.prb" {
		t.Fatalf("LoadPRB 应传入 /tmp/test.prb，实际 %q", loader.lastPRB)
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
	waitForRestoreSettled(t, mgr, func() bool { return loader.csvCalls > 0 })
	if loader.lastCSV != "/tmp/cal.csv" {
		t.Fatalf("LoadFiveHoleCSV 应传入 /tmp/cal.csv，实际 %q", loader.lastCSV)
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
	waitForRestoreSettled(t, mgr, func() bool { return loader.multiPRBCalls > 0 })

	if len(loader.lastMultiPRB) != 2 || loader.lastMultiPRB[0] != "a.prb" || loader.lastMultiPRB[1] != "b.prb" {
		t.Fatalf("LoadMultiPRB filePaths 透传错误: %#v", loader.lastMultiPRB)
	}
	if len(loader.lastMachs) != 2 || loader.lastMachs[0] != 0.3 || loader.lastMachs[1] != 0.6 {
		t.Fatalf("LoadMultiPRB machNumbers 透传错误: %#v", loader.lastMachs)
	}
	if loader.lastMode != coreinterp.ModeLinear {
		t.Fatalf("LoadMultiPRB mode 透传错误: %q", loader.lastMode)
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
	if loader.multiPRBCalls != 0 {
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
	if loader.prbCalls != 0 {
		t.Fatalf("CSV 优先时不应调用 LoadPRB")
	}
	if loader.csvCalls != 1 {
		t.Fatalf("CSV 优先时应调用 LoadFiveHoleCSV 一次，实际 %d", loader.csvCalls)
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
	waitForRestoreSettled(t, mgr, func() bool { return loader.multiPRBCalls > 0 })
	if loader.prbCalls != 0 {
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
	if loader.prbCalls+loader.csvCalls+loader.multiPRBCalls != 0 {
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
