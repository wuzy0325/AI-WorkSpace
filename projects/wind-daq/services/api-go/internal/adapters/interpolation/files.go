package interpolation

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	coreinterp "wind-daq/services/api-go/internal/core/interpolation"
)

func LoadPrbFile(filePath string) (*coreinterp.PrbInterpolator, error) {
	lines, err := readNonEmptyLines(filePath)
	if err != nil {
		return nil, err
	}
	interpolator := coreinterp.NewPrbInterpolator()
	if err := interpolator.LoadPrbLines(lines, filePath); err != nil {
		return nil, err
	}
	return interpolator, nil
}

func LoadFiveHoleNewFile(filePath string) (*coreinterp.FiveHoleNewInterpolator, error) {
	lines, err := readNonEmptyLines(filePath)
	if err != nil {
		return nil, err
	}
	interpolator := coreinterp.NewFiveHoleNewInterpolator()
	if err := interpolator.LoadPrbLines(lines); err != nil {
		return nil, err
	}
	return interpolator, nil
}

func LoadMultiPrbFiles(filePaths []string, machNumbers []float64) (*coreinterp.MultiPrbInterpolator, *coreinterp.MultiPrbLoadResult, error) {
	data := make([]coreinterp.PrbFileData, 0, len(filePaths))
	for _, filePath := range filePaths {
		lines, err := readNonEmptyLines(filePath)
		if err != nil {
			return nil, nil, err
		}
		data = append(data, coreinterp.PrbFileData{
			FilePath: filePath,
			Lines:    lines,
		})
	}
	interpolator := coreinterp.NewMultiPrbInterpolator()
	result, err := interpolator.LoadPrbData(data, machNumbers)
	if err != nil {
		return nil, nil, err
	}
	return interpolator, result, nil
}

func readNonEmptyLines(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open calibration file: %w", err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read calibration file: %w", err)
	}
	return lines, nil
}
