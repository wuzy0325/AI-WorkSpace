package calibration

import (
	"reflect"
	"sync"
	"testing"
)

// =====================================================================
// spec Task 10 测试：moveToPoint 七孔分支 + MoveToPointWithOrder 双坐标优先级
// + processPoint 七孔 RealtimeCallback/PrevRegion/PrevSector 注入与状态更新
// =====================================================================
//
// 测试前置：
//   - mockSevenHoleAlgorithm：最小 Algorithm 实现，Type()=TypeSevenHole，
//     AcquireDataWithConfig 返回构造好的 *SevenHoleDataPoint（带 Region/Sector），
//     便于测试 prevRegion/prevSector 注入与更新逻辑，避免依赖完整通道读取链路。
//   - captureEventPublisher：捕获 OnRealtime/OnProgress/OnComplete 调用，
//     用于断言 RealtimeCallback 包装层是否正确推送 RealtimeEvent。
//   - 复用既有 fakeCalibrationRuntime（automatic_five_hole_test.go）记录 moves。

// mockSevenHoleAlgorithm 七孔算法测试替身。
//
// 设计要点：
//   - Type() 返回 TypeSevenHole，触发 moveToPoint / processPoint 的七孔分支
//   - AcquireDataWithConfig 返回 nextDataPoint 字段值，测试可注入不同 Region/Sector
//     验证 prevRegion/prevSector 滞回状态更新逻辑
//   - 同时记录最后一次收到的 config，便于断言 RealtimeCallback/PrevRegion/PrevSector 注入
type mockSevenHoleAlgorithm struct {
	nextDataPoint DataPoint
	lastConfig    Config
	configMu      sync.Mutex
}

func (m *mockSevenHoleAlgorithm) Type() CalibrationType { return TypeSevenHole }

func (m *mockSevenHoleAlgorithm) AcquireData(point CalPoint, _ ChannelValueReader, _ int) (DataPoint, error) {
	return m.nextDataPoint, nil
}

func (m *mockSevenHoleAlgorithm) AcquireDataWithConfig(point CalPoint, _ ChannelValueReader, cfg Config, _ func() bool, _ func(current, total int)) (DataPoint, error) {
	m.configMu.Lock()
	m.lastConfig = cfg
	m.configMu.Unlock()
	return m.nextDataPoint, nil
}

func (m *mockSevenHoleAlgorithm) ValidateConfig(Config) error { return nil }

func (m *mockSevenHoleAlgorithm) getLastConfig() Config {
	m.configMu.Lock()
	defer m.configMu.Unlock()
	return m.lastConfig
}

// captureEventPublisher 捕获 EventPublisher 调用，用于断言七孔实时事件推送。
type captureEventPublisher struct {
	mu             sync.Mutex
	realtime       []RealtimeEvent
	progress       []ProgressEvent
	completions    []CompleteEvent
	regionChanges  []RegionChangedEvent
}

func (p *captureEventPublisher) OnProgress(e ProgressEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.progress = append(p.progress, e)
}

func (p *captureEventPublisher) OnComplete(e CompleteEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.completions = append(p.completions, e)
}

func (p *captureEventPublisher) OnRealtime(e RealtimeEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.realtime = append(p.realtime, e)
}

func (p *captureEventPublisher) OnRegionChanged(e RegionChangedEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.regionChanges = append(p.regionChanges, e)
}

func (p *captureEventPublisher) realtimeSnapshot() []RealtimeEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]RealtimeEvent, len(p.realtime))
	copy(out, p.realtime)
	return out
}

func (p *captureEventPublisher) regionChangesSnapshot() []RegionChangedEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]RegionChangedEvent, len(p.regionChanges))
	copy(out, p.regionChanges)
	return out
}

// =====================================================================
// P0 - moveToPoint 七孔分支验收测试
// =====================================================================

// TestMoveToPointSevenHole 【P0】验证七孔分支调用 MoveToPointWithOrder 严格按 α→β 顺序
//
// 测试前置：
//   - 七孔算法 mock，Type()=TypeSevenHole
//   - MotionAxes 配置 β 在前 α 在后，验证 MoveToPointWithOrder 仍按传入的 ["α","β"] 顺序
//   - CalPoint.Coordinates 同时含 α/β，MotionCoordinates 为 nil（内区点场景）
//
// 期待结果：moves == ["α=1", "β=2"]（按 axisOrder 顺序，非 MotionAxes 配置顺序）
func TestMoveToPointSevenHole(t *testing.T) {
	rt := &fakeCalibrationRuntime{}
	config := Config{
		TaskID: "task-sevenhole-move",
		Type:   string(TypeSevenHole),
		MotionAxes: []MotionAxisConfig{
			{Name: "β", ControllerID: "motion-1", Axis: "Y"},
			{Name: "α", ControllerID: "motion-1", Axis: "X"},
		},
		Points: []CalPoint{{ID: 1, Coordinates: map[string]float64{"β": 2, "α": 1}}},
	}
	engine := NewAutomaticCalibration(config, nil, rt, nil, nil)
	algo := &mockSevenHoleAlgorithm{
		nextDataPoint: &SevenHoleDataPoint{PointID: 1, Region: "inner", Sector: 7},
	}

	if err := engine.Start(algo); err != nil {
		t.Fatalf("start seven-hole calibration: %v", err)
	}

	expected := []string{"α=1", "β=2"}
	if !reflect.DeepEqual(rt.moves, expected) {
		t.Fatalf("七孔 moveToPoint 应按 [α,β] 顺序移动，期望 %v，实际 %v", expected, rt.moves)
	}
}

// TestMoveToPointDualCoordinates_PrefersMotionCoordinates 【P0】验证外区点优先用 MotionCoordinates
//
// 测试前置：构造外区点，Coordinates={θ,φ}, MotionCoordinates={α',β'}
//
// 期待结果：moves == ["α=-28.56", "β=17.5"]（取 MotionCoordinates 中的 α/β 值，而非 Coordinates 的 θ/φ）
func TestMoveToPointDualCoordinates_PrefersMotionCoordinates(t *testing.T) {
	rt := &fakeCalibrationRuntime{}
	config := Config{
		TaskID: "task-sevenhole-dual",
		Type:   string(TypeSevenHole),
		MotionAxes: []MotionAxisConfig{
			{Name: "α", ControllerID: "motion-1", Axis: "X"},
			{Name: "β", ControllerID: "motion-1", Axis: "Y"},
		},
		Points: []CalPoint{{
			ID:               1,
			Coordinates:      map[string]float64{"θ": 35, "φ": 180},
			MotionCoordinates: map[string]float64{"α": -28.56, "β": 17.5},
			Region:           "outer",
			Sector:           4,
		}},
	}
	engine := NewAutomaticCalibration(config, nil, rt, nil, nil)
	algo := &mockSevenHoleAlgorithm{
		nextDataPoint: &SevenHoleDataPoint{PointID: 1, Region: "outer", Sector: 4},
	}

	if err := engine.Start(algo); err != nil {
		t.Fatalf("start: %v", err)
	}

	expected := []string{"α=-28.56", "β=17.5"}
	if !reflect.DeepEqual(rt.moves, expected) {
		t.Fatalf("外区点应优先 MotionCoordinates，期望 %v，实际 %v", expected, rt.moves)
	}
}

// TestMoveToPointDualCoordinates_NilFallbackToCoordinates 【P0】验证 MotionCoordinates 为 nil 时回退
//
// 测试前置：构造七孔内区点，MotionCoordinates=nil，Coordinates={α,β}
//
// 期待结果：moves == ["α=5", "β=10"]（回退到 Coordinates）
func TestMoveToPointDualCoordinates_NilFallbackToCoordinates(t *testing.T) {
	rt := &fakeCalibrationRuntime{}
	config := Config{
		TaskID: "task-sevenhole-fallback",
		Type:   string(TypeSevenHole),
		MotionAxes: []MotionAxisConfig{
			{Name: "α", ControllerID: "motion-1", Axis: "X"},
			{Name: "β", ControllerID: "motion-1", Axis: "Y"},
		},
		Points: []CalPoint{{
			ID:          1,
			Coordinates: map[string]float64{"α": 5, "β": 10},
			Region:      "inner",
			Sector:      7,
		}},
	}
	engine := NewAutomaticCalibration(config, nil, rt, nil, nil)
	algo := &mockSevenHoleAlgorithm{
		nextDataPoint: &SevenHoleDataPoint{PointID: 1, Region: "inner", Sector: 7},
	}

	if err := engine.Start(algo); err != nil {
		t.Fatalf("start: %v", err)
	}

	expected := []string{"α=5", "β=10"}
	if !reflect.DeepEqual(rt.moves, expected) {
		t.Fatalf("MotionCoordinates=nil 应回退 Coordinates，期望 %v，实际 %v", expected, rt.moves)
	}
}

// TestMoveToPointFiveHole_Regression 【P0】回归测试：五孔分支保持原行为
//
// 测试前置：五孔算法（真实 NewFiveHoleAlgorithm），MotionAxes=[β,α]
//
// 期待结果：moves == ["α=1", "β=2"]（五孔仍走 MoveToPointWithOrder，未受七孔分支影响）
func TestMoveToPointFiveHole_Regression(t *testing.T) {
	config := completeFiveHoleConfig()
	config.MotionAxes = []MotionAxisConfig{
		{Name: "β", ControllerID: "motion-1", Axis: "Y"},
		{Name: "α", ControllerID: "motion-1", Axis: "X"},
	}
	config.Points = []CalPoint{{ID: 1, Coordinates: map[string]float64{"β": 2, "α": 1}}}
	rt := &fakeCalibrationRuntime{values: completeFiveHoleValues()}

	engine := NewAutomaticCalibration(config, nil, rt, nil, nil)
	if err := engine.Start(NewFiveHoleAlgorithm()); err != nil {
		t.Fatalf("start five-hole: %v", err)
	}

	expected := []string{"α=1", "β=2"}
	if !reflect.DeepEqual(rt.moves, expected) {
		t.Fatalf("五孔分支应保持原行为，期望 %v，实际 %v", expected, rt.moves)
	}
}

// =====================================================================
// P1 - processPoint 状态注入测试
// =====================================================================

// TestProcessSevenHole_InjectsRealtimeCallback 【P1】验证 RealtimeCallback 注入
//
// 测试前置：注入 captureEventPublisher，运行单点七孔校准
//
// 期待结果：
//   - 算法收到的 Config.RealtimeCallback 非 nil
//   - 通过 RealtimeCallback 推送一次实时数据后，captureEventPublisher.realtime 中出现对应事件
func TestProcessSevenHole_InjectsRealtimeCallback(t *testing.T) {
	rt := &fakeCalibrationRuntime{}
	publisher := &captureEventPublisher{}
	config := Config{
		TaskID: "task-sevenhole-rt-cb",
		Type:   string(TypeSevenHole),
		MotionAxes: []MotionAxisConfig{
			{Name: "α", ControllerID: "motion-1", Axis: "X"},
			{Name: "β", ControllerID: "motion-1", Axis: "Y"},
		},
		Points: []CalPoint{{ID: 1, Coordinates: map[string]float64{"α": 5, "β": 10}}},
	}
	engine := NewAutomaticCalibration(config, publisher, rt, nil, nil)
	algo := &mockSevenHoleAlgorithm{
		nextDataPoint: &SevenHoleDataPoint{PointID: 1, Region: "inner", Sector: 7},
	}

	if err := engine.Start(algo); err != nil {
		t.Fatalf("start: %v", err)
	}

	// 算法收到的 Config.RealtimeCallback 应非 nil
	lastCfg := algo.getLastConfig()
	if lastCfg.RealtimeCallback == nil {
		t.Fatal("七孔 processPoint 应注入 RealtimeCallback 到 Config，实际为 nil")
	}

	// 主动调用一次 RealtimeCallback 验证包装层正确推送 RealtimeEvent
	lastCfg.RealtimeCallback(
		SevenHoleRawData{P1: 100, P7: 500},
		SevenHoleCoefficients{Kalpha: 0.1, K0: 0.5},
		"inner", 7,
	)
	events := publisher.realtimeSnapshot()
	if len(events) != 1 {
		t.Fatalf("RealtimeCallback 应推送 1 个事件，实际 %d", len(events))
	}
	evt := events[0]
	if evt.Type != TypeSevenHole {
		t.Errorf("事件 Type 应为 seven-hole，实际 %s", evt.Type)
	}
	if evt.SevenHoleRaw == nil || evt.SevenHoleRaw.P7 != 500 {
		t.Errorf("SevenHoleRaw 应被填充，实际 %v", evt.SevenHoleRaw)
	}
	if evt.SevenHoleCoefficients == nil || evt.SevenHoleCoefficients.K0 != 0.5 {
		t.Errorf("SevenHoleCoefficients 应被填充，实际 %v", evt.SevenHoleCoefficients)
	}
	if evt.Point == nil || evt.Point.ID != 1 {
		t.Errorf("Point 应被填充为当前点位，实际 %v", evt.Point)
	}
}

// TestProcessSevenHole_InjectsPrevRegionSector 【P1】验证 PrevRegion/PrevSector 跨点传递
//
// 测试前置：构造 2 点七孔校准，第 1 点返回 outer/3，第 2 点返回 outer/4
//
// 期待结果：第 2 点算法收到的 Config.PrevRegion=="outer", PrevSector==3
// （即第 1 点数据点的 Region/Sector，证明 prevRegion/prevSector 正确更新并注入下一点）
func TestProcessSevenHole_InjectsPrevRegionSector(t *testing.T) {
	rt := &fakeCalibrationRuntime{}
	config := Config{
		TaskID: "task-sevenhole-prev",
		Type:   string(TypeSevenHole),
		MotionAxes: []MotionAxisConfig{
			{Name: "α", ControllerID: "motion-1", Axis: "X"},
			{Name: "β", ControllerID: "motion-1", Axis: "Y"},
		},
		Points: []CalPoint{
			{ID: 1, Coordinates: map[string]float64{"α": 0, "β": 0}},
			{ID: 2, Coordinates: map[string]float64{"α": 5, "β": 5}},
		},
	}
	engine := NewAutomaticCalibration(config, nil, rt, nil, nil)
	algo := &mockSevenHoleAlgorithm{
		nextDataPoint: &SevenHoleDataPoint{PointID: 1, Region: "outer", Sector: 3},
	}

	// onDataPoint 在每点采集完成 + 状态更新后调用，此时 prevRegion/prevSector 已写入。
	// 在第 1 点完成后切换 nextDataPoint，让第 2 点返回不同的 Region/Sector。
	engine.onDataPoint = func(dp DataPoint) {
		algo.nextDataPoint = &SevenHoleDataPoint{PointID: 2, Region: "outer", Sector: 4}
	}

	if err := engine.Start(algo); err != nil {
		t.Fatalf("start: %v", err)
	}

	// algo.lastConfig 是最后一次 AcquireDataWithConfig 调用收到的 Config，
	// 即第 2 点采集时的 Config——PrevRegion/PrevSector 应反映第 1 点的 outer/3
	lastCfg := algo.getLastConfig()
	if lastCfg.PrevRegion != "outer" {
		t.Errorf("第 2 点 PrevRegion 应为 outer（第 1 点 Region），实际 %q", lastCfg.PrevRegion)
	}
	if lastCfg.PrevSector != 3 {
		t.Errorf("第 2 点 PrevSector 应为 3（第 1 点 Sector），实际 %d", lastCfg.PrevSector)
	}

	// 同时验证引擎最终状态反映第 2 点（outer/4）
	if engine.prevRegion != "outer" {
		t.Errorf("引擎最终 prevRegion 应为 outer，实际 %q", engine.prevRegion)
	}
	if engine.prevSector != 4 {
		t.Errorf("引擎最终 prevSector 应为 4（第 2 点 Sector），实际 %d", engine.prevSector)
	}
}

// TestProcessSevenHole_UpdatesPrevRegionSector 【P1】验证采集完后 prevRegion/prevSector 更新
//
// 测试前置：单点七孔校准，nextDataPoint.Region="outer", Sector=2
//
// 期待结果：Start 完成后 engine.prevRegion=="outer", engine.prevSector==2
func TestProcessSevenHole_UpdatesPrevRegionSector(t *testing.T) {
	rt := &fakeCalibrationRuntime{}
	config := Config{
		TaskID: "task-sevenhole-update",
		Type:   string(TypeSevenHole),
		MotionAxes: []MotionAxisConfig{
			{Name: "α", ControllerID: "motion-1", Axis: "X"},
			{Name: "β", ControllerID: "motion-1", Axis: "Y"},
		},
		Points: []CalPoint{{ID: 1, Coordinates: map[string]float64{"α": 5, "β": 10}}},
	}
	engine := NewAutomaticCalibration(config, nil, rt, nil, nil)
	algo := &mockSevenHoleAlgorithm{
		nextDataPoint: &SevenHoleDataPoint{PointID: 1, Region: "outer", Sector: 2},
	}

	if err := engine.Start(algo); err != nil {
		t.Fatalf("start: %v", err)
	}

	if engine.prevRegion != "outer" {
		t.Errorf("采集后 prevRegion 应为 outer，实际 %q", engine.prevRegion)
	}
	if engine.prevSector != 2 {
		t.Errorf("采集后 prevSector 应为 2，实际 %d", engine.prevSector)
	}
}

// TestProcessSevenHole_FirstPointZeroPrevState 【P1】验证首点前 prevRegion/prevSector 为零值
//
// 测试前置：单点七孔校准
//
// 期待结果：算法收到的 Config.PrevRegion=="", PrevSector==0（首点跳过滞回）
func TestProcessSevenHole_FirstPointZeroPrevState(t *testing.T) {
	rt := &fakeCalibrationRuntime{}
	config := Config{
		TaskID: "task-sevenhole-first",
		Type:   string(TypeSevenHole),
		MotionAxes: []MotionAxisConfig{
			{Name: "α", ControllerID: "motion-1", Axis: "X"},
			{Name: "β", ControllerID: "motion-1", Axis: "Y"},
		},
		Points: []CalPoint{{ID: 1, Coordinates: map[string]float64{"α": 0, "β": 0}}},
	}
	engine := NewAutomaticCalibration(config, nil, rt, nil, nil)
	algo := &mockSevenHoleAlgorithm{
		nextDataPoint: &SevenHoleDataPoint{PointID: 1, Region: "inner", Sector: 7},
	}

	if err := engine.Start(algo); err != nil {
		t.Fatalf("start: %v", err)
	}

	lastCfg := algo.getLastConfig()
	if lastCfg.PrevRegion != "" {
		t.Errorf("首点 PrevRegion 应为空串，实际 %q", lastCfg.PrevRegion)
	}
	if lastCfg.PrevSector != 0 {
		t.Errorf("首点 PrevSector 应为 0，实际 %d", lastCfg.PrevSector)
	}
}

// =====================================================================
// P2 - makeSevenHoleRealtimeCallback 单元测试
// =====================================================================

// TestMakeSevenHoleRealtimeCallback_NilPublisher 【P2】验证 eventPublisher 为 nil 时返回 nil
//
// 期待结果：返回 nil（算法侧跳过推送，spec Task 5 约定）
func TestMakeSevenHoleRealtimeCallback_NilPublisher(t *testing.T) {
	engine := NewAutomaticCalibration(Config{}, nil, nil, nil, nil)
	cb := engine.makeSevenHoleRealtimeCallback(CalPoint{ID: 1})
	if cb != nil {
		t.Errorf("eventPublisher 为 nil 时应返回 nil，实际 %v", cb)
	}
}

// TestMakeSevenHoleRealtimeCallback_PublishesEvent 【P2】验证回调正确推送 RealtimeEvent
//
// 期待结果：调用 cb 后 captureEventPublisher.realtime 中出现 TypeSevenHole 事件，
// 字段 SevenHoleRaw/SevenHoleCoefficients/Point 正确填充
func TestMakeSevenHoleRealtimeCallback_PublishesEvent(t *testing.T) {
	publisher := &captureEventPublisher{}
	engine := NewAutomaticCalibration(Config{}, publisher, nil, nil, nil)
	point := CalPoint{ID: 42, Coordinates: map[string]float64{"α": 1, "β": 2}, Region: "outer", Sector: 3}

	cb := engine.makeSevenHoleRealtimeCallback(point)
	if cb == nil {
		t.Fatal("eventPublisher 非 nil 时应返回有效回调")
	}

	cb(SevenHoleRawData{P7: 999}, SevenHoleCoefficients{K0: 0.42}, "outer", 3)

	events := publisher.realtimeSnapshot()
	if len(events) != 1 {
		t.Fatalf("应推送 1 个事件，实际 %d", len(events))
	}
	evt := events[0]
	if evt.Type != TypeSevenHole {
		t.Errorf("Type 应为 seven-hole，实际 %s", evt.Type)
	}
	if evt.SevenHoleRaw == nil || evt.SevenHoleRaw.P7 != 999 {
		t.Errorf("SevenHoleRaw.P7 应为 999，实际 %v", evt.SevenHoleRaw)
	}
	if evt.SevenHoleCoefficients == nil || evt.SevenHoleCoefficients.K0 != 0.42 {
		t.Errorf("SevenHoleCoefficients.K0 应为 0.42，实际 %v", evt.SevenHoleCoefficients)
	}
	if evt.Point == nil || evt.Point.ID != 42 {
		t.Errorf("Point.ID 应为 42，实际 %v", evt.Point)
	}
}

// =====================================================================
// P2 - lookupAxisPosition 单元测试
// =====================================================================

// TestLookupAxisPosition_PrefersMotionCoordinates 【P2】MotionCoordinates 优先
func TestLookupAxisPosition_PrefersMotionCoordinates(t *testing.T) {
	point := CalPoint{
		Coordinates:       map[string]float64{"α": 1, "β": 2},
		MotionCoordinates: map[string]float64{"α": 10, "β": 20},
	}
	if v, ok := lookupAxisPosition(point, "α"); !ok || v != 10 {
		t.Errorf("应优先 MotionCoordinates[α]=10，实际 v=%v ok=%v", v, ok)
	}
	if v, ok := lookupAxisPosition(point, "β"); !ok || v != 20 {
		t.Errorf("应优先 MotionCoordinates[β]=20，实际 v=%v ok=%v", v, ok)
	}
}

// TestLookupAxisPosition_FallbackToCoordinates 【P2】MotionCoordinates 缺轴时回退 Coordinates
func TestLookupAxisPosition_FallbackToCoordinates(t *testing.T) {
	point := CalPoint{
		Coordinates:       map[string]float64{"α": 1, "β": 2, "θ": 30},
		MotionCoordinates: map[string]float64{"α": 10, "β": 20}, // 不含 θ
	}
	// α/β 走 MotionCoordinates
	if v, ok := lookupAxisPosition(point, "α"); !ok || v != 10 {
		t.Errorf("α 应走 MotionCoordinates=10，实际 v=%v ok=%v", v, ok)
	}
	// θ 走 Coordinates 回退
	if v, ok := lookupAxisPosition(point, "θ"); !ok || v != 30 {
		t.Errorf("θ 应回退 Coordinates=30，实际 v=%v ok=%v", v, ok)
	}
}

// TestLookupAxisPosition_NilMotionCoordinates 【P2】MotionCoordinates 为 nil 时回退
func TestLookupAxisPosition_NilMotionCoordinates(t *testing.T) {
	point := CalPoint{Coordinates: map[string]float64{"α": 5, "β": 10}}
	if v, ok := lookupAxisPosition(point, "α"); !ok || v != 5 {
		t.Errorf("MotionCoordinates=nil 时 α 应回退 5，实际 v=%v ok=%v", v, ok)
	}
}

// TestLookupAxisPosition_NotFound 【P2】两处都没有时返回 ok=false
func TestLookupAxisPosition_NotFound(t *testing.T) {
	point := CalPoint{Coordinates: map[string]float64{"α": 5}}
	if _, ok := lookupAxisPosition(point, "β"); ok {
		t.Error("β 不在 Coordinates 也不在 MotionCoordinates 时应返回 ok=false")
	}
}

// =====================================================================
// spec Task 11 测试：OnRegionChanged 七孔分区变更事件
// =====================================================================
//
// 测试覆盖：
//   - 首点必推送（PrevRegion=nil, PrevSector=nil, BoundaryFlag="first"）
//   - inner↔outer 切换推送（BoundaryFlag="inner-outer"）
//   - 同 outer 扇区切换推送（BoundaryFlag="sector-switch"）
//   - region 与 sector 均不变时不推送（避免噪声）
//   - 多点序列事件序列断言
//   - nil publisher 不 panic
//   - GetCurrentRegion/GetCurrentSector 反映最新值
//   - 五孔类型不触发 OnRegionChanged

// TestRegionChangedFirstPoint 【P0】首点必推送一次，PrevRegion/PrevSector=nil，BoundaryFlag="first"
func TestRegionChangedFirstPoint(t *testing.T) {
	rt := &fakeCalibrationRuntime{}
	config := Config{
		TaskID: "task-region-first",
		Type:   string(TypeSevenHole),
		MotionAxes: []MotionAxisConfig{
			{Name: "α", ControllerID: "motion-1", Axis: "X"},
			{Name: "β", ControllerID: "motion-1", Axis: "Y"},
		},
		Points: []CalPoint{{ID: 1, Coordinates: map[string]float64{"α": 0, "β": 0}}},
	}
	publisher := &captureEventPublisher{}
	engine := NewAutomaticCalibration(config, publisher, rt, nil, nil)
	algo := &mockSevenHoleAlgorithm{
		nextDataPoint: &SevenHoleDataPoint{PointID: 1, Region: "inner", Sector: 7},
	}

	if err := engine.Start(algo); err != nil {
		t.Fatalf("start: %v", err)
	}

	events := publisher.regionChangesSnapshot()
	if len(events) != 1 {
		t.Fatalf("首点应推送 1 个 RegionChangedEvent，实际 %d", len(events))
	}
	evt := events[0]
	if evt.Region != "inner" || evt.Sector != 7 {
		t.Errorf("Region/Sector 应为 inner/7，实际 %s/%d", evt.Region, evt.Sector)
	}
	if evt.PrevRegion != nil {
		t.Errorf("首点 PrevRegion 应为 nil（JSON null），实际 %v", *evt.PrevRegion)
	}
	if evt.PrevSector != nil {
		t.Errorf("首点 PrevSector 应为 nil（JSON null），实际 %v", *evt.PrevSector)
	}
	if evt.BoundaryFlag != "first" {
		t.Errorf("首点 BoundaryFlag 应为 first，实际 %s", evt.BoundaryFlag)
	}
	if evt.PointIndex != 0 || evt.TotalPoints != 1 {
		t.Errorf("PointIndex/TotalPoints 应为 0/1，实际 %d/%d", evt.PointIndex, evt.TotalPoints)
	}
}

// TestRegionChangedSwitch_InnerToOuter 【P0】内→外切换推送，BoundaryFlag="inner-outer"
func TestRegionChangedSwitch_InnerToOuter(t *testing.T) {
	rt := &fakeCalibrationRuntime{}
	config := Config{
		TaskID: "task-region-io",
		Type:   string(TypeSevenHole),
		MotionAxes: []MotionAxisConfig{
			{Name: "α", ControllerID: "motion-1", Axis: "X"},
			{Name: "β", ControllerID: "motion-1", Axis: "Y"},
		},
		Points: []CalPoint{
			{ID: 1, Coordinates: map[string]float64{"α": 0, "β": 0}},
			{ID: 2, Coordinates: map[string]float64{"α": 5, "β": 5}},
		},
	}
	publisher := &captureEventPublisher{}
	engine := NewAutomaticCalibration(config, publisher, rt, nil, nil)
	algo := &mockSevenHoleAlgorithm{
		nextDataPoint: &SevenHoleDataPoint{PointID: 1, Region: "inner", Sector: 7},
	}
	// 第 1 点完成后切到 outer/3，触发 inner→outer 切换
	engine.onDataPoint = func(dp DataPoint) {
		algo.nextDataPoint = &SevenHoleDataPoint{PointID: 2, Region: "outer", Sector: 3}
	}

	if err := engine.Start(algo); err != nil {
		t.Fatalf("start: %v", err)
	}

	events := publisher.regionChangesSnapshot()
	if len(events) != 2 {
		t.Fatalf("应推送 2 个事件（首点 + 切换），实际 %d", len(events))
	}

	// 第 1 个事件：首点 inner/7
	if events[0].BoundaryFlag != "first" {
		t.Errorf("事件 0 BoundaryFlag 应为 first，实际 %s", events[0].BoundaryFlag)
	}

	// 第 2 个事件：inner→outer 切换
	evt := events[1]
	if evt.Region != "outer" || evt.Sector != 3 {
		t.Errorf("事件 1 Region/Sector 应为 outer/3，实际 %s/%d", evt.Region, evt.Sector)
	}
	if evt.PrevRegion == nil || *evt.PrevRegion != "inner" {
		t.Errorf("事件 1 PrevRegion 应指向 inner，实际 %v", evt.PrevRegion)
	}
	if evt.PrevSector == nil || *evt.PrevSector != 7 {
		t.Errorf("事件 1 PrevSector 应指向 7，实际 %v", evt.PrevSector)
	}
	if evt.BoundaryFlag != "inner-outer" {
		t.Errorf("事件 1 BoundaryFlag 应为 inner-outer，实际 %s", evt.BoundaryFlag)
	}
	if evt.PointIndex != 1 {
		t.Errorf("事件 1 PointIndex 应为 1，实际 %d", evt.PointIndex)
	}
}

// TestRegionChangedSwitch_Sector 【P0】同 outer 扇区切换推送，BoundaryFlag="sector-switch"
func TestRegionChangedSwitch_Sector(t *testing.T) {
	rt := &fakeCalibrationRuntime{}
	config := Config{
		TaskID: "task-region-sec",
		Type:   string(TypeSevenHole),
		MotionAxes: []MotionAxisConfig{
			{Name: "α", ControllerID: "motion-1", Axis: "X"},
			{Name: "β", ControllerID: "motion-1", Axis: "Y"},
		},
		Points: []CalPoint{
			{ID: 1, Coordinates: map[string]float64{"α": 0, "β": 0}},
			{ID: 2, Coordinates: map[string]float64{"α": 5, "β": 5}},
		},
	}
	publisher := &captureEventPublisher{}
	engine := NewAutomaticCalibration(config, publisher, rt, nil, nil)
	algo := &mockSevenHoleAlgorithm{
		nextDataPoint: &SevenHoleDataPoint{PointID: 1, Region: "outer", Sector: 3},
	}
	// 第 1 点完成后切到 outer/4，同 outer 扇区切换
	engine.onDataPoint = func(dp DataPoint) {
		algo.nextDataPoint = &SevenHoleDataPoint{PointID: 2, Region: "outer", Sector: 4}
	}

	if err := engine.Start(algo); err != nil {
		t.Fatalf("start: %v", err)
	}

	events := publisher.regionChangesSnapshot()
	if len(events) != 2 {
		t.Fatalf("应推送 2 个事件（首点 + 扇区切换），实际 %d", len(events))
	}

	evt := events[1]
	if evt.Region != "outer" || evt.Sector != 4 {
		t.Errorf("事件 1 Region/Sector 应为 outer/4，实际 %s/%d", evt.Region, evt.Sector)
	}
	if evt.PrevRegion == nil || *evt.PrevRegion != "outer" {
		t.Errorf("事件 1 PrevRegion 应指向 outer，实际 %v", evt.PrevRegion)
	}
	if evt.PrevSector == nil || *evt.PrevSector != 3 {
		t.Errorf("事件 1 PrevSector 应指向 3，实际 %v", evt.PrevSector)
	}
	if evt.BoundaryFlag != "sector-switch" {
		t.Errorf("事件 1 BoundaryFlag 应为 sector-switch，实际 %s", evt.BoundaryFlag)
	}
}

// TestRegionChangedNoSwitch 【P0】region 与 sector 均不变时不推送（避免噪声）
func TestRegionChangedNoSwitch(t *testing.T) {
	rt := &fakeCalibrationRuntime{}
	config := Config{
		TaskID: "task-region-noop",
		Type:   string(TypeSevenHole),
		MotionAxes: []MotionAxisConfig{
			{Name: "α", ControllerID: "motion-1", Axis: "X"},
			{Name: "β", ControllerID: "motion-1", Axis: "Y"},
		},
		Points: []CalPoint{
			{ID: 1, Coordinates: map[string]float64{"α": 0, "β": 0}},
			{ID: 2, Coordinates: map[string]float64{"α": 5, "β": 5}},
		},
	}
	publisher := &captureEventPublisher{}
	engine := NewAutomaticCalibration(config, publisher, rt, nil, nil)
	algo := &mockSevenHoleAlgorithm{
		nextDataPoint: &SevenHoleDataPoint{PointID: 1, Region: "outer", Sector: 3},
	}
	// 第 2 点保持 outer/3 不变
	engine.onDataPoint = func(dp DataPoint) {
		algo.nextDataPoint = &SevenHoleDataPoint{PointID: 2, Region: "outer", Sector: 3}
	}

	if err := engine.Start(algo); err != nil {
		t.Fatalf("start: %v", err)
	}

	events := publisher.regionChangesSnapshot()
	if len(events) != 1 {
		t.Fatalf("不变时应只推送首点 1 个事件，实际 %d", len(events))
	}
	if events[0].BoundaryFlag != "first" {
		t.Errorf("事件 0 BoundaryFlag 应为 first，实际 %s", events[0].BoundaryFlag)
	}
}

// TestRegionChangedSequence 【P1】多点序列事件序列断言
//
// 测试前置：4 点七孔校准，序列为 inner/7 → outer/3 → outer/3 → outer/5
//
// 期待结果：推送 3 个事件——首点 first、第 2 点 inner-outer、第 4 点 sector-switch
// （第 3 点 outer/3 与上一时刻相同，不推送）
func TestRegionChangedSequence(t *testing.T) {
	rt := &fakeCalibrationRuntime{}
	config := Config{
		TaskID: "task-region-seq",
		Type:   string(TypeSevenHole),
		MotionAxes: []MotionAxisConfig{
			{Name: "α", ControllerID: "motion-1", Axis: "X"},
			{Name: "β", ControllerID: "motion-1", Axis: "Y"},
		},
		Points: []CalPoint{
			{ID: 1, Coordinates: map[string]float64{"α": 0, "β": 0}},
			{ID: 2, Coordinates: map[string]float64{"α": 5, "β": 5}},
			{ID: 3, Coordinates: map[string]float64{"α": 10, "β": 10}},
			{ID: 4, Coordinates: map[string]float64{"α": 15, "β": 15}},
		},
	}
	publisher := &captureEventPublisher{}
	engine := NewAutomaticCalibration(config, publisher, rt, nil, nil)

	// 序列：inner/7 → outer/3 → outer/3 → outer/5
	seq := []*SevenHoleDataPoint{
		{PointID: 1, Region: "inner", Sector: 7},
		{PointID: 2, Region: "outer", Sector: 3},
		{PointID: 3, Region: "outer", Sector: 3}, // 与第 2 点相同，不推送
		{PointID: 4, Region: "outer", Sector: 5},
	}
	algo := &mockSevenHoleAlgorithm{nextDataPoint: seq[0]}
	idx := 0
	engine.onDataPoint = func(dp DataPoint) {
		idx++
		if idx < len(seq) {
			algo.nextDataPoint = seq[idx]
		}
	}

	if err := engine.Start(algo); err != nil {
		t.Fatalf("start: %v", err)
	}

	events := publisher.regionChangesSnapshot()
	if len(events) != 3 {
		t.Fatalf("应推送 3 个事件（首点 + 切换 ×2，第 3 点跳过），实际 %d", len(events))
	}

	wantFlags := []string{"first", "inner-outer", "sector-switch"}
	for i, want := range wantFlags {
		if events[i].BoundaryFlag != want {
			t.Errorf("事件 %d BoundaryFlag 应为 %s，实际 %s", i, want, events[i].BoundaryFlag)
		}
	}

	// 第 3 个事件（sector-switch）的 Prev 应指向 outer/3
	last := events[2]
	if last.PrevRegion == nil || *last.PrevRegion != "outer" {
		t.Errorf("事件 2 PrevRegion 应指向 outer，实际 %v", last.PrevRegion)
	}
	if last.PrevSector == nil || *last.PrevSector != 3 {
		t.Errorf("事件 2 PrevSector 应指向 3，实际 %v", last.PrevSector)
	}
	if last.Region != "outer" || last.Sector != 5 {
		t.Errorf("事件 2 Region/Sector 应为 outer/5，实际 %s/%d", last.Region, last.Sector)
	}
}

// TestRegionChangedNilPublisher 【P1】nil publisher 不 panic
func TestRegionChangedNilPublisher(t *testing.T) {
	rt := &fakeCalibrationRuntime{}
	config := Config{
		TaskID: "task-region-nil",
		Type:   string(TypeSevenHole),
		MotionAxes: []MotionAxisConfig{
			{Name: "α", ControllerID: "motion-1", Axis: "X"},
			{Name: "β", ControllerID: "motion-1", Axis: "Y"},
		},
		Points: []CalPoint{{ID: 1, Coordinates: map[string]float64{"α": 0, "β": 0}}},
	}
	engine := NewAutomaticCalibration(config, nil, rt, nil, nil)
	algo := &mockSevenHoleAlgorithm{
		nextDataPoint: &SevenHoleDataPoint{PointID: 1, Region: "inner", Sector: 7},
	}

	// 不应 panic
	if err := engine.Start(algo); err != nil {
		t.Fatalf("start: %v", err)
	}
}

// TestGetCurrentRegionSector 【P1】GetCurrentRegion/GetCurrentSector 反映最新值
func TestGetCurrentRegionSector(t *testing.T) {
	rt := &fakeCalibrationRuntime{}
	config := Config{
		TaskID: "task-region-get",
		Type:   string(TypeSevenHole),
		MotionAxes: []MotionAxisConfig{
			{Name: "α", ControllerID: "motion-1", Axis: "X"},
			{Name: "β", ControllerID: "motion-1", Axis: "Y"},
		},
		Points: []CalPoint{
			{ID: 1, Coordinates: map[string]float64{"α": 0, "β": 0}},
			{ID: 2, Coordinates: map[string]float64{"α": 5, "β": 5}},
		},
	}
	engine := NewAutomaticCalibration(config, nil, rt, nil, nil)
	algo := &mockSevenHoleAlgorithm{
		nextDataPoint: &SevenHoleDataPoint{PointID: 1, Region: "inner", Sector: 7},
	}
	engine.onDataPoint = func(dp DataPoint) {
		algo.nextDataPoint = &SevenHoleDataPoint{PointID: 2, Region: "outer", Sector: 4}
	}

	if err := engine.Start(algo); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Start 完成后最新值为第 2 点 outer/4
	if r := engine.GetCurrentRegion(); r != "outer" {
		t.Errorf("GetCurrentRegion 应为 outer，实际 %q", r)
	}
	if s := engine.GetCurrentSector(); s != 4 {
		t.Errorf("GetCurrentSector 应为 4，实际 %d", s)
	}
}

// TestRegionChangedFiveHoleNotTriggered 【P1】五孔类型不触发 OnRegionChanged
//
// 测试前置：五孔校准，captureEventPublisher 捕获事件
//
// 期待结果：regionChanges 切片为空（五孔走 moveToPoint 五孔分支，processPoint 不进七孔 if 块）
func TestRegionChangedFiveHoleNotTriggered(t *testing.T) {
	rt := &fakeCalibrationRuntime{values: completeFiveHoleValues()}
	config := completeFiveHoleConfig()
	config.Points = []CalPoint{
		{ID: 1, Coordinates: map[string]float64{"α": 0, "β": 0}},
		{ID: 2, Coordinates: map[string]float64{"α": 5, "β": 0}},
	}
	publisher := &captureEventPublisher{}
	engine := NewAutomaticCalibration(config, publisher, rt, nil, nil)

	if err := engine.Start(NewFiveHoleAlgorithm()); err != nil {
		t.Fatalf("start: %v", err)
	}

	if events := publisher.regionChangesSnapshot(); len(events) != 0 {
		t.Errorf("五孔类型不应推送 OnRegionChanged 事件，实际推送 %d 个", len(events))
	}
}

// TestExtractSevenHoleRegionSector 【P2】extractSevenHoleRegionSector 类型断言
func TestExtractSevenHoleRegionSector(t *testing.T) {
	// 七孔数据点
	sh := &SevenHoleDataPoint{Region: "outer", Sector: 3}
	r, s, ok := extractSevenHoleRegionSector(sh)
	if !ok || r != "outer" || s != 3 {
		t.Errorf("七孔点应返回 outer/3/true，实际 %s/%d/%v", r, s, ok)
	}

	// 非七孔数据点
	fp := &FiveHoleDataPoint{}
	r, s, ok = extractSevenHoleRegionSector(fp)
	if ok || r != "" || s != 0 {
		t.Errorf("五孔点应返回 /0/false，实际 %s/%d/%v", r, s, ok)
	}
}
