package calibration

import (
	"fmt"
	"math"
)

// RealtimeCoefficientInput is a single unaveraged DAQ snapshot used only for display.
type RealtimeCoefficientInput struct {
	P1, P2, P3, P4, P5, P6, P7 *float64
	PAtm, TAtm                 *float64
	PTotal, PStatic            *float64
	PTunnelTotal               *float64
	PTunnelStatic              *float64
	TTunnel                    *float64
	PProbeTotal                *float64
	TestProbeTemp              *float64
	StandardProbeTemp          *float64
}

type RealtimeSevenHoleCoefficients struct {
	Coefficients SevenHoleCoefficients `json:"coefficients"`
	Region       string                `json:"region"`
	Sector       int                   `json:"sector"`
}

// CalculateRealtimeCoefficients evaluates the existing calibration formulas for one
// current DAQ snapshot. It does not create a calibration point or persist any data.
func CalculateRealtimeCoefficients(kind CalibrationType, input RealtimeCoefficientInput) (any, error) {
	switch kind {
	case TypeFiveHole:
		if !required(input.P1, input.P2, input.P3, input.P4, input.P5, input.PAtm) {
			return nil, fmt.Errorf("five-hole realtime input is incomplete")
		}
		return CalculateFiveHoleCoefficients(FiveHoleRawData{
			P1: value(input.P1), P2: value(input.P2), P3: value(input.P3), P4: value(input.P4), P5: value(input.P5),
			PAtm: value(input.PAtm), TAtm: value(input.TAtm), PTotal: input.PTotal, PStatic: input.PStatic,
		}), nil
	case TypeThreeHole:
		if !required(input.P1, input.P2, input.P3, input.PAtm) {
			return nil, fmt.Errorf("three-hole realtime input is incomplete")
		}
		return CalculateThreeHoleCoefficients(ThreeHoleRawData{
			P1: value(input.P1), P2: value(input.P2), P3: value(input.P3), PAtm: value(input.PAtm), TAtm: value(input.TAtm),
			PTotal: input.PTotal, PStatic: input.PStatic,
		}), nil
	case TypeTotalPressure:
		if !required(input.PAtm, input.PTunnelTotal, input.PTunnelStatic, input.PProbeTotal) {
			return nil, fmt.Errorf("total-pressure realtime input is incomplete")
		}
		return CalculateTotalPressureCoefficients(TotalPressureRawData{
			PAtm: value(input.PAtm), TAtm: value(input.TAtm), PTunnelTotal: value(input.PTunnelTotal),
			PTunnelStatic: value(input.PTunnelStatic), TTunnel: value(input.TTunnel), PProbeTotal: value(input.PProbeTotal),
		}), nil
	case TypeSevenHole:
		if !required(input.P1, input.P2, input.P3, input.P4, input.P5, input.P6, input.P7, input.PAtm) {
			return nil, fmt.Errorf("seven-hole realtime input is incomplete")
		}
		raw := SevenHoleRawData{P1: value(input.P1), P2: value(input.P2), P3: value(input.P3), P4: value(input.P4), P5: value(input.P5), P6: value(input.P6), P7: value(input.P7), PAtm: value(input.PAtm), TAtm: value(input.TAtm), PTotal: input.PTotal, PStatic: input.PStatic}
		if value(input.P7) >= maxOuterPressure(input) {
			coefficients, err := CalculateSevenHoleInnerCoefficients(raw)
			return RealtimeSevenHoleCoefficients{Coefficients: coefficients, Region: "inner", Sector: 7}, err
		}
		sector := maxOuterPressureIndex(input)
		coefficients, err := CalculateSevenHoleOuterCoefficients(raw, sector)
		return RealtimeSevenHoleCoefficients{Coefficients: coefficients, Region: "outer", Sector: sector}, err
	case TypeTotalTemperature:
		if !required(input.TestProbeTemp, input.StandardProbeTemp) {
			return nil, fmt.Errorf("total-temperature realtime input is incomplete")
		}
		coefficient, err := CalculateRecoveryCoefficient(value(input.TestProbeTemp), value(input.StandardProbeTemp))
		return coefficient, err
	default:
		return nil, fmt.Errorf("unsupported calibration type %q", kind)
	}
}

func required(values ...*float64) bool {
	for _, value := range values {
		if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
			return false
		}
	}
	return true
}

func value(input *float64) float64 {
	if input == nil {
		return 0
	}
	return *input
}

func maxOuterPressure(input RealtimeCoefficientInput) float64 {
	values := []float64{value(input.P1), value(input.P2), value(input.P3), value(input.P4), value(input.P5), value(input.P6)}
	max := values[0]
	for _, value := range values[1:] {
		if value > max {
			max = value
		}
	}
	return max
}

func maxOuterPressureIndex(input RealtimeCoefficientInput) int {
	values := []float64{value(input.P1), value(input.P2), value(input.P3), value(input.P4), value(input.P5), value(input.P6)}
	index := 0
	for i := 1; i < len(values); i++ {
		if values[i] > values[index] {
			index = i
		}
	}
	return index + 1
}
