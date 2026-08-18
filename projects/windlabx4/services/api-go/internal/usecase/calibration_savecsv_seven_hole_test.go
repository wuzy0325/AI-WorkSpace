package usecase

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"windlabx4/services/api-go/internal/core/calibration"
)

// calibration_savecsv_seven_hole_test.go — SaveCsv 七孔分区导出测试
//
// 验证七孔校准结果按 region+sector 分区落盘为参考数据集格式 CSV
// （spec §7.1 方案 A + §7.2/§7.3 18 列基础格式）：
//   - 文件命名：<stem>(小角度区).csv / <stem>(大角度<n>区).csv
//   - 外区点写入 φ/θ 坐标与 Kθ[n]/Kφ[n]/K0[n]/Ks[n] 系数（不再丢失为 0）
//   - 扇区边界点（φ=30°/90°/...）共享到相邻两个扇区文件，同 key 去重

// makeSevenHoleSaveCsvPoint 构造 SaveCsv 分区导出测试数据点
func makeSevenHoleSaveCsvPoint(id int, region string, sector int, coords map[string]float64) *calibration.SevenHoleDataPoint {
	pt, ps := 4117.517, -30.133
	dp := &calibration.SevenHoleDataPoint{
		PointID:     id,
		Region:      region,
		Sector:      sector,
		Coordinates: coords,
		RawData: calibration.SevenHoleRawData{
			P1: 3260.217, P2: -874.900, P3: -2771.350, P4: -2918.583,
			P5: -1093.750, P6: 2973.950, P7: 2168.100,
			PAtm: 98884.0, TAtm: 28.0,
			PTotal: &pt, PStatic: &ps,
		},
		SampleCount: 5,
	}
	if region == "outer" {
		dp.Coefficients = calibration.SevenHoleCoefficients{
			Ktheta: 0.494, Kphi: 1.741, K0Outer: -0.207, KsOuter: -0.260,
		}
	} else {
		dp.Coefficients = calibration.SevenHoleCoefficients{
			Kalpha: -4.988, Kbeta: -6.688, K0: -0.897, Ks: 0.106,
		}
	}
	return dp
}

func setupSevenHoleSaveCsvManager(points []calibration.DataPoint) (*CalibrationManager, *fakeSevenHoleWriterFactory) {
	manager := NewCalibrationManager(nil, nil, nil, nil)
	factory := newFakeSevenHoleWriterFactory()
	manager.SetSevenHoleWriterFactory(factory)
	manager.lastExport = &calibration.ExportPayload{
		Type: calibration.TypeSevenHole,
		Config: calibration.Config{
			TaskID: "cal-7h",
			Type:   string(calibration.TypeSevenHole),
		},
		DataPoints: points,
	}
	return manager, factory
}

// TestSaveCsv_SevenHoleZonedExport 验证七孔 SaveCsv 分区导出
//
// 测试前置：lastExport 含 1 内区点 + 2 外区点（Sector 1 的 φ=355°、Sector 2 的 φ=30°）
// 测试步骤：调用 SaveCsv
// 期待结果：
//   - 创建 (小角度区)/(大角度1区)/(大角度2区) 三个 writer（空分区不生成文件）
//   - 返回路径为内区文件路径
//   - 边界点 φ=30°（Sector 2）同时共享到 Sector 1 文件（13 条 φ 网格线对齐参考数据集）
//   - 各 writer 使用 18 列导出 schema（表头 18 列）且已 Flush
func TestSaveCsv_SevenHoleZonedExport(t *testing.T) {
	points := []calibration.DataPoint{
		makeSevenHoleSaveCsvPoint(1, "inner", 7, map[string]float64{"α": -30.0, "β": -30.0}),
		makeSevenHoleSaveCsvPoint(2, "outer", 1, map[string]float64{"θ": 30.0, "φ": 355.0}),
		makeSevenHoleSaveCsvPoint(3, "outer", 2, map[string]float64{"θ": 30.0, "φ": 30.0}),
	}
	manager, factory := setupSevenHoleSaveCsvManager(points)

	path, err := manager.SaveCsv("cal-7h", "D:/data/sevenhole.csv")
	if err != nil {
		t.Fatalf("save csv: %v", err)
	}

	stem := strings.TrimSuffix(filepath.Clean("D:/data/sevenhole.csv"), ".csv")
	innerPath := stem + "(小角度区).csv"
	if path != innerPath {
		t.Errorf("返回路径应为内区文件 %q, 实际 %q", innerPath, path)
	}
	if factory.count() != 3 {
		t.Fatalf("应创建 3 个分区 writer, 实际 %d", factory.count())
	}

	innerWriter := factory.getByPath(innerPath)
	if innerWriter == nil {
		t.Fatalf("内区 writer 未创建（期望路径 %s）", innerPath)
	}
	if got := len(innerWriter.points); got != 1 {
		t.Errorf("内区文件应有 1 点, 实际 %d", got)
	}
	if got := len(innerWriter.schema.BuildHeader()); got != 18 {
		t.Errorf("内区导出表头应为 18 列, 实际 %d", got)
	}
	if !innerWriter.flushed {
		t.Error("内区 writer 未 Flush")
	}

	// 外区 1 区：自有 φ=355° 点 + 共享的边界点 φ=30°（来自 Sector 2）
	outer1Writer := factory.getByPath(stem + "(大角度1区).csv")
	if outer1Writer == nil {
		t.Fatal("外区 1 区 writer 未创建")
	}
	if got := len(outer1Writer.points); got != 2 {
		t.Errorf("外区 1 区文件应有 2 点（含共享边界点）, 实际 %d", got)
	}

	// 外区 2 区：自有边界点 φ=30°
	outer2Writer := factory.getByPath(stem + "(大角度2区).csv")
	if outer2Writer == nil {
		t.Fatal("外区 2 区 writer 未创建")
	}
	if got := len(outer2Writer.points); got != 1 {
		t.Errorf("外区 2 区文件应有 1 点, 实际 %d", got)
	}

	// 外区点落盘必须是 φ/θ 坐标 + 外区系数（修复前按内区 schema 落盘全为 0）
	rec := outer2Writer.schema.BuildRecord(outer2Writer.points[0])
	if rec[0] != "30.000" || rec[1] != "30.000" {
		t.Errorf("外区数据行 col0/col1 应为 φ/θ=30.000/30.000, 实际 %s/%s", rec[0], rec[1])
	}
	if rec[12] != "0.494" || rec[13] != "1.741" {
		t.Errorf("外区数据行系数列应为 Kθ/Kφ=0.494/1.741, 实际 %s/%s", rec[12], rec[13])
	}
}

// TestSaveCsv_SevenHoleBoundaryPointDedup 验证数据集模式边界点两扇区各采一次时，
// 共享逻辑按 (θ,φ) 去重，不会在同一文件内产生重复网格点
//
// 测试前置：Sector 1 与 Sector 2 各自拥有 (θ=30°, φ=30°) 边界点（数据集模式）
// 测试步骤：调用 SaveCsv
// 期待结果：
//   - 两个扇区文件各含 1 个 (30°,30°) 点（无重复）
//   - 不创建其他扇区文件（φ=30° 的相邻扇区对是 {1,2}，不得误入 6 区）
func TestSaveCsv_SevenHoleBoundaryPointDedup(t *testing.T) {
	points := []calibration.DataPoint{
		makeSevenHoleSaveCsvPoint(1, "outer", 1, map[string]float64{"θ": 30.0, "φ": 30.0}),
		makeSevenHoleSaveCsvPoint(2, "outer", 2, map[string]float64{"θ": 30.0, "φ": 30.0}),
	}
	manager, factory := setupSevenHoleSaveCsvManager(points)

	if _, err := manager.SaveCsv("cal-7h", "D:/data/sevenhole.csv"); err != nil {
		t.Fatalf("save csv: %v", err)
	}

	if factory.count() != 2 {
		t.Fatalf("应只创建 2 个分区 writer（φ=30° 邻接扇区对为 {1,2}）, 实际 %d", factory.count())
	}
	stem := strings.TrimSuffix(filepath.Clean("D:/data/sevenhole.csv"), ".csv")
	for _, sector := range []int{1, 2} {
		w := factory.getByPath(fmt.Sprintf("%s(大角度%d区).csv", stem, sector))
		if w == nil {
			t.Fatalf("外区 %d 区 writer 未创建", sector)
		}
		if got := len(w.points); got != 1 {
			t.Errorf("外区 %d 区文件应恰有 1 个边界点（去重后）, 实际 %d", sector, got)
		}
	}
}

// TestSaveCsv_SevenHoleBoundaryGeometricSharing 验证边界点共享按 φ 几何位置判定邻接扇区
//
// 数据集模式下 Sector 1 合法地同时包含两个边界：
//   - φ=330°（下边界）应共享到 6 区
//   - φ=30°（上边界）应共享到 2 区（不是 6 区——由已分配扇区减 1 推算会误判）
//
// 测试前置：Sector 1 含 φ=330° 与 φ=30° 两个边界点
// 期待结果：6 区文件获得 φ=330° 点，2 区文件获得 φ=30° 点，1 区文件保留 2 点
func TestSaveCsv_SevenHoleBoundaryGeometricSharing(t *testing.T) {
	points := []calibration.DataPoint{
		makeSevenHoleSaveCsvPoint(1, "outer", 1, map[string]float64{"θ": 30.0, "φ": 330.0}),
		makeSevenHoleSaveCsvPoint(2, "outer", 1, map[string]float64{"θ": 30.0, "φ": 30.0}),
	}
	manager, factory := setupSevenHoleSaveCsvManager(points)

	if _, err := manager.SaveCsv("cal-7h", "D:/data/sevenhole.csv"); err != nil {
		t.Fatalf("save csv: %v", err)
	}

	stem := strings.TrimSuffix(filepath.Clean("D:/data/sevenhole.csv"), ".csv")
	sector1 := factory.getByPath(stem + "(大角度1区).csv")
	sector2 := factory.getByPath(stem + "(大角度2区).csv")
	sector6 := factory.getByPath(stem + "(大角度6区).csv")
	if sector1 == nil || sector2 == nil || sector6 == nil {
		t.Fatalf("1/2/6 区 writer 都应创建, 实际: %v/%v/%v", sector1 != nil, sector2 != nil, sector6 != nil)
	}
	if got := len(sector1.points); got != 2 {
		t.Errorf("1 区文件应含 2 个自有点, 实际 %d", got)
	}
	if got := len(sector2.points); got != 1 {
		t.Errorf("2 区文件应含 1 个共享点（φ=30°）, 实际 %d", got)
	}
	if dp := sector2.points[0].(*calibration.SevenHoleDataPoint); dp.Coordinates["φ"] != 30.0 {
		t.Errorf("2 区共享点应为 φ=30°, 实际 %.1f", dp.Coordinates["φ"])
	}
	if got := len(sector6.points); got != 1 {
		t.Errorf("6 区文件应含 1 个共享点（φ=330°）, 实际 %d", got)
	}
	if dp := sector6.points[0].(*calibration.SevenHoleDataPoint); dp.Coordinates["φ"] != 330.0 {
		t.Errorf("6 区共享点应为 φ=330°, 实际 %.1f", dp.Coordinates["φ"])
	}
}

// TestSaveCsv_SevenHoleFactoryMissing 验证七孔分区导出工厂未注入时返回明确错误
func TestSaveCsv_SevenHoleFactoryMissing(t *testing.T) {
	manager := NewCalibrationManager(nil, nil, nil, nil)
	manager.lastExport = &calibration.ExportPayload{
		Type:   calibration.TypeSevenHole,
		Config: calibration.Config{TaskID: "cal-7h", Type: string(calibration.TypeSevenHole)},
		DataPoints: []calibration.DataPoint{
			makeSevenHoleSaveCsvPoint(1, "inner", 7, map[string]float64{"α": 0, "β": 0}),
		},
	}
	if _, err := manager.SaveCsv("cal-7h", "D:/data/sevenhole.csv"); err == nil {
		t.Fatal("七孔 writer 工厂未注入时应返回错误，实际返回 nil")
	}
}
