package interpolation

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	coreinterp "wind-daq/services/api-go/internal/core/interpolation"
)

type InterpolationInput = coreinterp.InterpolationInput
type InterpolationResult = coreinterp.InterpolationResult
type PrbValidRange = coreinterp.PrbValidRange
type PrbFileInfo = coreinterp.PrbFileInfo
type PrbInterpolator = coreinterp.PrbInterpolator
type MultiPrbInterpolator = coreinterp.MultiPrbInterpolator
type PrbFileData = coreinterp.PrbFileData
type MultiPrbLoadResult = coreinterp.MultiPrbLoadResult
type MultiPrbInterpolationMode = coreinterp.MultiPrbInterpolationMode

const (
	ModeNearest MultiPrbInterpolationMode = coreinterp.ModeNearest
	ModeLinear  MultiPrbInterpolationMode = coreinterp.ModeLinear
)

func NewPrbInterpolator() *PrbInterpolator {
	return coreinterp.NewPrbInterpolator()
}

func NewMultiPrbInterpolator() *MultiPrbInterpolator {
	return coreinterp.NewMultiPrbInterpolator()
}

func ReadPrbLines(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open PRB file: %w", err)
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
		return nil, fmt.Errorf("read PRB file: %w", err)
	}
	return lines, nil
}

func LoadPrbFile(filePath string) (*PrbInterpolator, error) {
	lines, err := ReadPrbLines(filePath)
	if err != nil {
		return nil, err
	}
	p := NewPrbInterpolator()
	if err := p.LoadPrbLines(lines, filePath); err != nil {
		return nil, err
	}
	return p, nil
}

func GetPrbFileInfo(interpolator *PrbInterpolator) PrbFileInfo {
	return PrbFileInfo{
		FilePath:   "",
		FileName:   "",
		LoadedAt:   0,
		ValidRange: interpolator.GetValidRange(),
	}
}
