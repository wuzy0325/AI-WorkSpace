package calibration

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// CalibrationCsvWriter 校准专用CSV写入器
// 每采集一个点就追加写入CSV，校准结束或异常时flush
type CalibrationCsvWriter struct {
	mu     sync.Mutex
	file   *os.File
	writer *csv.Writer
	path   string
	config Config
	header []string
}

// NewCalibrationCsvWriter 创建校准CSV写入器
func NewCalibrationCsvWriter(config Config) *CalibrationCsvWriter {
	return &CalibrationCsvWriter{
		config: config,
	}
}

// Initialize 初始化CSV文件，写入表头
func (w *CalibrationCsvWriter) Initialize() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.config.SavePath == "" {
		return fmt.Errorf("保存路径为空")
	}

	// 确保目录存在
	dir := filepath.Dir(w.config.SavePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 构建文件路径
	w.path = w.config.SavePath
	if filepath.Ext(w.path) == "" {
		w.path = filepath.Join(w.path, fmt.Sprintf("calibration_%s.csv", w.config.TaskID))
	}

	// 打开文件（追加模式）
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("打开CSV文件失败: %w", err)
	}

	w.file = file
	w.writer = csv.NewWriter(file)

	// 写入表头
	w.header = w.buildHeader()
	if err := w.writer.Write(w.header); err != nil {
		return fmt.Errorf("写入CSV表头失败: %w", err)
	}
	w.writer.Flush()

	return nil
}

// AppendPoint 追加一个数据点到CSV
func (w *CalibrationCsvWriter) AppendPoint(dataPoint DataPoint) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.writer == nil {
		return fmt.Errorf("CSV写入器未初始化")
	}

	record := w.buildRecord(dataPoint)
	if err := w.writer.Write(record); err != nil {
		return fmt.Errorf("写入CSV数据失败: %w", err)
	}
	w.writer.Flush()

	return nil
}

// Flush 刷新并关闭CSV文件
func (w *CalibrationCsvWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.writer != nil {
		w.writer.Flush()
	}
	if w.file != nil {
		err := w.file.Close()
		w.file = nil
		w.writer = nil
		return err
	}
	return nil
}

// Path 获取CSV文件路径
func (w *CalibrationCsvWriter) Path() string {
	return w.path
}

// buildHeader 构建CSV表头
func (w *CalibrationCsvWriter) buildHeader() []string {
	base := []string{"pointId", "sampleCount", "stdDev", "startTime", "endTime"}

	switch CalibrationType(w.config.Type) {
	case TypeFiveHole:
		return append(base,
			"p1", "p2", "p3", "p4", "p5", "pAtm", "tAtm", "pTotal", "pStatic",
			"Kalpha", "Kbeta", "CPT", "CPS", "machNumber",
		)
	case TypeThreeHole:
		return append(base,
			"p1", "p2", "p3", "pAtm", "pTotal",
			"K", "Cv", "Cp",
		)
	case TypeTotalPressure:
		return append(base,
			"alpha",
			"pAtm", "tAtm", "pTunnelTotal", "pTunnelStatic", "tTunnel", "pProbeTotal",
			"CPT", "error", "machNumber",
		)
	case TypeTotalTemperature:
		return []string{
			"id", "targetMachNumber", "actualMachNumber",
			"testProbeTemp", "standardProbeTemp", "recoveryCoefficient",
			"totalPressure", "staticPressure", "atmosphericPressure",
			"atmosphericTemperature", "stdDev", "timestamp",
		}
	default:
		return base
	}
}

// buildRecord 构建CSV数据行
func (w *CalibrationCsvWriter) buildRecord(dataPoint DataPoint) []string {
	switch dp := dataPoint.(type) {
	case *FiveHoleDataPoint:
		return w.buildFiveHoleRecord(dp)
	case *ThreeHoleDataPoint:
		return w.buildThreeHoleRecord(dp)
	case *TotalPressureDataPoint:
		return w.buildTotalPressureRecord(dp)
	case *TotalTemperatureDataPoint:
		return w.buildTotalTemperatureRecord(dp)
	default:
		return []string{fmt.Sprintf("%d", dataPoint.GetPointID())}
	}
}

func (w *CalibrationCsvWriter) buildFiveHoleRecord(dp *FiveHoleDataPoint) []string {
	pTotal := ""
	if dp.RawData.PTotal != nil {
		pTotal = fmt.Sprintf("%f", *dp.RawData.PTotal)
	}
	pStatic := ""
	if dp.RawData.PStatic != nil {
		pStatic = fmt.Sprintf("%f", *dp.RawData.PStatic)
	}
	machNumber := ""
	if dp.Coefficients.MachNumber != nil {
		machNumber = fmt.Sprintf("%f", *dp.Coefficients.MachNumber)
	}

	return []string{
		fmt.Sprintf("%d", dp.PointID),
		fmt.Sprintf("%d", dp.SampleCount),
		fmt.Sprintf("%f", dp.StdDev),
		fmt.Sprintf("%d", dp.StartTime),
		fmt.Sprintf("%d", dp.EndTime),
		fmt.Sprintf("%f", dp.RawData.P1),
		fmt.Sprintf("%f", dp.RawData.P2),
		fmt.Sprintf("%f", dp.RawData.P3),
		fmt.Sprintf("%f", dp.RawData.P4),
		fmt.Sprintf("%f", dp.RawData.P5),
		fmt.Sprintf("%f", dp.RawData.PAtm),
		fmt.Sprintf("%f", dp.RawData.TAtm),
		pTotal,
		pStatic,
		fmt.Sprintf("%f", dp.Coefficients.Kalpha),
		fmt.Sprintf("%f", dp.Coefficients.Kbeta),
		fmt.Sprintf("%f", dp.Coefficients.CPT),
		fmt.Sprintf("%f", dp.Coefficients.CPS),
		machNumber,
	}
}

func (w *CalibrationCsvWriter) buildThreeHoleRecord(dp *ThreeHoleDataPoint) []string {
	pTotal := ""
	if dp.RawData.PTotal != nil {
		pTotal = fmt.Sprintf("%f", *dp.RawData.PTotal)
	}

	return []string{
		fmt.Sprintf("%d", dp.PointID),
		fmt.Sprintf("%d", dp.SampleCount),
		fmt.Sprintf("%f", dp.StdDev),
		fmt.Sprintf("%d", dp.StartTime),
		fmt.Sprintf("%d", dp.EndTime),
		fmt.Sprintf("%f", dp.RawData.P1),
		fmt.Sprintf("%f", dp.RawData.P2),
		fmt.Sprintf("%f", dp.RawData.P3),
		fmt.Sprintf("%f", dp.RawData.PAtm),
		pTotal,
		fmt.Sprintf("%f", dp.Coefficients.K),
		fmt.Sprintf("%f", dp.Coefficients.Cv),
		fmt.Sprintf("%f", dp.Coefficients.Cp),
	}
}

func (w *CalibrationCsvWriter) buildTotalPressureRecord(dp *TotalPressureDataPoint) []string {
	return []string{
		fmt.Sprintf("%d", dp.PointID),
		fmt.Sprintf("%d", dp.SampleCount),
		"0", // stdDev not in this type
		fmt.Sprintf("%d", dp.StartTime),
		fmt.Sprintf("%d", dp.EndTime),
		fmt.Sprintf("%f", dp.Alpha),
		fmt.Sprintf("%f", dp.RawData.PAtm),
		fmt.Sprintf("%f", dp.RawData.TAtm),
		fmt.Sprintf("%f", dp.RawData.PTunnelTotal),
		fmt.Sprintf("%f", dp.RawData.PTunnelStatic),
		fmt.Sprintf("%f", dp.RawData.TTunnel),
		fmt.Sprintf("%f", dp.RawData.PProbeTotal),
		fmt.Sprintf("%f", dp.Coefficients.CPT),
		fmt.Sprintf("%f", dp.Coefficients.Error),
		fmt.Sprintf("%f", dp.Coefficients.MachNumber),
	}
}

func (w *CalibrationCsvWriter) buildTotalTemperatureRecord(dp *TotalTemperatureDataPoint) []string {
	return []string{
		fmt.Sprintf("%d", dp.ID),
		fmt.Sprintf("%f", dp.TargetMachNumber),
		fmt.Sprintf("%f", dp.ActualMachNumber),
		fmt.Sprintf("%f", dp.TestProbeTemp),
		fmt.Sprintf("%f", dp.StandardProbeTemp),
		fmt.Sprintf("%f", dp.RecoveryCoefficient),
		fmt.Sprintf("%f", dp.TotalPressure),
		fmt.Sprintf("%f", dp.StaticPressure),
		fmt.Sprintf("%f", dp.AtmosphericPressure),
		fmt.Sprintf("%f", dp.AtmosphericTemperature),
		fmt.Sprintf("%f", dp.StdDev),
		fmt.Sprintf("%d", dp.Timestamp),
	}
}

// EnsureCsvWriter 确保CSV写入器已初始化（不影响校准启动）
func EnsureCsvWriter(writer **CalibrationCsvWriter, config Config) {
	if *writer != nil {
		return
	}
	savePath := config.SavePath
	if savePath == "" {
		return
	}
	w := NewCalibrationCsvWriter(config)
	if err := w.Initialize(); err != nil {
		log.Printf("[Calibration] CSV写入器初始化失败: %v", err)
		return
	}
	*writer = w
}
