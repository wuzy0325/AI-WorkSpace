package report

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"cal1604/internal/application/calibration"
	"cal1604/internal/domain"
	apperrors "cal1604/internal/errors"

	"github.com/xuri/excelize/v2"
)

// Service 封装报告模板路径拼装逻辑与报告导出。
type Service struct {
	templateDir           string
	embedTemplateProvider *EmbedTemplateProvider

	// deviceNameResolver 将设备 ID 解析为设备名（可注入、可空）。
	// 多设备导出时报告文件名后缀优先使用设备名；未注入或解析为空时回退设备 ID。
	deviceNameResolver func(deviceID string) string
}

// ReportTemplate 描述一个可用报告模板。
type ReportTemplate struct {
	Name       string `json:"name"`
	PointCount int    `json:"pointCount"`
	Mode       string `json:"mode"`
	Path       string `json:"path"`
}

// NewService 创建报告服务。
// templateDir 为外部模板目录（可选），embedFS 为嵌入模板文件系统（可选）。
// embedPrefix 为 embedFS 内的模板目录前缀，默认 "templates/reports"。
// 优先使用外部目录，不存在时回退到 embed.FS。
func NewService(templateDir string, embedFS ...fs.FS) *Service {
	s := &Service{templateDir: templateDir}
	if len(embedFS) > 0 && embedFS[0] != nil {
		s.embedTemplateProvider = NewEmbedTemplateProvider(embedFS[0], "templates/reports")
	}
	return s
}

// NewServiceWithPrefix 创建报告服务并指定 embed 前缀。
func NewServiceWithPrefix(templateDir string, embedFS fs.FS, embedPrefix string) *Service {
	s := &Service{templateDir: templateDir}
	if embedFS != nil {
		s.embedTemplateProvider = NewEmbedTemplateProvider(embedFS, embedPrefix)
	}
	return s
}

// SetEmbedTemplateProvider 设置嵌入模板提供者（用于运行时动态注入）。
func (s *Service) SetEmbedTemplateProvider(provider *EmbedTemplateProvider) {
	s.embedTemplateProvider = provider
}

// SetDeviceNameResolver 注入设备名称解析器（设备 ID → 设备名）。
// 多设备导出时报告文件名后缀优先使用设备名，未注入或解析为空时回退设备 ID。
func (s *Service) SetDeviceNameResolver(resolver func(deviceID string) string) {
	s.deviceNameResolver = resolver
}

// deviceFileLabel 计算单台设备在报告文件名中的后缀标识。
// 优先设备名（经文件名安全化），名称缺失时回退设备 ID；
// 标识与先前设备冲突时追加序号后缀（如 "_2"），确保多台设备的报告文件互不覆盖。
func (s *Service) deviceFileLabel(deviceID string, used map[string]bool) string {
	label := sanitizeFileName(s.deviceDisplayName(deviceID))
	if label == "" {
		label = sanitizeFileName(deviceID)
	}
	base := label
	for suffix := 2; label == "" || used[label]; suffix++ {
		label = fmt.Sprintf("%s_%d", base, suffix)
	}
	used[label] = true
	return label
}

// deviceDisplayName 返回设备展示名；解析器未注入或结果为空时返回空串。
func (s *Service) deviceDisplayName(deviceID string) string {
	if s.deviceNameResolver == nil {
		return ""
	}
	return strings.TrimSpace(s.deviceNameResolver(deviceID))
}

// sanitizeFileName 把任意字符串转换为 Windows/Linux 通用的安全文件名片段：
// 非法字符（\ / : * ? " < > |）替换为下划线，去除首尾空白与点号
// （避免相对路径语义与 Windows 尾点问题）。结果可能为空，由调用方回退。
func sanitizeFileName(name string) string {
	replacer := strings.NewReplacer(
		`\`, "_", "/", "_", ":", "_", "*", "_",
		"?", "_", `"`, "_", "<", "_", ">", "_", "|", "_",
	)
	cleaned := strings.TrimSpace(replacer.Replace(name))
	return strings.Trim(cleaned, ".")
}

// CleanupEmbedTemplates 清理嵌入模板解压的临时目录。
func (s *Service) CleanupEmbedTemplates() error {
	if s.embedTemplateProvider != nil {
		return s.embedTemplateProvider.Cleanup()
	}
	return nil
}

// ResolveTemplatePath 解析模板绝对路径。
func (s *Service) ResolveTemplatePath(points int, mode string) (string, error) {
	return s.MatchTemplate(points, mode)
}

// ExportReport 根据 CalibrationSession 生成校准报告并保存到 outputPath。
// 多设备场景：为每台参与计量设备分别生成独立报告文件。
//   - 单设备（或旧数据）：outputPath 作为报告文件路径。
//   - 多设备：outputPath 作为基路径，在文件名中插入设备 ID，例如
//     "报告.xlsx" → "报告_dev-a.xlsx"，避免静默把用户指定的文件路径改造成目录。
//
// 返回实际生成的报告文件路径列表（多设备时每台一个，顺序与设备集合一致）。
// 优先使用模板文件填充数据，无模板时创建默认工作簿。
// ExportReport 根据 CalibrationSession 生成校准报告并保存到 outputPath。
// unit 为报告压力单位（来自计量设备真实单位，如 "psi"/"kPa"），不再写死。
func (s *Service) ExportReport(ctx context.Context, session *calibration.CalibrationSession, outputPath, unit string) ([]string, error) {
	if session == nil {
		return nil, fmt.Errorf("%w: calibration session is nil", apperrors.ErrNoActiveSession)
	}
	if unit == "" {
		unit = "kPa"
	}

	// 多设备：每台设备生成独立报告
	devIDs := session.MeasureDeviceIDs
	if len(devIDs) == 0 && session.MeasureDeviceID != "" {
		devIDs = []string{session.MeasureDeviceID}
	}
	if len(devIDs) > 1 {
		paths := make([]string, 0, len(devIDs))
		used := make(map[string]bool, len(devIDs))
		for _, devID := range devIDs {
			devPath := perDeviceReportPath(outputPath, s.deviceFileLabel(devID, used))
			if err := s.exportReportForDevice(ctx, session, devID, devPath, unit); err != nil {
				return nil, err
			}
			paths = append(paths, devPath)
		}
		return paths, nil
	}
	if err := s.exportReportForDevice(ctx, session, "", outputPath, unit); err != nil {
		return nil, err
	}
	return []string{outputPath}, nil
}

// perDeviceReportPath 为多设备场景生成单台设备的报告文件路径。
// 在扩展名前插入设备文件名标识（优先设备名，回退设备 ID，见 deviceFileLabel），
// 保留原路径的目录与扩展名，不改变 outputPath 的文件语义。
func perDeviceReportPath(outputPath, deviceID string) string {
	ext := filepath.Ext(outputPath)
	if ext == "" {
		return filepath.Join(outputPath, deviceID+"_report.xlsx")
	}
	base := strings.TrimSuffix(outputPath, ext)
	return base + "_" + deviceID + ext
}

// exportReportForDevice 为单台计量设备生成报告。
// deviceID 为空时回退单设备字段（兼容旧数据）。
func (s *Service) exportReportForDevice(ctx context.Context, session *calibration.CalibrationSession, deviceID, outputPath, unit string) error {
	// 收集标准压力值（仅正程）
	standardValues := make([]float64, 0, len(session.Points))
	for _, p := range session.Points {
		if p.Direction == "" || p.Direction == "forward" {
			standardValues = append(standardValues, p.TargetPressure)
		}
	}

	// 收集通道数据
	channels := collectChannelData(session, deviceID)

	// 确定压力单位（由调用方传入，来自计量设备真实单位）

	// 尝试加载模板
	templatePath, err := s.ResolveTemplatePath(
		session.Config.PointCount,
		string(session.Config.PressureMode),
	)
	if err == nil && templatePath != "" {
		return s.exportWithTemplate(templatePath, outputPath, standardValues, channels, unit, session, deviceID)
	}

	// 无模板，创建默认工作簿
	return s.exportFallback(outputPath, standardValues, channels, unit)
}

// exportWithTemplate 使用模板文件导出报告。
func (s *Service) exportWithTemplate(templatePath, outputPath string, standardValues []float64, channels [][]float64, unit string, session *calibration.CalibrationSession, deviceID ...string) error {
	f, err := LoadTemplate(templatePath)
	if err != nil {
		return fmt.Errorf("%w: load template: %v", apperrors.ErrReportExport, err)
	}
	defer f.Close()

	blocks, err := FindChannelBlocks(f)
	if err != nil {
		return fmt.Errorf("%w: find channel blocks: %v", apperrors.ErrReportExport, err)
	}

	targetDev := ""
	if len(deviceID) > 0 {
		targetDev = deviceID[0]
	}

	for i, block := range blocks {
		if i >= len(channels) {
			break
		}

		// B 列填标准值（仅第一个块填充）
		if i == 0 {
			if err := FillStandardValues(f, block, "B", standardValues, unit); err != nil {
				return fmt.Errorf("%w: fill standard values: %v", apperrors.ErrReportExport, err)
			}
		}

		// C 列填测量值
		header := fmt.Sprintf("测量值-块%d", i+1)
		if err := FillMeasureData(f, block, "C", header, channels[i]); err != nil {
			return fmt.Errorf("%w: fill measure data block %d: %v", apperrors.ErrReportExport, i+1, err)
		}

		// 回程模式：D 列填回程数据
		if session.Config.PressureMode == domain.PressureModeRoundTrip {
			backwardData := collectBackwardData(session, i, targetDev)
			if len(backwardData) > 0 {
				if err := FillRoundTripData(f, block, "D", channels[i], backwardData); err != nil {
					return fmt.Errorf("%w: fill round-trip data block %d: %v", apperrors.ErrReportExport, i+1, err)
				}
			}
		}
	}

	// 改写模板主单位格并联动全部公式引用（结果分析表头、通道块 Unit 等）
	applyReportTemplateUnit(f, unit)

	if err := f.SaveAs(outputPath); err != nil {
		return fmt.Errorf("%w: save report: %v", apperrors.ErrReportExport, err)
	}

	return nil
}

// exportFallback 创建无模板的默认报告。
func (s *Service) exportFallback(outputPath string, standardValues []float64, channels [][]float64, unit string) error {
	f := CreateFallbackWorkbook(standardValues, channels, unit)
	defer f.Close()

	if err := f.SaveAs(outputPath); err != nil {
		return fmt.Errorf("%w: save fallback report: %v", apperrors.ErrReportExport, err)
	}

	return nil
}

// GetTemplates 返回模板目录中可用的模板列表与元信息。
// 优先扫描外部目录，不存在时从 embed.FS 获取。
func (s *Service) GetTemplates() ([]ReportTemplate, error) {
	var entries []os.DirEntry
	var dirPath string
	var err error

	if s.templateDir != "" {
		entries, err = os.ReadDir(s.templateDir)
		dirPath = s.templateDir
	}
	if s.templateDir == "" || (err != nil && os.IsNotExist(err)) {
		if s.embedTemplateProvider != nil {
			files, listErr := s.embedTemplateProvider.ListTemplates()
			if listErr != nil {
				return nil, fmt.Errorf("%w: list embed templates: %v", apperrors.ErrReportExport, listErr)
			}
			templates := make([]ReportTemplate, 0, len(files))
			for _, name := range files {
				template, ok := parseTemplateFileName(name)
				if !ok {
					continue
				}
				templates = append(templates, template)
			}
			sortTemplates(templates)
			return templates, nil
		}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: read template dir: %v", apperrors.ErrReportExport, err)
	}

	templates := make([]ReportTemplate, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		template, ok := parseTemplateFileName(entry.Name())
		if !ok {
			continue
		}
		template.Path = filepath.Join(dirPath, entry.Name())
		templates = append(templates, template)
	}

	sortTemplates(templates)
	return templates, nil
}

// sortTemplates 按点数、模式、名称排序模板列表。
func sortTemplates(templates []ReportTemplate) {
	sort.Slice(templates, func(i, j int) bool {
		if templates[i].PointCount != templates[j].PointCount {
			return templates[i].PointCount < templates[j].PointCount
		}
		if templates[i].Mode != templates[j].Mode {
			return templates[i].Mode < templates[j].Mode
		}
		return templates[i].Name < templates[j].Name
	})
}

// MatchTemplate 根据点数与模式匹配模板绝对路径。
// 优先检查外部目录，其次从 embed.FS 解压到临时目录后返回路径。
func (s *Service) MatchTemplate(pointCount int, mode string) (string, error) {
	filename, err := SelectTemplate(pointCount, mode)
	if err != nil {
		return "", err
	}

	// 优先使用外部模板目录
	if s.templateDir != "" {
		fullPath := filepath.Join(s.templateDir, filename)
		if _, err := os.Stat(fullPath); err == nil {
			return fullPath, nil
		}
	}

	// 回退到 embed.FS
	if s.embedTemplateProvider != nil {
		return s.embedTemplateProvider.ResolvePath(filename)
	}

	return "", fmt.Errorf("%w: template not found: %s", apperrors.ErrReportExport, filename)
}

func parseTemplateFileName(filename string) (ReportTemplate, bool) {
	ext := filepath.Ext(filename)
	if !strings.EqualFold(ext, ".xlsx") {
		return ReportTemplate{}, false
	}

	base := strings.TrimSuffix(filename, ext)
	if len(base) < 2 {
		return ReportTemplate{}, false
	}

	suffix := strings.ToLower(base[len(base)-1:])
	pointPart := base[:len(base)-1]
	pointCount, err := strconv.Atoi(pointPart)
	if err != nil || pointCount <= 0 {
		return ReportTemplate{}, false
	}

	mode := ""
	switch suffix {
	case "s":
		mode = string(domain.PressureModeSingle)
	case "m":
		mode = string(domain.PressureModeRoundTrip)
	default:
		return ReportTemplate{}, false
	}

	return ReportTemplate{
		Name:       base,
		PointCount: pointCount,
		Mode:       mode,
	}, true
}

// ExportMeasurementReport 根据计量采集数据生成报告并保存到 outputPath。
// unit 为报告压力单位（来自计量设备真实单位，如 "psi"/"kPa"），不再写死。
// 多设备场景：为每台设备分别生成独立报告文件，outputPath 作为基路径，
// 在文件名中插入设备 ID（如 "报告.xlsx" → "报告_dev-a.xlsx"）。
// 返回实际生成的报告文件路径列表（多设备时每台一个，顺序与设备集合一致）。
func (s *Service) ExportMeasurementReport(ctx context.Context, points []domain.PressurePoint, config domain.WorkflowConfig, outputPath, unit string) ([]string, error) {
	if len(points) == 0 {
		return nil, fmt.Errorf("%w: no measurement points", apperrors.ErrNoActiveSession)
	}
	if unit == "" {
		unit = "kPa"
	}

	// 探测设备 ID 集合（多设备场景）
	deviceIDs := detectDeviceIDs(points)
	if len(deviceIDs) > 1 {
		paths := make([]string, 0, len(deviceIDs))
		used := make(map[string]bool, len(deviceIDs))
		for _, devID := range deviceIDs {
			devPath := perDeviceReportPath(outputPath, s.deviceFileLabel(devID, used))
			if err := s.exportMeasurementReportForDevice(ctx, points, config, devPath, devID, unit); err != nil {
				return nil, err
			}
			paths = append(paths, devPath)
		}
		return paths, nil
	}
	if err := s.exportMeasurementReportForDevice(ctx, points, config, outputPath, "", unit); err != nil {
		return nil, err
	}
	return []string{outputPath}, nil
}

// detectDeviceIDs 从压力点的设备维度数据中探测参与计量设备 ID 集合。
func detectDeviceIDs(points []domain.PressurePoint) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, p := range points {
		for devID := range p.CollectedByDevice {
			if !seen[devID] {
				seen[devID] = true
				ids = append(ids, devID)
			}
		}
	}
	return ids
}

// exportMeasurementReportForDevice 为单台计量设备生成计量报告。
// deviceID 为空时回退单设备字段。
func (s *Service) exportMeasurementReportForDevice(ctx context.Context, points []domain.PressurePoint, config domain.WorkflowConfig, outputPath, deviceID, unit string) error {
	// 多设备场景：报告「设备编号」元数据按设备分别派生（spec per-device-report）。
	// 设备配置无独立编号字段，以设备 ID 作为该台报告的编号；
	// 单设备场景保留会话配置的 DeviceNumber（兼容旧行为）。
	if deviceID != "" {
		config.DeviceNumber = deviceID
	}

	// 始终输出全部16通道
	numChannels := 16

	// 提取正程标准压力值
	standardValues := collectMeasurementStandardValues(points, "forward", deviceID)
	// 按通道聚合正程采集数据（从平铺数据计算每通道平均值）
	forwardChannels := collectMeasurementChannelData(points, numChannels, config.AverageCount, "forward", deviceID)
	// 回程模式下额外按 targetPressure 索引聚合回程数据，避免回程点缺失时与正程错位；
	// 单程模式 backwardByTarget 为空。
	var backwardByTarget []map[float64]float64
	if config.PressureMode == domain.PressureModeRoundTrip {
		backwardByTarget = collectMeasurementChannelByTarget(points, numChannels, config.AverageCount, "backward", deviceID)
	}

	// 压力单位由调用方传入（来自计量设备真实单位），不再写死

	// 尝试加载模板
	templatePath, err := s.ResolveTemplatePath(
		config.PointCount,
		string(config.PressureMode),
	)
	if err == nil && templatePath != "" {
		return s.exportMeasurementWithTemplate(ctx, templatePath, outputPath, standardValues, forwardChannels, backwardByTarget, unit, points, config, deviceID)
	}

	// 无模板，创建默认工作簿
	return s.exportMeasurementFallback(outputPath, standardValues, forwardChannels, backwardByTarget, unit, points, config, deviceID)
}

// exportMeasurementWithTemplate 使用模板文件导出计量报告。
// 计量模板列映射：
//
//	单程模板 *s.xlsx：A=标准压力，B=设备显示值，C=示值误差(公式)，D=不确定度
//	回程模板 *m.xlsx：A=标准压力，B=正程显示值(Forward stroke)，C=回程显示值(Return stroke)，D=示值误差(公式)，E=回差(公式)
//
// backwardByTarget 按 (通道, 标准压力) 索引，确保 C 列严格按 A 列标准值对齐，
// 即便部分回程点未完成也不会发生静默错位。
func (s *Service) exportMeasurementWithTemplate(ctx context.Context, templatePath, outputPath string, standardValues []float64, forwardChannels [][]float64, backwardByTarget []map[float64]float64, unit string, points []domain.PressurePoint, config domain.WorkflowConfig, deviceID ...string) error {
	f, err := LoadTemplate(templatePath)
	if err != nil {
		return fmt.Errorf("%w: load template: %v", apperrors.ErrReportExport, err)
	}
	defer f.Close()

	blocks, err := FindChannelBlocks(f)
	if err != nil {
		return fmt.Errorf("%w: find channel blocks: %v", apperrors.ErrReportExport, err)
	}

	isRoundTrip := config.PressureMode == domain.PressureModeRoundTrip

	for i, block := range blocks {
		if i >= len(forwardChannels) {
			break
		}

		// A 列填标准压力
		for j, val := range standardValues {
			cell := fmt.Sprintf("A%d", block.DataStart+j)
			rounded := math.Round(val*100) / 100
			if err := f.SetCellValue(block.Sheet, cell, rounded); err != nil {
				return fmt.Errorf("%w: fill standard values block %d row %d: %v", apperrors.ErrReportExport, i+1, j+1, err)
			}
		}

		// B 列填正程显示值（单程模式即唯一显示值）
		for j, val := range forwardChannels[i] {
			cell := fmt.Sprintf("B%d", block.DataStart+j)
			rounded := math.Round(val*1e6) / 1e6
			if err := f.SetCellValue(block.Sheet, cell, rounded); err != nil {
				return fmt.Errorf("%w: fill measure data block %d row %d: %v", apperrors.ErrReportExport, i+1, j+1, err)
			}
		}

		// 回程模式：C 列按 A 列标准压力精确匹配回程显示值（Return stroke），
		// 缺失的回程点保留模板初值，绝不写错行。
		if isRoundTrip && i < len(backwardByTarget) {
			lookup := backwardByTarget[i]
			for j, std := range standardValues {
				val, ok := lookup[std]
				if !ok {
					continue
				}
				cell := fmt.Sprintf("C%d", block.DataStart+j)
				rounded := math.Round(val*1e6) / 1e6
				if err := f.SetCellValue(block.Sheet, cell, rounded); err != nil {
					return fmt.Errorf("%w: fill return stroke block %d row %d: %v", apperrors.ErrReportExport, i+1, j+1, err)
				}
			}
		}
	}

	// 填充元数据：单位、日期等
	fillMeasurementWorksheetMetadata(f, unit, points, config)

	if err := f.SaveAs(outputPath); err != nil {
		return fmt.Errorf("%w: save report: %v", apperrors.ErrReportExport, err)
	}

	return nil
}

// exportMeasurementFallback 创建无模板的计量报告。
// 回程模式下若 backwardByTarget 非空，会同时写入"回程值"列。
func (s *Service) exportMeasurementFallback(outputPath string, standardValues []float64, forwardChannels [][]float64, backwardByTarget []map[float64]float64, unit string, points []domain.PressurePoint, config domain.WorkflowConfig, deviceID ...string) error {
	f := CreateMeasurementFallbackWorkbook(standardValues, forwardChannels, backwardByTarget, unit, points, config)
	defer f.Close()

	fillMeasurementWorksheetMetadata(f, unit, points, config)

	if err := f.SaveAs(outputPath); err != nil {
		return fmt.Errorf("%w: save fallback report: %v", apperrors.ErrReportExport, err)
	}

	return nil
}

// fillMeasurementWorksheetMetadata 扫描工作表填写元数据字段（日期、量程等）。
// 单位字段由 applyReportTemplateUnit 单独处理（模板单位为主格+公式联动结构，不能逐格覆盖）。
func fillMeasurementWorksheetMetadata(f *excelize.File, unit string, points []domain.PressurePoint, config domain.WorkflowConfig) {
	applyReportTemplateUnit(f, unit)

	if len(f.GetSheetList()) == 0 {
		return
	}
	sheet := f.GetSheetList()[0]

	nowStr := time.Now().Format("2006-01-02 15:04:05")

	// 从 points 提取首条采集时间（如有），转换为可读格式
	startTime := nowStr
	for _, p := range points {
		if p.CollectTime != "" {
			if t, err := time.Parse(time.RFC3339, p.CollectTime); err == nil {
				startTime = t.Format("2006-01-02 15:04:05")
			} else {
				startTime = p.CollectTime
			}
			break
		}
	}

	// 扫描前 50 行的前 12 列，匹配中文标签
	for row := 1; row <= 50; row++ {
		for col := 1; col <= 12; col++ {
			cell := cellName(col, row)
			text, _ := f.GetCellValue(sheet, cell)
			text = strings.TrimSpace(text)

			// 匹配"校准日期"或"日期"标签→右侧单元格填充日期
			if strings.Contains(text, "日期") || strings.Contains(strings.ToLower(text), "date") {
				rightCell := cellName(col+1, row)
				f.SetCellValue(sheet, rightCell, startTime)
				continue
			}

			// 匹配"Min(Range)"标签→右侧单元格填充最小量程
			if strings.Contains(text, "Min(Range)") || strings.Contains(text, "Min（Range）") {
				rightCell := cellName(col+1, row)
				f.SetCellValue(sheet, rightCell, config.MinPressure)
				continue
			}

			// 匹配"Max(Range)"标签→右侧单元格填充最大量程
			if strings.Contains(text, "Max(Range)") || strings.Contains(text, "Max（Range）") {
				rightCell := cellName(col+1, row)
				f.SetCellValue(sheet, rightCell, config.MaxPressure)
				continue
			}

			// 匹配"Accuracy"标签→右侧单元格填充准确度等级（按百分数显示，如 0.02 表示 0.02%）
			if strings.Contains(text, "Accuracy") || strings.Contains(text, "准确度") {
				rightCell := cellName(col+1, row)
				f.SetCellValue(sheet, rightCell, fmt.Sprintf("%.2f", config.PrecisionLevel*100))
				continue
			}

			// 匹配"Equipment Number"或"设备编号"标签→右侧单元格填充设备编号
			if strings.Contains(text, "Equipment Number") || strings.Contains(text, "设备编号") {
				rightCell := cellName(col+1, row)
				if config.DeviceNumber != "" {
					f.SetCellValue(sheet, rightCell, config.DeviceNumber)
				}
				continue
			}
		}
	}
}

// applyReportTemplateUnit 将报告模板的主单位格替换为设备真实单位。
//
// 模板设计：元数据区 "Unit"/“单位” 标签右侧只有一个字面量主单位格
// （如 6s 模板 K5、6m 模板 N5），其余单位格（各通道块 Unit、结果分析
// 表头 “（psi）” 等）均为引用主格的公式（=$K$5、"（"&$K$5&"）"）。
// 因此只需改写主格，再设置 FullCalcOnLoad 让 Excel/WPS 打开文件时
// 强制重算全部公式，所有引用处即联动更新。
//
// 注意：公式引用格不可直接 SetCellValue 覆盖，否则会把公式变成静态值、
// 破坏模板联动；判断依据是右侧格存在公式则跳过。
func applyReportTemplateUnit(f *excelize.File, unit string) {
	if unit == "" || len(f.GetSheetList()) == 0 {
		return
	}
	sheet := f.GetSheetList()[0]

	// 主单位格位于元数据区（第 5 行附近）；扫描前 10 行 × 30 列，
	// 覆盖单程（K5）与回程（N5）两种模板布局。通道块内的 Unit 标签
	// （第 17 行起）右格均为公式，天然被跳过。
	for row := 1; row <= 10; row++ {
		for col := 1; col <= 30; col++ {
			cell := cellName(col, row)
			text, _ := f.GetCellValue(sheet, cell)
			text = strings.TrimSpace(text)
			if !strings.Contains(text, "单位") && !strings.Contains(strings.ToLower(text), "unit") {
				continue
			}
			right := cellName(col+1, row)
			if formula, _ := f.GetCellFormula(sheet, right); formula != "" {
				continue
			}
			f.SetCellValue(sheet, right, unit)
		}
	}

	// 主格改写后，公式格的缓存结果仍是模板旧单位（如 “（psi）”），
	// 必须设置打开时重算，否则用户看到的仍是旧缓存值。
	fullCalc := true
	_ = f.SetCalcProps(&excelize.CalcPropsOptions{FullCalcOnLoad: &fullCalc})
}

// matchDirection 按指定方向过滤压力点。
// direction="forward" 接受 Direction 为空或 "forward"；direction="backward" 仅接受 "backward"。
func matchDirection(pointDirection, direction string) bool {
	if direction == "backward" {
		return pointDirection == "backward"
	}
	// 默认按正程：兼容旧数据 Direction 为空的情况
	return pointDirection == "" || pointDirection == "forward"
}

// collectMeasurementStandardValues 从计量压力点中提取指定方向已完成的标准值。
// 过滤条件与 collectMeasurementChannelData 保持一致，确保标准值和通道数据一一对应。
func collectMeasurementStandardValues(points []domain.PressurePoint, direction string, deviceID ...string) []float64 {
	targetDev := ""
	if len(deviceID) > 0 {
		targetDev = deviceID[0]
	}

	values := make([]float64, 0, len(points))
	for _, p := range points {
		if !matchDirection(p.Direction, direction) || p.Status != "completed" {
			continue
		}
		if _, ok := measurementPointData(&p, targetDev); !ok {
			continue
		}
		values = append(values, p.TargetPressure)
	}
	return values
}

// measurementPointData 返回计量压力点指定设备的采集数据。
// deviceID 为空时回退单设备字段 CollectedData。
func measurementPointData(p *domain.PressurePoint, deviceID string) ([]float64, bool) {
	if deviceID != "" {
		if d, ok := p.CollectedByDevice[deviceID]; ok {
			return d.Collected, true
		}
		return nil, false
	}
	if len(p.CollectedData) > 0 {
		return p.CollectedData, true
	}
	return nil, false
}

// collectMeasurementChannelByTarget 按 (通道, 标准压力) 聚合指定方向的平均值，
// 返回 channels[ch][targetPressure] = avg。键直接使用 float64 的 TargetPressure
// （等距生成时由 RoundToPrecision 量化，存在精确相等保证）。
// 跳过未完成、数据为空、以及无法推断 samplesPerChannel 的异常点。
// deviceID 为空时回退单设备字段。
func collectMeasurementChannelByTarget(points []domain.PressurePoint, numChannels, averageCount int, direction string, deviceID ...string) []map[float64]float64 {
	targetDev := ""
	if len(deviceID) > 0 {
		targetDev = deviceID[0]
	}

	result := make([]map[float64]float64, numChannels)
	for i := range result {
		result[i] = make(map[float64]float64)
	}

	for _, p := range points {
		if !matchDirection(p.Direction, direction) || p.Status != "completed" {
			continue
		}
		data, ok := measurementPointData(&p, targetDev)
		if !ok {
			continue
		}

		samplesPerChannel := averageCount
		if samplesPerChannel <= 0 {
			samplesPerChannel = len(data) / numChannels
		}
		if samplesPerChannel <= 0 {
			log.Printf("report: skip measurement point index=%d direction=%s: cannot infer samplesPerChannel from len(data)=%d, numChannels=%d",
				p.Index, p.Direction, len(data), numChannels)
			continue
		}

		for ch := 0; ch < numChannels; ch++ {
			sum := 0.0
			count := 0
			for sIdx := 0; sIdx < samplesPerChannel; sIdx++ {
				idx := sIdx*numChannels + ch
				if idx < len(data) {
					sum += data[idx]
					count++
				}
			}
			if count == 0 {
				continue
			}
			result[ch][p.TargetPressure] = sum / float64(count)
		}
	}

	return result
}

// collectMeasurementChannelData 从平铺数据中按通道聚合指定方向的平均值。
// 计量模块的数据是平铺格式：sample0_ch0, sample0_ch1, ..., sampleN_ch0, sampleN_ch1。
// direction 为 "forward" 时仅聚合正程点，为 "backward" 时仅聚合回程点。
// 返回顺序与 points 中匹配方向点的出现顺序一致；如需按标准压力查找回程数据，
// 请改用 collectMeasurementChannelByTarget。
// deviceID 为空时回退单设备字段。
func collectMeasurementChannelData(points []domain.PressurePoint, numChannels, averageCount int, direction string, deviceID ...string) [][]float64 {
	targetDev := ""
	if len(deviceID) > 0 {
		targetDev = deviceID[0]
	}

	channels := make([][]float64, numChannels)
	for i := range channels {
		channels[i] = make([]float64, 0)
	}

	for _, p := range points {
		if !matchDirection(p.Direction, direction) || p.Status != "completed" {
			continue
		}
		data, ok := measurementPointData(&p, targetDev)
		if !ok {
			continue
		}

		samplesPerChannel := averageCount
		if samplesPerChannel <= 0 {
			samplesPerChannel = len(data) / numChannels
		}
		// 异常数据点直接跳过，避免向通道写入 0 导致下游对齐错误。
		if samplesPerChannel <= 0 {
			log.Printf("report: skip measurement point index=%d direction=%s: cannot infer samplesPerChannel from len(data)=%d, numChannels=%d",
				p.Index, p.Direction, len(data), numChannels)
			continue
		}

		for ch := 0; ch < numChannels; ch++ {
			sum := 0.0
			count := 0
			for s := 0; s < samplesPerChannel; s++ {
				idx := s*numChannels + ch
				if idx < len(data) {
					sum += data[idx]
					count++
				}
			}
			avg := 0.0
			if count > 0 {
				avg = sum / float64(count)
			}
			channels[ch] = append(channels[ch], avg)
		}
	}

	return channels
}

// collectChannelData 从会话压力点中按通道提取采集数据。
// deviceID 为空时回退单设备字段 CollectedData；非空时读取该设备的设备维度数据。
func collectChannelData(session *calibration.CalibrationSession, deviceID ...string) [][]float64 {
	if len(session.Points) == 0 {
		return nil
	}

	targetDev := ""
	if len(deviceID) > 0 {
		targetDev = deviceID[0]
	}

	// 确定通道数
	numChannels := 0
	for _, p := range session.Points {
		if data, ok := pointDataForReport(&p, targetDev); ok {
			numChannels = len(data)
			break
		}
	}
	if numChannels == 0 {
		return nil
	}

	// 按通道聚合正程数据
	channels := make([][]float64, numChannels)
	for i := range channels {
		channels[i] = make([]float64, 0)
	}

	for _, p := range session.Points {
		if p.Direction == "backward" {
			continue
		}
		data, ok := pointDataForReport(&p, targetDev)
		if !ok {
			continue
		}
		for ch := 0; ch < numChannels && ch < len(data); ch++ {
			channels[ch] = append(channels[ch], data[ch])
		}
	}

	return channels
}

// collectBackwardData 从会话压力点中提取指定通道的回程数据。
// deviceID 为空时回退单设备字段 CollectedData；非空时读取该设备的设备维度数据。
func collectBackwardData(session *calibration.CalibrationSession, channelIdx int, deviceID ...string) []float64 {
	targetDev := ""
	if len(deviceID) > 0 {
		targetDev = deviceID[0]
	}

	var data []float64
	for _, p := range session.Points {
		if p.Direction != "backward" {
			continue
		}
		pData, ok := pointDataForReport(&p, targetDev)
		if !ok {
			continue
		}
		if channelIdx < len(pData) {
			data = append(data, pData[channelIdx])
		}
	}
	return data
}

// pointDataForReport 返回压力点指定设备的采集数据。
// deviceID 为空时回退单设备字段；否则读取设备维度数据。
func pointDataForReport(p *domain.PressurePoint, deviceID string) ([]float64, bool) {
	if deviceID != "" {
		if d, ok := p.CollectedByDevice[deviceID]; ok {
			return d.Collected, true
		}
		return nil, false
	}
	if len(p.CollectedData) > 0 {
		return p.CollectedData, true
	}
	return nil, false
}
