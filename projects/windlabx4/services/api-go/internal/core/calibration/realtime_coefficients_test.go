package calibration

import "testing"

func realtimeValue(value float64) *float64 {
	return &value
}

func TestCalculateRealtimeCoefficientsCalculatesEveryProbeType(t *testing.T) {
	pTotal := 80.0
	pStatic := 15.0

	tests := []struct {
		name string
		kind CalibrationType
		raw  RealtimeCoefficientInput
	}{
		{
			name: "five-hole",
			kind: TypeFiveHole,
			raw: RealtimeCoefficientInput{
				P1: realtimeValue(100), P2: realtimeValue(200), P3: realtimeValue(100), P4: realtimeValue(110), P5: realtimeValue(90),
				PAtm: realtimeValue(101325), TAtm: realtimeValue(20), PTotal: &pTotal, PStatic: &pStatic,
			},
		},
		{
			name: "three-hole",
			kind: TypeThreeHole,
			raw: RealtimeCoefficientInput{
				P1: realtimeValue(90), P2: realtimeValue(110), P3: realtimeValue(100),
				PAtm: realtimeValue(101325), TAtm: realtimeValue(20), PTotal: &pTotal, PStatic: &pStatic,
			},
		},
		{
			name: "total-pressure",
			kind: TypeTotalPressure,
			raw: RealtimeCoefficientInput{
				PAtm: realtimeValue(101325), TAtm: realtimeValue(20), PTunnelTotal: realtimeValue(200),
				PTunnelStatic: realtimeValue(15), PProbeTotal: realtimeValue(190),
			},
		},
		{
			name: "seven-hole",
			kind: TypeSevenHole,
			raw: RealtimeCoefficientInput{
				P1: realtimeValue(90), P2: realtimeValue(90), P3: realtimeValue(90), P4: realtimeValue(90),
				P5: realtimeValue(90), P6: realtimeValue(90), P7: realtimeValue(110),
				PAtm: realtimeValue(101325), TAtm: realtimeValue(20), PTotal: &pTotal, PStatic: &pStatic,
			},
		},
		{
			name: "total-temperature",
			kind: TypeTotalTemperature,
			raw:  RealtimeCoefficientInput{TestProbeTemp: realtimeValue(105), StandardProbeTemp: realtimeValue(100)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CalculateRealtimeCoefficients(tt.kind, tt.raw)
			if err != nil {
				t.Fatalf("CalculateRealtimeCoefficients() error = %v", err)
			}
			if result == nil {
				t.Fatal("CalculateRealtimeCoefficients() returned nil result")
			}
		})
	}
}

func TestCalculateRealtimeCoefficientsRejectsIncompleteInput(t *testing.T) {
	_, err := CalculateRealtimeCoefficients(TypeFiveHole, RealtimeCoefficientInput{P1: realtimeValue(1), P2: realtimeValue(2)})
	if err == nil {
		t.Fatal("expected incomplete five-hole input to be rejected")
	}
}
