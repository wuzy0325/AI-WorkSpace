package report

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// ReportType 报告类型
type ReportType string

const (
	TypeCalibration ReportType = "calibration"
	TypeTraversal   ReportType = "traversal"
	TypeDataSummary ReportType = "data-summary"
)

// ReportData 报告数据
type ReportData struct {
	Type      ReportType        `json:"type"`
	Title     string            `json:"title"`
	Generated time.Time         `json:"generated"`
	Sections  []ReportSection   `json:"sections"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// ReportSection 报告章节
type ReportSection struct {
	Title   string `json:"title"`
	Content string `json:"content"` // Markdown 格式内容
}

// Service 报告生成服务
type Service struct {
	outputDir string
}

// NewService 创建报告服务
func NewService(outputDir string) *Service {
	return &Service{outputDir: outputDir}
}

// GenerateReport 生成报告文件（Markdown 格式，可后续转 PDF）
func (s *Service) GenerateReport(data ReportData) (string, error) {
	if data.Generated.IsZero() {
		data.Generated = time.Now()
	}

	filename := fmt.Sprintf("%s_%s_%s.md",
		data.Type,
		data.Generated.Format("20060102_150405"),
		sanitizeFilename(data.Title),
	)

	if err := os.MkdirAll(s.outputDir, 0755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	path := filepath.Join(s.outputDir, filename)

	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("create report file: %w", err)
	}
	defer f.Close()

	// 写入报告头
	fmt.Fprintf(f, "# %s\n\n", data.Title)
	fmt.Fprintf(f, "**报告类型**: %s  \n", data.Type)
	fmt.Fprintf(f, "**生成时间**: %s  \n\n", data.Generated.Format("2006-01-02 15:04:05"))

	// 写入元数据
	if len(data.Metadata) > 0 {
		fmt.Fprintf(f, "## 基本信息\n\n")
		fmt.Fprintln(f, "| 项目 | 值 |")
		fmt.Fprintln(f, "|------|-----|")
		for k, v := range data.Metadata {
			fmt.Fprintf(f, "| %s | %s |\n", k, v)
		}
		fmt.Fprintln(f)
	}

	// 写入各章节
	for i, section := range data.Sections {
		fmt.Fprintf(f, "## %d. %s\n\n", i+1, section.Title)
		fmt.Fprintf(f, "%s\n\n", section.Content)
	}

	slog.Info("Report generated", "path", path, "type", data.Type)
	return path, nil
}

// GenerateCalibrationReport 生成校准报告
func (s *Service) GenerateCalibrationReport(
	calibType string,
	config map[string]interface{},
	results map[string]interface{},
) (string, error) {
	data := ReportData{
		Type:      TypeCalibration,
		Title:     fmt.Sprintf("校准报告 - %s", calibType),
		Generated: time.Now(),
		Metadata:  map[string]string{"校准类型": calibType},
	}

	// 配置章节
	var configContent string
	for k, v := range config {
		configContent += fmt.Sprintf("- **%s**: %v\n", k, v)
	}
	data.Sections = append(data.Sections, ReportSection{
		Title:   "校准配置",
		Content: configContent,
	})

	// 结果章节
	var resultContent string
	for k, v := range results {
		resultContent += fmt.Sprintf("- **%s**: %v\n", k, v)
	}
	data.Sections = append(data.Sections, ReportSection{
		Title:   "校准结果",
		Content: resultContent,
	})

	return s.GenerateReport(data)
}

// GenerateTraversalReport 生成位移测试报告
func (s *Service) GenerateTraversalReport(
	config map[string]interface{},
	results map[string]interface{},
) (string, error) {
	data := ReportData{
		Type:      TypeTraversal,
		Title:     "位移测试报告",
		Generated: time.Now(),
	}

	var configContent string
	for k, v := range config {
		configContent += fmt.Sprintf("- **%s**: %v\n", k, v)
	}
	data.Sections = append(data.Sections, ReportSection{
		Title:   "测试配置",
		Content: configContent,
	})

	var resultContent string
	for k, v := range results {
		resultContent += fmt.Sprintf("- **%s**: %v\n", k, v)
	}
	data.Sections = append(data.Sections, ReportSection{
		Title:   "测试结果",
		Content: resultContent,
	})

	return s.GenerateReport(data)
}

// sanitizeFilename 清理文件名
func sanitizeFilename(name string) string {
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name) && len(result) < 50; i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			result = append(result, c)
		}
	}
	if len(result) == 0 {
		return "report"
	}
	return string(result)
}
