package usecase

import "windlabx4/services/api-go/internal/core/calibration"

// CalculateRealtimeCoefficients evaluates one current DAQ snapshot for display.
// The API layer delegates here so it does not own calibration domain behavior.
func (m *CalibrationManager) CalculateRealtimeCoefficients(kind calibration.CalibrationType, input calibration.RealtimeCoefficientInput) (any, error) {
	return calibration.CalculateRealtimeCoefficients(kind, input)
}
