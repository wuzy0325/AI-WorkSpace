package usecase

import (
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
