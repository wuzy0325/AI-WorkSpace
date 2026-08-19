package calibration

import (
	"bufio"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const fiveHolePointAngleLimit = 60.0

// ParseFiveHolePointFile parses ordered beta/alpha pairs from CSV or whitespace-delimited text.
func ParseFiveHolePointFile(content string) ([]FiveHoleSnakePoint, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	points := make([]FiveHoleSnakePoint, 0)
	lineNumber := 0
	firstContentLine := true

	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if line == "" {
			continue
		}
		fields := splitFiveHolePointFields(line)
		if firstContentLine && isFiveHolePointHeader(fields) {
			firstContentLine = false
			continue
		}
		firstContentLine = false
		if len(fields) != 2 {
			return nil, fmt.Errorf("第 %d 行必须恰好包含两列数据（第一列 β，第二列 α）", lineNumber)
		}

		beta, err := parseFiveHolePointAngle(fields[0], "β", lineNumber)
		if err != nil {
			return nil, err
		}
		alpha, err := parseFiveHolePointAngle(fields[1], "α", lineNumber)
		if err != nil {
			return nil, err
		}
		points = append(points, FiveHoleSnakePoint{
			ID:          len(points) + 1,
			Coordinates: map[string]float64{"α": alpha, "β": beta},
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取布点文件失败: %w", err)
	}
	if len(points) == 0 {
		return nil, fmt.Errorf("布点文件必须至少包含一个点")
	}
	return points, nil
}

func splitFiveHolePointFields(line string) []string {
	if strings.Contains(line, ",") {
		fields := strings.Split(line, ",")
		for index := range fields {
			fields[index] = strings.TrimSpace(fields[index])
		}
		return fields
	}
	return strings.Fields(line)
}

func isFiveHolePointHeader(fields []string) bool {
	if len(fields) != 2 {
		return false
	}
	beta := strings.ToLower(strings.TrimSpace(fields[0]))
	alpha := strings.ToLower(strings.TrimSpace(fields[1]))
	return (beta == "beta" || beta == "β") && (alpha == "alpha" || alpha == "α")
}

func parseFiveHolePointAngle(raw, name string, lineNumber int) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("第 %d 行的 %s 必须是有限数值", lineNumber, name)
	}
	if value < -fiveHolePointAngleLimit || value > fiveHolePointAngleLimit {
		return 0, fmt.Errorf("第 %d 行的 %s=%g 超出允许范围 [-60, 60]", lineNumber, name, value)
	}
	return value, nil
}
