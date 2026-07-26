package usecase

import (
	"errors"
	"path/filepath"
	"testing"

	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/ports"
)

type fakeCalibrationCsvWriter struct {
	path    string
	points  []calibration.DataPoint
	flushed bool
}

func (w *fakeCalibrationCsvWriter) Initialize(config calibration.Config) error {
	w.path = config.SavePath
	return nil
}

func (w *fakeCalibrationCsvWriter) AppendPoint(point calibration.DataPoint) error {
	w.points = append(w.points, point)
	return nil
}

func (w *fakeCalibrationCsvWriter) Flush() error {
	w.flushed = true
	return nil
}

func (w *fakeCalibrationCsvWriter) Path() string { return w.path }

// failingCalibrationCsvWriter 可注入错误的 CSV writer 桩，用于测试 SaveCsv 的
// append+cleanup 错误聚合（spec Task 20）。
//
// 与 fakeCalibrationCsvWriter 区别：AppendPoint 和 Flush 均可注入指定错误。
type failingCalibrationCsvWriter struct {
	path      string
	appendErr error // AppendPoint 返回的错误（每次调用都返回）
	flushErr  error // Flush 返回的错误
	flushed   bool
}

func (w *failingCalibrationCsvWriter) Initialize(config calibration.Config) error {
	w.path = config.SavePath
	return nil
}

func (w *failingCalibrationCsvWriter) AppendPoint(point calibration.DataPoint) error {
	return w.appendErr
}

func (w *failingCalibrationCsvWriter) Flush() error {
	w.flushed = true
	return w.flushErr
}

func (w *failingCalibrationCsvWriter) Path() string { return w.path }

func TestCalibrationManagerSaveCsvUsesStoredExportPayload(t *testing.T) {
	manager := NewCalibrationManager(nil, nil, nil, nil)
	writer := &fakeCalibrationCsvWriter{}
	manager.SetCsvWriterFactory(func(calibration.Config) ports.CalibrationCsvWriter {
		return writer
	})
	manager.lastExport = &calibration.ExportPayload{
		Type: calibration.TypeFiveHole,
		Config: calibration.Config{
			TaskID: "cal-1",
			Type:   string(calibration.TypeFiveHole),
		},
		DataPoints: []calibration.DataPoint{
			&calibration.FiveHoleDataPoint{PointID: 1},
		},
	}

	path, err := manager.SaveCsv("cal-1", "D:/data/five-hole.csv")
	if err != nil {
		t.Fatalf("save csv: %v", err)
	}
	// SaveCsv 入口会对 savePath 做 filepath.Clean 归一（Windows 下 / → \）
	if want := filepath.Clean("D:/data/five-hole.csv"); path != want {
		t.Fatalf("expected saved path %q, got %q", want, path)
	}
	if len(writer.points) != 1 || !writer.flushed {
		t.Fatalf("expected one flushed point, points=%d flushed=%v", len(writer.points), writer.flushed)
	}
}

// TestSaveCsv_JoinsAppendAndCleanupErrors 验证 AppendPoint 失败时 cleanup Flush 也失败，
// SaveCsv 返回的 error 通过 errors.Join 聚合两个错误，调用方可同时识别。
//
// 测试前置：manager 注入 failingCalibrationCsvWriter（AppendPoint 和 Flush 均返回错误）
// 测试步骤：调用 SaveCsv
// 期待结果：
//   - 返回非 nil 错误
//   - 错误可通过 errors.Is 识别 append 错误和 cleanup Flush 错误
//   - cleanup Flush 被调用（flushed=true）
//
// 修复场景（spec Task 20）：旧实现 `_ = writer.Flush()` 静默丢弃 cleanup 错误，
// 只返回 AppendPoint 错误。修复后用 errors.Join 聚合两个错误，确保均可识别。
func TestSaveCsv_JoinsAppendAndCleanupErrors(t *testing.T) {
	manager := NewCalibrationManager(nil, nil, nil, nil)
	appendErr := errors.New("append failed")
	flushErr := errors.New("cleanup flush failed")
	writer := &failingCalibrationCsvWriter{appendErr: appendErr, flushErr: flushErr}
	manager.SetCsvWriterFactory(func(calibration.Config) ports.CalibrationCsvWriter {
		return writer
	})
	manager.lastExport = &calibration.ExportPayload{
		Type: calibration.TypeFiveHole,
		Config: calibration.Config{
			TaskID: "cal-1",
			Type:   string(calibration.TypeFiveHole),
		},
		DataPoints: []calibration.DataPoint{
			&calibration.FiveHoleDataPoint{PointID: 1},
		},
	}

	_, err := manager.SaveCsv("cal-1", "D:/data/five-hole.csv")
	if err == nil {
		t.Fatal("SaveCsv 应返回错误，实际返回 nil")
	}
	// 验证 append 错误可识别
	if !errors.Is(err, appendErr) {
		t.Errorf("SaveCsv 错误应包含 append 错误 %v，实际: %v", appendErr, err)
	}
	// 验证 cleanup Flush 错误可识别（errors.Join 聚合后 errors.Is 仍可匹配子错误）
	if !errors.Is(err, flushErr) {
		t.Errorf("SaveCsv 错误应包含 cleanup flush 错误 %v，实际: %v", flushErr, err)
	}
	// 验证 cleanup Flush 被调用
	if !writer.flushed {
		t.Error("SaveCsv AppendPoint 失败时应调用 cleanup Flush（关闭文件句柄）")
	}
}

// TestSaveCsv_CleanupFlushSuccessReturnsAppendErrorOnly 验证 AppendPoint 失败但 cleanup Flush
// 成功时，SaveCsv 只返回 AppendPoint 错误（errors.Join 忽略 nil cleanup 错误）。
//
// 测试前置：manager 注入 failingCalibrationCsvWriter（仅 AppendPoint 返回错误，Flush 成功）
// 测试步骤：调用 SaveCsv
// 期待结果：返回的错误只包含 append 错误，不包含 nil cleanup 错误
func TestSaveCsv_CleanupFlushSuccessReturnsAppendErrorOnly(t *testing.T) {
	manager := NewCalibrationManager(nil, nil, nil, nil)
	appendErr := errors.New("append failed")
	writer := &failingCalibrationCsvWriter{appendErr: appendErr} // flushErr 为 nil
	manager.SetCsvWriterFactory(func(calibration.Config) ports.CalibrationCsvWriter {
		return writer
	})
	manager.lastExport = &calibration.ExportPayload{
		Type: calibration.TypeFiveHole,
		Config: calibration.Config{
			TaskID: "cal-1",
			Type:   string(calibration.TypeFiveHole),
		},
		DataPoints: []calibration.DataPoint{
			&calibration.FiveHoleDataPoint{PointID: 1},
		},
	}

	_, err := manager.SaveCsv("cal-1", "D:/data/five-hole.csv")
	if err == nil {
		t.Fatal("SaveCsv 应返回错误，实际返回 nil")
	}
	if !errors.Is(err, appendErr) {
		t.Errorf("SaveCsv 错误应包含 append 错误 %v，实际: %v", appendErr, err)
	}
	// cleanup Flush 应被调用（即使成功也要调用，关闭文件句柄）
	if !writer.flushed {
		t.Error("SaveCsv AppendPoint 失败时应调用 cleanup Flush")
	}
}
