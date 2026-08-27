package report

import (
	"fmt"

	"cal1604/internal/domain"
)

// SelectTemplate 根据测点数量与模式返回模板文件名。
// 模板命名规则: {点数}{模式}.xlsx，如 5s.xlsx（5点单程）、5m.xlsx（5点回程）。
func SelectTemplate(points int, mode string) (string, error) {
	if points < 2 || points > 11 {
		return "", fmt.Errorf("invalid point count: %d (supported: 2-11)", points)
	}

	suffix, err := modeSuffix(mode)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%d%s.xlsx", points, suffix), nil
}

// ListTemplates 返回所有可用模板的文件名列表。
func ListTemplates() []string {
	templates := make([]string, 0, 20)
	for points := 2; points <= 11; points++ {
		templates = append(templates, fmt.Sprintf("%ds.xlsx", points))
		templates = append(templates, fmt.Sprintf("%dm.xlsx", points))
	}
	return templates
}

// GetTemplateInfo 返回模板信息（名称、点数、模式）。
func GetTemplateInfo(filename string) (points int, mode string, err error) {
	var suffix rune
	n, parseErr := fmt.Sscanf(filename, "%d%c.xlsx", &points, &suffix)
	if parseErr != nil || n != 2 {
		return 0, "", fmt.Errorf("invalid template filename: %s", filename)
	}
	switch suffix {
	case 's':
		mode = string(domain.PressureModeSingle)
	case 'm':
		mode = string(domain.PressureModeRoundTrip)
	default:
		return 0, "", fmt.Errorf("invalid template mode suffix: %c", suffix)
	}
	if points < 2 || points > 11 {
		return 0, "", fmt.Errorf("invalid point count in template: %d", points)
	}
	return points, mode, nil
}

func modeSuffix(mode string) (string, error) {
	switch mode {
	case "single", "s":
		return "s", nil
	case "roundTrip", "return", "m":
		return "m", nil
	default:
		return "", fmt.Errorf("invalid pressure mode: %s", mode)
	}
}
