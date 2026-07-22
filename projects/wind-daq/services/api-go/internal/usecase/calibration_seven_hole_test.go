// Package usecase — 七孔校准 usecase 层测试（spec Task 9）
//
// 测试覆盖：
//   - createAlgorithm 工厂分支：TypeSevenHole → *SevenHoleAlgorithm
//   - PreviewSevenHolePoints：完整模式 673 点 / 数据集模式 481 点
//   - 七孔 CSV writer 路由：region+sector 分流、文件命名、懒加载、flush
//
// 测试不依赖真实文件 I/O——通过 fakeSevenHoleWriterFactory 桩注入，
// 与 calibration_savecsv_test.go 的 fakeCalibrationCsvWriter 风格一致。
package usecase

import (
	"errors"
	"log/slog"
	"sync"
	"testing"

	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/ports"
)

// ==================== createAlgorithm 工厂分支测试 ====================

// TestCreateAlgorithmSevenHole 验证 createAlgorithm 工厂为 TypeSevenHole 返回 *SevenHoleAlgorithm
//
// 测试前置：CalibrationManager 已注入空 csvWriter（避免 nil panic）
// 测试步骤：调用 createAlgorithm 传入 TypeSevenHole config
// 期待结果：返回 *calibration.SevenHoleAlgorithm 实例，无错误
func TestCreateAlgorithmSevenHole(t *testing.T) {
	manager := NewCalibrationManager(nil, nil, nil, nil)
	config := calibration.Config{
		Type: string(calibration.TypeSevenHole),
	}

	algo, err := manager.createAlgorithm(config)
	if err != nil {
		t.Fatalf("createAlgorithm(TypeSevenHole) 返回错误: %v", err)
	}
	if algo == nil {
		t.Fatal("createAlgorithm(TypeSevenHole) 返回 nil")
	}

	// 类型断言：必须是 *SevenHoleAlgorithm
	shAlgo, ok := algo.(*calibration.SevenHoleAlgorithm)
	if !ok {
		t.Fatalf("createAlgorithm(TypeSevenHole) 返回类型 %T，期望 *SevenHoleAlgorithm", algo)
	}

	// Type() 必须返回 TypeSevenHole
	if got := shAlgo.Type(); got != calibration.TypeSevenHole {
		t.Fatalf("Type() = %q，期望 %q", got, calibration.TypeSevenHole)
	}
}

// TestCreateAlgorithmSevenHole_TypeCheck 验证返回的算法类型字段正确
//
// 测试前置：CalibrationManager 已构造
// 测试步骤：createAlgorithm 返回后调用 Type() 方法
// 期待结果：Type() == TypeSevenHole
func TestCreateAlgorithmSevenHole_TypeCheck(t *testing.T) {
	manager := NewCalibrationManager(nil, nil, nil, nil)
	algo, err := manager.createAlgorithm(calibration.Config{Type: string(calibration.TypeSevenHole)})
	if err != nil {
		t.Fatalf("createAlgorithm 失败: %v", err)
	}
	if algo.Type() != calibration.TypeSevenHole {
		t.Fatalf("算法类型 = %q，期望 %q", algo.Type(), calibration.TypeSevenHole)
	}
}

// TestCreateAlgorithmUnknownType 验证未知类型返回错误（边界用例）
//
// 测试前置：CalibrationManager 已构造
// 测试步骤：传入未知类型 "unknown"
// 期待结果：返回非 nil 错误
func TestCreateAlgorithmUnknownType(t *testing.T) {
	manager := NewCalibrationManager(nil, nil, nil, nil)
	_, err := manager.createAlgorithm(calibration.Config{Type: "unknown"})
	if err == nil {
		t.Fatal("未知类型应返回错误，实际返回 nil")
	}
}

// ==================== PreviewSevenHolePoints 测试 ====================

// sevenHoleBuildFullConfig 构建完整模式默认配置（673 点）
// 与 core/calibration/seven_hole_points_test.go 的 sevenHoleBuildFullConfig 对齐
func sevenHoleBuildFullConfig() calibration.SevenHoleConfig {
	return calibration.SevenHoleConfig{
		Mode:           calibration.SevenHoleModeFull,
		InnerAlphaMin:  -30.0,
		InnerAlphaMax:  30.0,
		InnerAlphaStep: 5.0,
		InnerBetaMin:   -30.0,
		InnerBetaMax:   30.0,
		InnerBetaStep:  5.0,
		OuterThetaMin:  30.0,
		OuterThetaMax:  60.0,
		OuterThetaStep: 5.0,
		OuterPhiMin:    0.0,
		OuterPhiMax:    355.0,
		OuterPhiStep:   5.0,
		Serpentine:     false,
	}
}

// TestPreviewSevenHolePoints_FullMode 验证完整模式返回 673 点（内区 169 + 外区 504）
//
// 测试前置：默认完整模式配置（α/β ±30° 步长 5°，θ 30-60° 步长 5°，φ 0-355° 步长 5°）
// 测试步骤：调用 PreviewSevenHolePoints
// 期待结果：TotalCount=673, InnerCount=169, OuterCount=504
func TestPreviewSevenHolePoints_FullMode(t *testing.T) {
	manager := NewCalibrationManager(nil, nil, nil, nil)
	result, err := manager.PreviewSevenHolePoints(sevenHoleBuildFullConfig())
	if err != nil {
		t.Fatalf("PreviewSevenHolePoints(full) 返回错误: %v", err)
	}
	if result.TotalCount != 673 {
		t.Errorf("TotalCount = %d，期望 673", result.TotalCount)
	}
	if result.InnerCount != 169 {
		t.Errorf("InnerCount = %d，期望 169", result.InnerCount)
	}
	if result.OuterCount != 504 {
		t.Errorf("OuterCount = %d，期望 504", result.OuterCount)
	}
	if len(result.Points) != result.TotalCount {
		t.Errorf("len(Points) = %d，期望 %d", len(result.Points), result.TotalCount)
	}
}

// TestPreviewSevenHolePoints_DatasetMode 验证数据集模式返回 481 点（内区 169 + 外区 312）
//
// 测试前置：数据集模式配置（θ 取 {30,35,40,45}°，每扇区 φ 跨 60° 步长 5°）
// 测试步骤：调用 PreviewSevenHolePoints
// 期待结果：TotalCount=481, InnerCount=169, OuterCount=312
func TestPreviewSevenHolePoints_DatasetMode(t *testing.T) {
	manager := NewCalibrationManager(nil, nil, nil, nil)
	cfg := sevenHoleBuildFullConfig()
	cfg.Mode = calibration.SevenHoleModeDataset

	result, err := manager.PreviewSevenHolePoints(cfg)
	if err != nil {
		t.Fatalf("PreviewSevenHolePoints(dataset) 返回错误: %v", err)
	}
	if result.TotalCount != 481 {
		t.Errorf("TotalCount = %d，期望 481", result.TotalCount)
	}
	if result.InnerCount != 169 {
		t.Errorf("InnerCount = %d，期望 169", result.InnerCount)
	}
	if result.OuterCount != 312 {
		t.Errorf("OuterCount = %d，期望 312", result.OuterCount)
	}
}

// TestPreviewSevenHolePoints_InvalidConfig 验证非法配置返回错误
//
// 测试前置：步长为 0 的非法配置
// 测试步骤：调用 PreviewSevenHolePoints
// 期待结果：返回非 nil 错误，结果为零值
func TestPreviewSevenHolePoints_InvalidConfig(t *testing.T) {
	manager := NewCalibrationManager(nil, nil, nil, nil)
	cfg := sevenHoleBuildFullConfig()
	cfg.InnerAlphaStep = 0 // 非法步长

	result, err := manager.PreviewSevenHolePoints(cfg)
	if err == nil {
		t.Fatal("非法步长应返回错误，实际返回 nil")
	}
	if result.TotalCount != 0 {
		t.Errorf("错误时 TotalCount = %d，期望 0", result.TotalCount)
	}
}

// TestPreviewSevenHolePoints_DoesNotCreateCSVWriter 验证 PreviewSevenHolePoints 不创建 CSV writer
//
// 测试前置：manager 未注入任何 csvWriter/factory
// 测试步骤：调用 PreviewSevenHolePoints
// 期待结果：方法正常返回，不 panic，csvWriter/sevenHoleWriters 仍为 nil
func TestPreviewSevenHolePoints_DoesNotCreateCSVWriter(t *testing.T) {
	manager := NewCalibrationManager(nil, nil, nil, nil)
	_, err := manager.PreviewSevenHolePoints(sevenHoleBuildFullConfig())
	if err != nil {
		t.Fatalf("PreviewSevenHolePoints 失败: %v", err)
	}
	if manager.csvWriter != nil {
		t.Error("PreviewSevenHolePoints 不应初始化 csvWriter")
	}
	if manager.sevenHoleWriters != nil {
		t.Error("PreviewSevenHolePoints 不应初始化 sevenHoleWriters")
	}
}

// ==================== 七孔 CSV writer 路由辅助函数测试 ====================

// TestSevenHoleWriterKey 验证缓存 key 生成逻辑
//
// 测试前置：无
// 测试步骤：覆盖内区/外区 1-6 共 7 种组合
// 期待结果：内区 → "inner"；外区 n → "outer_<n>"
func TestSevenHoleWriterKey(t *testing.T) {
	cases := []struct {
		region string
		sector int
		want   string
	}{
		{"inner", 7, "inner"},
		{"inner", 0, "inner"}, // 内区 sector 字段无意义，统一 "inner"
		{"outer", 1, "outer_1"},
		{"outer", 2, "outer_2"},
		{"outer", 3, "outer_3"},
		{"outer", 4, "outer_4"},
		{"outer", 5, "outer_5"},
		{"outer", 6, "outer_6"},
	}
	for _, c := range cases {
		got := sevenHoleWriterKey(c.region, c.sector)
		if got != c.want {
			t.Errorf("sevenHoleWriterKey(%q, %d) = %q，期望 %q", c.region, c.sector, got, c.want)
		}
	}
}

// TestSevenHoleFileSuffix 验证文件命名后缀
//
// 测试前置：无
// 测试步骤：覆盖内区/外区 1-6
// 期待结果：内区 → "_小角度区"；外区 n → "_大角度<n>区"
func TestSevenHoleFileSuffix(t *testing.T) {
	cases := []struct {
		region string
		sector int
		want   string
	}{
		{"inner", 7, "_小角度区"},
		{"outer", 1, "_大角度1区"},
		{"outer", 2, "_大角度2区"},
		{"outer", 3, "_大角度3区"},
		{"outer", 6, "_大角度6区"},
	}
	for _, c := range cases {
		got := sevenHoleFileSuffix(c.region, c.sector)
		if got != c.want {
			t.Errorf("sevenHoleFileSuffix(%q, %d) = %q，期望 %q", c.region, c.sector, got, c.want)
		}
	}
}

// ==================== 七孔 CSV writer 路由集成测试 ====================

// fakeSevenHoleWriter 七孔 CSV writer 桩，记录所有调用供测试断言
type fakeSevenHoleWriter struct {
	path      string
	schema    calibration.CsvSchema
	points    []calibration.DataPoint
	flushed   bool
	flushMu   sync.Mutex
	appendCnt int
	flushErr  error // Flush 返回的错误（spec Task 20 测试 double-check 丢弃路径用）
}

func (w *fakeSevenHoleWriter) Initialize(config calibration.Config) error {
	w.path = config.SavePath
	return nil
}

func (w *fakeSevenHoleWriter) AppendPoint(point calibration.DataPoint) error {
	w.points = append(w.points, point)
	w.appendCnt++
	return nil
}

func (w *fakeSevenHoleWriter) Flush() error {
	w.flushMu.Lock()
	defer w.flushMu.Unlock()
	w.flushed = true
	return w.flushErr
}

func (w *fakeSevenHoleWriter) Path() string { return w.path }

// fakeSevenHoleWriterFactory 工厂桩，记录所有创建的 writer 供测试断言
type fakeSevenHoleWriterFactory struct {
	mu      sync.Mutex
	writers map[string]*fakeSevenHoleWriter // key = path
}

func newFakeSevenHoleWriterFactory() *fakeSevenHoleWriterFactory {
	return &fakeSevenHoleWriterFactory{writers: make(map[string]*fakeSevenHoleWriter)}
}

func (f *fakeSevenHoleWriterFactory) NewWriter(path string, schema calibration.CsvSchema) (ports.CalibrationCsvWriter, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w := &fakeSevenHoleWriter{path: path, schema: schema}
	f.writers[path] = w
	return w, nil
}

func (f *fakeSevenHoleWriterFactory) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.writers)
}

func (f *fakeSevenHoleWriterFactory) getByPath(path string) *fakeSevenHoleWriter {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.writers[path]
}

// TestBuildSevenHoleCsvSink_FactoryNotInjected 验证工厂未注入时返回 nil sink
//
// 测试前置：manager 未注入 sevenHoleWriterFactory，但 SavePath 非空
// 测试步骤：调用 buildSevenHoleCsvSink
// 期待结果：返回 nil，不 panic；记录 slog.Error（factory 缺失是真实配置错误）
//
// spec Task 22：factory 缺失明确 Error。旧实现 log.Printf 不经 slog，slog 断言红灯。
func TestBuildSevenHoleCsvSink_FactoryNotInjected(t *testing.T) {
	handler, restore := withRecordingLogger(t)
	defer restore()

	manager := NewCalibrationManager(nil, nil, nil, nil)
	config := calibration.Config{
		Type:     string(calibration.TypeSevenHole),
		TaskID:   "task-factory-missing",
		SavePath: "D:/data/sevenhole.csv",
	}
	sink := manager.buildSevenHoleCsvSink(config)
	if sink != nil {
		t.Fatalf("工厂未注入时应返回 nil sink，实际返回 %T", sink)
	}
	// spec Task 22：factory 缺失应记录 slog.Error（含 'factory' 关键字）
	if !handler.hasLevelMessage(slog.LevelError, "factory") {
		t.Error("factory 未注入时应记录 slog.Error（含 'factory' 关键字），实际未记录")
	}
}

// TestBuildSevenHoleCsvSink_EmptySavePath 验证空 SavePath 返回 nil sink
//
// 测试前置：manager 已注入 factory，但 SavePath 为空
// 测试步骤：调用 buildSevenHoleCsvSink
// 期待结果：返回 nil；不记录 slog.Error（SavePath 空是合法的可选项，不是错误）
//
// spec Task 22：SavePath 空不报假错误。旧实现 log.Printf 统一打印，无法区分级别。
func TestBuildSevenHoleCsvSink_EmptySavePath(t *testing.T) {
	handler, restore := withRecordingLogger(t)
	defer restore()

	manager := NewCalibrationManager(nil, nil, nil, nil)
	manager.SetSevenHoleWriterFactory(newFakeSevenHoleWriterFactory())
	config := calibration.Config{Type: string(calibration.TypeSevenHole), TaskID: "task-empty-path"}
	sink := manager.buildSevenHoleCsvSink(config)
	if sink != nil {
		t.Fatalf("空 SavePath 时应返回 nil sink，实际返回 %T", sink)
	}
	// spec Task 22：SavePath 空不应记录 slog.Error（不是错误，只是跳过 CSV 落盘）
	if handler.hasLevelMessage(slog.LevelError, "factory") {
		t.Error("空 SavePath 时不应记录 slog.Error（SavePath 空是合法可选项，不是错误）")
	}
}

// TestBuildSevenHoleCsvSink_RouteByRegionSector 验证按 region+sector 路由分流
//
// 测试前置：注入 fakeSevenHoleWriterFactory，SavePath = "D:/data/sevenhole.csv"
// 测试步骤：依次推送 1 个内区点 + 2 个外区点（sector=1, sector=2）+ 1 个内区点
// 期待结果：
//   - 工厂创建 3 个 writer（_小角度区.csv / _大角度1区.csv / _大角度2区.csv）
//   - 内区 writer 收到 2 个点（首尾两个内区点）
//   - 外区 1 writer 收到 1 个点
//   - 外区 2 writer 收到 1 个点
func TestBuildSevenHoleCsvSink_RouteByRegionSector(t *testing.T) {
	manager := NewCalibrationManager(nil, nil, nil, nil)
	factory := newFakeSevenHoleWriterFactory()
	manager.SetSevenHoleWriterFactory(factory)
	config := calibration.Config{
		Type:     string(calibration.TypeSevenHole),
		SavePath: "D:/data/sevenhole.csv",
	}

	sink := manager.buildSevenHoleCsvSink(config)
	if sink == nil {
		t.Fatal("buildSevenHoleCsvSink 返回 nil sink")
	}

	// 推送 4 个点：内区 → 外区1 → 外区2 → 内区
	sink(&calibration.SevenHoleDataPoint{Region: "inner", Sector: 7, PointID: 1})
	sink(&calibration.SevenHoleDataPoint{Region: "outer", Sector: 1, PointID: 2})
	sink(&calibration.SevenHoleDataPoint{Region: "outer", Sector: 2, PointID: 3})
	sink(&calibration.SevenHoleDataPoint{Region: "inner", Sector: 7, PointID: 4})

	// 验证：3 个 writer 被创建
	if got := factory.count(); got != 3 {
		t.Fatalf("工厂创建 writer 数 = %d，期望 3", got)
	}

	// 验证文件路径与命名后缀
	innerWriter := factory.getByPath("D:/data/sevenhole_小角度区.csv")
	outer1Writer := factory.getByPath("D:/data/sevenhole_大角度1区.csv")
	outer2Writer := factory.getByPath("D:/data/sevenhole_大角度2区.csv")
	if innerWriter == nil {
		t.Fatal("内区 writer 未创建（期望路径 D:/data/sevenhole_小角度区.csv）")
	}
	if outer1Writer == nil {
		t.Fatal("外区 1 writer 未创建（期望路径 D:/data/sevenhole_大角度1区.csv）")
	}
	if outer2Writer == nil {
		t.Fatal("外区 2 writer 未创建（期望路径 D:/data/sevenhole_大角度2区.csv）")
	}

	// 验证点数分流
	if got := len(innerWriter.points); got != 2 {
		t.Errorf("内区 writer 收到 %d 点，期望 2", got)
	}
	if got := len(outer1Writer.points); got != 1 {
		t.Errorf("外区 1 writer 收到 %d 点，期望 1", got)
	}
	if got := len(outer2Writer.points); got != 1 {
		t.Errorf("外区 2 writer 收到 %d 点，期望 1", got)
	}
}

// TestBuildSevenHoleCsvSink_LazyCreation 验证 writer 懒加载
//
// 测试前置：注入 fakeSevenHoleWriterFactory
// 测试步骤：只推送 1 个内区点（不推送外区点）
// 期待结果：工厂只创建 1 个 writer（内区），6 个外区 writer 不被创建
func TestBuildSevenHoleCsvSink_LazyCreation(t *testing.T) {
	manager := NewCalibrationManager(nil, nil, nil, nil)
	factory := newFakeSevenHoleWriterFactory()
	manager.SetSevenHoleWriterFactory(factory)
	config := calibration.Config{
		Type:     string(calibration.TypeSevenHole),
		SavePath: "D:/data/sevenhole.csv",
	}

	sink := manager.buildSevenHoleCsvSink(config)
	sink(&calibration.SevenHoleDataPoint{Region: "inner", Sector: 7, PointID: 1})

	// 仅内区 writer 被创建
	if got := factory.count(); got != 1 {
		t.Fatalf("懒加载失败：工厂创建 writer 数 = %d，期望 1（仅内区）", got)
	}
	if factory.getByPath("D:/data/sevenhole_小角度区.csv") == nil {
		t.Error("内区 writer 未创建")
	}
}

// TestBuildSevenHoleCsvSink_NonSevenHoleDataPoint 验证非 *SevenHoleDataPoint 类型跳过
//
// 测试前置：注入 fakeSevenHoleWriterFactory
// 测试步骤：推送一个 *FiveHoleDataPoint（错误类型）
// 期待结果：sink 不创建任何 writer，不 panic
func TestBuildSevenHoleCsvSink_NonSevenHoleDataPoint(t *testing.T) {
	manager := NewCalibrationManager(nil, nil, nil, nil)
	factory := newFakeSevenHoleWriterFactory()
	manager.SetSevenHoleWriterFactory(factory)
	config := calibration.Config{
		Type:     string(calibration.TypeSevenHole),
		SavePath: "D:/data/sevenhole.csv",
	}

	sink := manager.buildSevenHoleCsvSink(config)
	// 推送错误类型的数据点（模拟算法误返回）
	sink(&calibration.FiveHoleDataPoint{PointID: 1})

	if got := factory.count(); got != 0 {
		t.Errorf("非 *SevenHoleDataPoint 时不应创建 writer，实际创建 %d 个", got)
	}
}

// TestFlushAllSevenHoleWriters 验证所有 writer 被 flush
//
// 测试前置：注入 fakeSevenHoleWriterFactory，推送 3 个不同 region+sector 的点
// 测试步骤：调用 flushAllSevenHoleWriters
// 期待结果：所有 3 个 writer 的 flushed 字段为 true，sevenHoleWriters 重置为 nil
func TestFlushAllSevenHoleWriters(t *testing.T) {
	manager := NewCalibrationManager(nil, nil, nil, nil)
	factory := newFakeSevenHoleWriterFactory()
	manager.SetSevenHoleWriterFactory(factory)
	config := calibration.Config{
		Type:     string(calibration.TypeSevenHole),
		SavePath: "D:/data/sevenhole.csv",
	}

	sink := manager.buildSevenHoleCsvSink(config)
	sink(&calibration.SevenHoleDataPoint{Region: "inner", Sector: 7, PointID: 1})
	sink(&calibration.SevenHoleDataPoint{Region: "outer", Sector: 1, PointID: 2})
	sink(&calibration.SevenHoleDataPoint{Region: "outer", Sector: 2, PointID: 3})

	// flush 前所有 writer flushed=false
	for _, w := range factory.writers {
		if w.flushed {
			t.Error("flush 前 writer.flushed 应为 false")
		}
	}

	manager.flushAllSevenHoleWriters()

	// flush 后所有 writer flushed=true
	for path, w := range factory.writers {
		if !w.flushed {
			t.Errorf("writer %s flush 后 flushed 应为 true", path)
		}
	}
	// sevenHoleWriters 重置为 nil
	if manager.sevenHoleWriters != nil {
		t.Error("flush 后 sevenHoleWriters 应为 nil")
	}
}

// TestFlushAllSevenHoleWriters_NilWriters 验证 nil sevenHoleWriters 时不 panic
//
// 测试前置：未调用 buildSevenHoleCsvSink，sevenHoleWriters 为 nil
// 测试步骤：调用 flushAllSevenHoleWriters
// 期待结果：不 panic，无副作用
func TestFlushAllSevenHoleWriters_NilWriters(t *testing.T) {
	manager := NewCalibrationManager(nil, nil, nil, nil)
	// 不应 panic
	manager.flushAllSevenHoleWriters()
}

// ==================== 七孔 double-check 丢弃路径错误契约测试（spec Task 20） ====================

// racingSevenHoleWriterFactory 模拟"并发期间其他 goroutine 已创建同 key writer"场景：
// NewWriter 期间预填 cache，触发 routeSevenHoleWriter 的 double-check 丢弃路径。
//
// 测试策略：
//   - routeSevenHoleWriter 快速路径 RLock 检查 cache miss（map 为空）
//   - 进入慢路径调用 factory.NewWriter
//   - NewWriter 内部锁住 manager.mu 把 cachedWriter 写入 sevenHoleWriters[key]
//     （模拟并发期间其他 goroutine 已抢先创建）
//   - 返回 tempWriter（将被 double-check 丢弃）
//   - routeSevenHoleWriter 加写锁 double-check，发现 cached，解锁后调 tempWriter.Flush()
//   - tempWriter.Flush 失败应仅记录警告，函数仍返回 cachedWriter（spec Task 20）
type racingSevenHoleWriterFactory struct {
	mgr          *CalibrationManager
	key          string
	cachedWriter *fakeSevenHoleWriter
	tempWriter   *fakeSevenHoleWriter
}

func (f *racingSevenHoleWriterFactory) NewWriter(path string, schema calibration.CsvSchema) (ports.CalibrationCsvWriter, error) {
	// 模拟并发期间其他 goroutine 已抢先创建同 key writer
	f.mgr.mu.Lock()
	if f.mgr.sevenHoleWriters == nil {
		f.mgr.sevenHoleWriters = make(map[string]ports.CalibrationCsvWriter)
	}
	f.mgr.sevenHoleWriters[f.key] = f.cachedWriter
	f.mgr.mu.Unlock()
	// 返回 temp writer（将被 double-check 丢弃）
	return f.tempWriter, nil
}

// TestRouteSevenHoleWriter_DuplicateTempWriterFlushFailure_OnlyWarning 验证 double-check
// 路径丢弃 temp writer 时，Flush 失败仅记录警告，不影响 cached writer 返回。
//
// 测试前置：manager 注入 racingSevenHoleWriterFactory（NewWriter 期间预填 cache），
// tempWriter.Flush 返回错误
// 测试步骤：调用 routeSevenHoleWriter
// 期待结果：
//   - 返回 cached writer（非 nil，非错误）
//   - temp writer 的 Flush 被调用（flushed=true，即丢弃路径被执行）
//   - temp writer 的 Flush 错误未传播（函数返回 nil 错误）
//
// 修复场景（spec Task 20）：旧实现 `_ = writer.Flush()` 静默丢弃错误，调试困难。
// 修复后 Flush 失败记录 slog.Warn 警告，但函数仍返回 cached writer。
func TestRouteSevenHoleWriter_DuplicateTempWriterFlushFailure_OnlyWarning(t *testing.T) {
	manager := NewCalibrationManager(nil, nil, nil, nil)
	cachedWriter := &fakeSevenHoleWriter{path: "D:/data/sevenhole_小角度区.csv"}
	tempWriter := &fakeSevenHoleWriter{
		path:     "D:/data/sevenhole_小角度区.csv",
		flushErr: errors.New("temp writer flush failed"),
	}

	// 初始化 sevenHoleWriters map（routeSevenHoleWriter 快速路径需要非 nil map 才能查缓存）
	manager.sevenHoleWriters = make(map[string]ports.CalibrationCsvWriter)

	manager.SetSevenHoleWriterFactory(&racingSevenHoleWriterFactory{
		mgr:          manager,
		key:          "inner",
		cachedWriter: cachedWriter,
		tempWriter:   tempWriter,
	})

	config := calibration.Config{
		Type:     string(calibration.TypeSevenHole),
		SavePath: "D:/data/sevenhole.csv",
	}

	// 调用 routeSevenHoleWriter：
	// - 快速路径 cache miss（map 为空）
	// - 慢路径 factory.NewWriter 期间预填 cache + 返回 tempWriter
	// - double-check 发现 cached，丢弃 tempWriter（调用其 Flush）
	// - tempWriter.Flush 返回错误，应仅警告，不传播
	writer, err := manager.routeSevenHoleWriter(config, "D:/data/sevenhole", "inner", 7)
	if err != nil {
		t.Fatalf("routeSevenHoleWriter 应返回 cached writer（temp Flush 错误仅警告），实际返回错误: %v", err)
	}
	if writer != cachedWriter {
		t.Fatalf("routeSevenHoleWriter 应返回 cached writer，实际返回 %T", writer)
	}
	// 验证 temp writer 的 Flush 被调用（即丢弃路径被执行，警告被记录）
	if !tempWriter.flushed {
		t.Error("temp writer 的 Flush 应被调用（double-check 丢弃路径）")
	}
}

// TestRouteSevenHoleWriter_DuplicateTempWriterFlushSuccess_NoWarning 验证 double-check
// 路径丢弃 temp writer 时，Flush 成功不记录警告（正常路径）。
//
// 测试前置：manager 注入 racingSevenHoleWriterFactory，tempWriter.Flush 返回 nil
// 测试步骤：调用 routeSevenHoleWriter
// 期待结果：返回 cached writer，temp writer Flush 被调用，函数返回 nil 错误
func TestRouteSevenHoleWriter_DuplicateTempWriterFlushSuccess_NoWarning(t *testing.T) {
	manager := NewCalibrationManager(nil, nil, nil, nil)
	cachedWriter := &fakeSevenHoleWriter{path: "D:/data/sevenhole_小角度区.csv"}
	tempWriter := &fakeSevenHoleWriter{path: "D:/data/sevenhole_小角度区.csv"} // flushErr 为 nil

	manager.sevenHoleWriters = make(map[string]ports.CalibrationCsvWriter)
	manager.SetSevenHoleWriterFactory(&racingSevenHoleWriterFactory{
		mgr:          manager,
		key:          "inner",
		cachedWriter: cachedWriter,
		tempWriter:   tempWriter,
	})

	config := calibration.Config{
		Type:     string(calibration.TypeSevenHole),
		SavePath: "D:/data/sevenhole.csv",
	}

	writer, err := manager.routeSevenHoleWriter(config, "D:/data/sevenhole", "inner", 7)
	if err != nil {
		t.Fatalf("routeSevenHoleWriter 不应返回错误: %v", err)
	}
	if writer != cachedWriter {
		t.Fatalf("routeSevenHoleWriter 应返回 cached writer，实际返回 %T", writer)
	}
	if !tempWriter.flushed {
		t.Error("temp writer 的 Flush 应被调用（double-check 丢弃路径）")
	}
}
