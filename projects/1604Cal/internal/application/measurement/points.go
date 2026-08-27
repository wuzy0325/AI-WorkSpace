package measurement

import (
	"fmt"
	"strconv"

	"cal1604/internal/domain"
	apperrors "cal1604/internal/errors"
)

// SetConfig 更新计量模块当前配置。
func (s *Service) SetConfig(config domain.WorkflowConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = config
	s.points = nil
	s.session = nil
	s.alarmPending = false
	s.currentAlarm = nil
}

// GetConfig 返回当前计量配置快照。
func (s *Service) GetConfig() domain.WorkflowConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	config := s.config
	if len(config.Channels) > 0 {
		config.Channels = append([]int(nil), config.Channels...)
	}
	return config
}

// GeneratePressurePoints 根据 measurement 自己的配置生成测点计划。
func (s *Service) GeneratePressurePoints() ([]domain.PressurePoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	points, err := generatePointsFromConfig(s.config)
	if err != nil {
		return nil, err
	}

	s.points = points
	result := make([]domain.PressurePoint, len(points))
	copy(result, points)
	return result, nil
}

// GetPoints 返回当前计量测点快照。
func (s *Service) GetPoints() []domain.PressurePoint {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]domain.PressurePoint, len(s.points))
	copy(result, s.points)
	return result
}

func generatePointsFromConfig(config domain.WorkflowConfig) ([]domain.PressurePoint, error) {
	if len(config.CustomPoints) > 0 {
		return generatePointsFromCustom(config)
	}

	if config.PointCount < 2 {
		return nil, fmt.Errorf("%w: point count must be at least 2, got %d", apperrors.ErrInvalidArgument, config.PointCount)
	}

	precision := config.Precision
	if precision < 0 {
		precision = 0
	}

	roundTrip := config.PressureMode == domain.PressureModeRoundTrip
	points := domain.EquidistantPoints(config.MinPressure, config.MaxPressure, config.PointCount, precision, roundTrip)

	for i := range points {
		points[i].ID = "measurement-point-" + strconv.Itoa(i+1)
	}

	return points, nil
}

// generatePointsFromCustom 根据用户自定义压力值生成测点计划。
func generatePointsFromCustom(config domain.WorkflowConfig) ([]domain.PressurePoint, error) {
	if len(config.CustomPoints) < 1 {
		return nil, fmt.Errorf("%w: custom points must not be empty", apperrors.ErrInvalidArgument)
	}

	precision := config.Precision
	if precision < 0 {
		precision = 0
	}

	points := make([]domain.PressurePoint, 0, len(config.CustomPoints)*2)
	for i, pressure := range config.CustomPoints {
		rounded := domain.RoundToPrecision(pressure, precision)
		points = append(points, domain.PressurePoint{
			ID:             "measurement-point-" + strconv.Itoa(i+1),
			Index:          i + 1,
			TargetPressure: rounded,
			Direction:      pointDirection(config, i),
			Status:         domain.PointStatusPending,
		})
	}

	if config.PressureMode != domain.PressureModeRoundTrip || len(config.CustomPoints) != config.PointCount {
		return points, nil
	}

	for i := len(config.CustomPoints) - 1; i >= 0; i-- {
		rounded := domain.RoundToPrecision(config.CustomPoints[i], precision)
		points = append(points, domain.PressurePoint{
			ID:             "measurement-point-" + strconv.Itoa(len(points)+1),
			Index:          len(points) + 1,
			TargetPressure: rounded,
			Direction:      "backward",
			Status:         domain.PointStatusPending,
		})
	}

	return points, nil
}

func pointDirection(config domain.WorkflowConfig, index int) string {
	if config.PressureMode != domain.PressureModeRoundTrip || config.PointCount <= 0 || index < config.PointCount {
		return "forward"
	}
	return "backward"
}
