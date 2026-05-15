package calibration

import (
	"math"
	"testing"
)

const tol = 1e-9

func nearlyEqual(a, b float64) bool {
	return math.Abs(a-b) <= tol
}

// ── CalculateFiveHoleCoefficients ──

func TestCalculateFiveHoleCoefficients_Valid(t *testing.T) {
	raw := FiveHoleRawData{P1: 101200, P2: 101150, P3: 101050, P4: 101120, P5: 101080, PAtm: 100000, PTotal: 101500}
	c := CalculateFiveHoleCoefficients(raw)
	if !nearlyEqual(c.Kalpha, 1.0) {
		t.Errorf("Kalpha=%.10f, want 1.0", c.Kalpha)
	}
	if !nearlyEqual(c.Kbeta, 0.4) {
		t.Errorf("Kbeta=%.10f, want 0.4", c.Kbeta)
	}
	if !nearlyEqual(c.CPT, 0.8) {
		t.Errorf("CPT=%.10f, want 0.8", c.CPT)
	}
	if !nearlyEqual(c.CPS, 11.0) {
		t.Errorf("CPS=%.10f, want 11.0", c.CPS)
	}
}

func TestCalculateFiveHoleCoefficients_DenomNearZero(t *testing.T) {
	raw := FiveHoleRawData{P1: 101100, P2: 101150, P3: 101050, P4: 101120, P5: 101080, PAtm: 100000, PTotal: 101500}
	c := CalculateFiveHoleCoefficients(raw)
	if !nearlyEqual(c.Kalpha, 1e8) {
		t.Errorf("Kalpha=%.10f, want 1e8", c.Kalpha)
	}
	if !nearlyEqual(c.Kbeta, 4e7) {
		t.Errorf("Kbeta=%.10f, want 4e7", c.Kbeta)
	}
	if !nearlyEqual(c.CPT, 1100.0/1500.0) {
		t.Errorf("CPT=%.10f, want %.10f", c.CPT, 1100.0/1500.0)
	}
	if !nearlyEqual(c.CPS, 1.1e9) {
		t.Errorf("CPS=%.10f, want 1.1e9", c.CPS)
	}
}

func TestCalculateFiveHoleCoefficients_PTotalZero(t *testing.T) {
	t.Run("P1 greater than PAtm", func(t *testing.T) {
		raw := FiveHoleRawData{P1: 101200, P2: 101150, P3: 101050, P4: 101120, P5: 101080, PAtm: 100000, PTotal: 0}
		c := CalculateFiveHoleCoefficients(raw)
		if !nearlyEqual(c.Kalpha, 1.0) {
			t.Errorf("Kalpha=%.10f, want 1.0", c.Kalpha)
		}
		if c.CPT != 1.0 {
			t.Errorf("CPT=%.10f, want 1.0", c.CPT)
		}
		if !nearlyEqual(c.CPS, 11.0) {
			t.Errorf("CPS=%.10f, want 11.0", c.CPS)
		}
	})
	t.Run("P1 not greater than PAtm", func(t *testing.T) {
		raw := FiveHoleRawData{P1: 100000, P2: 100100, P3: 99900, P4: 100060, P5: 99940, PAtm: 100000, PTotal: 0}
		c := CalculateFiveHoleCoefficients(raw)
		if c.CPT != 0 {
			t.Errorf("CPT=%.10f, want 0", c.CPT)
		}
		if c.CPS != 0 {
			t.Errorf("CPS=%.10f, want 0", c.CPS)
		}
	})
}

func TestCalculateFiveHoleCoefficients_P1EqualsPAtm(t *testing.T) {
	raw := FiveHoleRawData{P1: 100000, P2: 100100, P3: 99900, P4: 100060, P5: 99940, PAtm: 100000, PTotal: 101500}
	c := CalculateFiveHoleCoefficients(raw)
	if c.CPT != 0 {
		t.Errorf("CPT=%.10f, want 0", c.CPT)
	}
	if c.CPS != 0 {
		t.Errorf("CPS=%.10f, want 0", c.CPS)
	}
}

// ── CalculateThreeHoleCoefficients ──

func TestCalculateThreeHoleCoefficients_Valid(t *testing.T) {
	raw := ThreeHoleRawData{P1: 101200, P2: 101150, P3: 101050, PAtm: 100000, PTotal: 101500}
	c := CalculateThreeHoleCoefficients(raw)
	if !nearlyEqual(c.K, 100.0/1200.0) {
		t.Errorf("K=%.10f, want %.10f", c.K, 100.0/1200.0)
	}
	if !nearlyEqual(c.Cv, 0.8) {
		t.Errorf("Cv=%.10f, want 0.8", c.Cv)
	}
	if !nearlyEqual(c.Cp, 0.8) {
		t.Errorf("Cp=%.10f, want 0.8", c.Cp)
	}
}

func TestCalculateThreeHoleCoefficients_DenomNearZero(t *testing.T) {
	raw := ThreeHoleRawData{P1: 100000, P2: 100100, P3: 99900, PAtm: 100000, PTotal: 101500}
	c := CalculateThreeHoleCoefficients(raw)
	if !nearlyEqual(c.K, 2e8) {
		t.Errorf("K=%.10f, want 2e8", c.K)
	}
	if c.Cv != 0 {
		t.Errorf("Cv=%.10f, want 0", c.Cv)
	}
	if c.Cp != 0 {
		t.Errorf("Cp=%.10f, want 0", c.Cp)
	}
}

func TestCalculateThreeHoleCoefficients_PTotalZero(t *testing.T) {
	t.Run("P1 not equal to PAtm", func(t *testing.T) {
		raw := ThreeHoleRawData{P1: 101200, P2: 101150, P3: 101050, PAtm: 100000, PTotal: 0}
		c := CalculateThreeHoleCoefficients(raw)
		if c.Cv != 0 {
			t.Errorf("Cv=%.10f, want 0", c.Cv)
		}
		if c.Cp != 1.0 {
			t.Errorf("Cp=%.10f, want 1.0", c.Cp)
		}
	})
	t.Run("P1 equals PAtm", func(t *testing.T) {
		raw := ThreeHoleRawData{P1: 100000, P2: 100100, P3: 99900, PAtm: 100000, PTotal: 0}
		c := CalculateThreeHoleCoefficients(raw)
		if c.Cv != 0 {
			t.Errorf("Cv=%.10f, want 0", c.Cv)
		}
		if c.Cp != 0 {
			t.Errorf("Cp=%.10f, want 0", c.Cp)
		}
	})
}

// ── CalculateTotalPressureCoefficients ──

func TestCalculateTotalPressureCoefficients_Valid(t *testing.T) {
	raw := TotalPressureRawData{PTunnelTotal: 2000, PProbeTotal: 1950, PTunnelStatic: 1500, PAtm: 100000}
	c := CalculateTotalPressureCoefficients(raw)
	if !nearlyEqual(c.CPT, 0.975) {
		t.Errorf("CPT=%.10f, want 0.975", c.CPT)
	}
	wantErr := -50.0 / 102000.0 * 100.0
	if !nearlyEqual(c.Error, wantErr) {
		t.Errorf("Error=%.10f, want %.10f", c.Error, wantErr)
	}
	wantMa := math.Sqrt(1000.0 / (1.4 * 101500.0))
	if !nearlyEqual(c.MachNumber, wantMa) {
		t.Errorf("MachNumber=%.10f, want %.10f", c.MachNumber, wantMa)
	}
}

func TestCalculateTotalPressureCoefficients_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		raw  TotalPressureRawData
		want TotalPressureCoefficients
	}{
		{
			name: "zero tunnel total pressure",
			raw:  TotalPressureRawData{PTunnelTotal: 0, PProbeTotal: 100, PTunnelStatic: 500, PAtm: 100000},
			want: TotalPressureCoefficients{CPT: 0, Error: 0.1, MachNumber: 0},
		},
		{
			name: "zero absolute tunnel total pressure",
			raw:  TotalPressureRawData{PTunnelTotal: -100000, PProbeTotal: -100000, PTunnelStatic: -100000, PAtm: 100000},
			want: TotalPressureCoefficients{CPT: 1, Error: 0, MachNumber: 0},
		},
		{
			name: "zero dynamic pressure q=0",
			raw:  TotalPressureRawData{PTunnelTotal: 1000, PProbeTotal: 1000, PTunnelStatic: 1000, PAtm: 100000},
			want: TotalPressureCoefficients{CPT: 1, Error: 0, MachNumber: 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateTotalPressureCoefficients(tt.raw)
			if !nearlyEqual(got.CPT, tt.want.CPT) {
				t.Errorf("CPT=%.10f, want %.10f", got.CPT, tt.want.CPT)
			}
			if !nearlyEqual(got.Error, tt.want.Error) {
				t.Errorf("Error=%.10f, want %.10f", got.Error, tt.want.Error)
			}
			if !nearlyEqual(got.MachNumber, tt.want.MachNumber) {
				t.Errorf("MachNumber=%.10f, want %.10f", got.MachNumber, tt.want.MachNumber)
			}
		})
	}
}

// ── CalculateMachNumber ──

func TestCalculateMachNumber(t *testing.T) {
	tests := []struct {
		name           string
		totalPressure  float64
		staticPressure float64
		want           float64
	}{
		{
			name:           "valid ratio 2",
			totalPressure:  200000,
			staticPressure: 100000,
			want:           math.Sqrt(5.0 * (math.Pow(2.0, 2.0/7.0) - 1.0)),
		},
		{
			name:           "zero static pressure",
			totalPressure:  100000,
			staticPressure: 0,
			want:           0,
		},
		{
			name:           "negative static pressure",
			totalPressure:  100000,
			staticPressure: -1000,
			want:           0,
		},
		{
			name:           "total less than static",
			totalPressure:  50000,
			staticPressure: 100000,
			want:           0,
		},
		{
			name:           "ratio equals 1",
			totalPressure:  100000,
			staticPressure: 100000,
			want:           0,
		},
		{
			name:           "zero total pressure",
			totalPressure:  0,
			staticPressure: 100000,
			want:           0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateMachNumber(tt.totalPressure, tt.staticPressure)
			if !nearlyEqual(got, tt.want) {
				t.Errorf("CalculateMachNumber(%v,%v)=%.10f, want %.10f",
					tt.totalPressure, tt.staticPressure, got, tt.want)
			}
		})
	}
}

// ── CalculateRecoveryCoefficient ──

func TestCalculateRecoveryCoefficient(t *testing.T) {
	t.Run("valid temperatures", func(t *testing.T) {
		got := CalculateRecoveryCoefficient(25, 30)
		want := (25 + 273.15) / (30 + 273.15)
		if !nearlyEqual(got, want) {
			t.Errorf("r=%.10f, want %.10f", got, want)
		}
	})
	t.Run("zero total temperature in Kelvin", func(t *testing.T) {
		got := CalculateRecoveryCoefficient(-273.15, -273.15)
		if got != 0 {
			t.Errorf("r=%.10f, want 0", got)
		}
	})
}

// ── CalculateAverage ──

func TestCalculateAverage(t *testing.T) {
	t.Run("non-empty slice", func(t *testing.T) {
		got := CalculateAverage([]float64{10, 20, 30, 40})
		if !nearlyEqual(got, 25.0) {
			t.Errorf("avg=%.10f, want 25", got)
		}
	})
	t.Run("empty slice", func(t *testing.T) {
		got := CalculateAverage([]float64{})
		if got != 0 {
			t.Errorf("avg=%.10f, want 0", got)
		}
	})
}

// ── CalculateStdDev ──

func TestCalculateStdDev(t *testing.T) {
	t.Run("multiple values", func(t *testing.T) {
		vals := []float64{10, 12, 23, 23, 16, 23, 21, 16}
		got := CalculateStdDev(vals)
		want := math.Sqrt(192.0 / 7.0)
		if !nearlyEqual(got, want) {
			t.Errorf("stdDev=%.10f, want %.10f", got, want)
		}
	})
	t.Run("single value", func(t *testing.T) {
		got := CalculateStdDev([]float64{42})
		if got != 0 {
			t.Errorf("stdDev=%.10f, want 0", got)
		}
	})
	t.Run("empty slice", func(t *testing.T) {
		got := CalculateStdDev([]float64{})
		if got != 0 {
			t.Errorf("stdDev=%.10f, want 0", got)
		}
	})
}

// ── CheckTemperatureStability ──

func TestCheckTemperatureStability(t *testing.T) {
	t.Run("stable", func(t *testing.T) {
		if !CheckTemperatureStability([]float64{25.0, 25.1, 25.05}, 0.1) {
			t.Error("expected stable")
		}
	})
	t.Run("unstable", func(t *testing.T) {
		if CheckTemperatureStability([]float64{20.0, 30.0}, 1.0) {
			t.Error("expected unstable")
		}
	})
}

// ── IsSphereTankGateSatisfied ──

func TestIsSphereTankGateSatisfied(t *testing.T) {
	t.Run("nil gate", func(t *testing.T) {
		if !IsSphereTankGateSatisfied(nil, 0) {
			t.Error("nil gate should return true")
		}
	})
	t.Run("disabled gate", func(t *testing.T) {
		gate := &SphereTankGateConfig{Enabled: false, WaitTimeSec: 10}
		if !IsSphereTankGateSatisfied(gate, 0) {
			t.Error("disabled gate should return true")
		}
	})
	t.Run("enabled before wait time", func(t *testing.T) {
		gate := &SphereTankGateConfig{Enabled: true, WaitTimeSec: 10}
		if IsSphereTankGateSatisfied(gate, 5) {
			t.Error("expected false when stableTime < WaitTimeSec")
		}
	})
	t.Run("enabled after wait time", func(t *testing.T) {
		gate := &SphereTankGateConfig{Enabled: true, WaitTimeSec: 10}
		if !IsSphereTankGateSatisfied(gate, 15) {
			t.Error("expected true when stableTime >= WaitTimeSec")
		}
	})
	t.Run("enabled exactly at wait time", func(t *testing.T) {
		gate := &SphereTankGateConfig{Enabled: true, WaitTimeSec: 10}
		if !IsSphereTankGateSatisfied(gate, 10) {
			t.Error("expected true when stableTime == WaitTimeSec")
		}
	})
}

// ── CalculateFiveHoleAverage ──

func TestCalculateFiveHoleAverage(t *testing.T) {
	t.Run("2 samples", func(t *testing.T) {
		samples := []FiveHoleRawData{
			{P1: 100, P2: 110, P3: 90, P4: 105, P5: 95, PAtm: 101300, TAtm: 25, PTotal: 101500},
			{P1: 102, P2: 112, P3: 92, P4: 107, P5: 97, PAtm: 101300, TAtm: 25, PTotal: 101500},
		}
		got := CalculateFiveHoleAverage(samples)
		want := FiveHoleRawData{P1: 101, P2: 111, P3: 91, P4: 106, P5: 96, PAtm: 101300, TAtm: 25, PTotal: 101500}
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})
	t.Run("1 sample", func(t *testing.T) {
		samples := []FiveHoleRawData{
			{P1: 100, P2: 110, P3: 90, P4: 105, P5: 95, PAtm: 101300, TAtm: 25, PTotal: 101500},
		}
		got := CalculateFiveHoleAverage(samples)
		if got != samples[0] {
			t.Errorf("got %+v, want %+v", got, samples[0])
		}
	})
	t.Run("0 samples", func(t *testing.T) {
		got := CalculateFiveHoleAverage([]FiveHoleRawData{})
		if got != (FiveHoleRawData{}) {
			t.Errorf("got %+v, want zero value", got)
		}
	})
}

// ── CalculateThreeHoleAverage ──

func TestCalculateThreeHoleAverage(t *testing.T) {
	t.Run("2 samples", func(t *testing.T) {
		samples := []ThreeHoleRawData{
			{P1: 100, P2: 110, P3: 90, PAtm: 101300, PTotal: 101500},
			{P1: 102, P2: 112, P3: 92, PAtm: 101300, PTotal: 101500},
		}
		got := CalculateThreeHoleAverage(samples)
		want := ThreeHoleRawData{P1: 101, P2: 111, P3: 91, PAtm: 101300, PTotal: 101500}
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})
	t.Run("0 samples", func(t *testing.T) {
		got := CalculateThreeHoleAverage([]ThreeHoleRawData{})
		if got != (ThreeHoleRawData{}) {
			t.Errorf("got %+v, want zero value", got)
		}
	})
}

// ── CalculateCoefficientsStdDev ──

func TestCalculateCoefficientsStdDev(t *testing.T) {
	t.Run("3 coefficient sets", func(t *testing.T) {
		coeffs := []FiveHoleCoefficients{
			{Kalpha: 1.0, Kbeta: 0.5, CPT: 0.8, CPS: 10.0},
			{Kalpha: 1.1, Kbeta: 0.6, CPT: 0.9, CPS: 11.0},
			{Kalpha: 0.9, Kbeta: 0.4, CPT: 0.7, CPS: 9.0},
		}
		got := CalculateCoefficientsStdDev(coeffs)
		want := FiveHoleCoefficientsStdDev{Kalpha: 0.1, Kbeta: 0.1, CPT: 0.1, CPS: 1.0}
		if !nearlyEqual(got.Kalpha, want.Kalpha) {
			t.Errorf("Kalpha=%.10f, want %.10f", got.Kalpha, want.Kalpha)
		}
		if !nearlyEqual(got.Kbeta, want.Kbeta) {
			t.Errorf("Kbeta=%.10f, want %.10f", got.Kbeta, want.Kbeta)
		}
		if !nearlyEqual(got.CPT, want.CPT) {
			t.Errorf("CPT=%.10f, want %.10f", got.CPT, want.CPT)
		}
		if !nearlyEqual(got.CPS, want.CPS) {
			t.Errorf("CPS=%.10f, want %.10f", got.CPS, want.CPS)
		}
	})
	t.Run("less than 2 sets", func(t *testing.T) {
		coeffs := []FiveHoleCoefficients{{Kalpha: 1.0, Kbeta: 0.5, CPT: 0.8, CPS: 10.0}}
		got := CalculateCoefficientsStdDev(coeffs)
		if got != (FiveHoleCoefficientsStdDev{}) {
			t.Errorf("got %+v, want zero value", got)
		}
	})
}

// ── CalculateThreeHoleCoefficientsStdDev ──

func TestCalculateThreeHoleCoefficientsStdDev(t *testing.T) {
	t.Run("3 sets", func(t *testing.T) {
		coeffs := []ThreeHoleCoefficients{
			{K: 1.0, Cv: 0.8, Cp: 0.8},
			{K: 1.1, Cv: 0.9, Cp: 0.9},
			{K: 0.9, Cv: 0.7, Cp: 0.7},
		}
		got := CalculateThreeHoleCoefficientsStdDev(coeffs)
		want := ThreeHoleCoefficientsStdDev{K: 0.1, Cv: 0.1, Cp: 0.1}
		if !nearlyEqual(got.K, want.K) {
			t.Errorf("K=%.10f, want %.10f", got.K, want.K)
		}
		if !nearlyEqual(got.Cv, want.Cv) {
			t.Errorf("Cv=%.10f, want %.10f", got.Cv, want.Cv)
		}
		if !nearlyEqual(got.Cp, want.Cp) {
			t.Errorf("Cp=%.10f, want %.10f", got.Cp, want.Cp)
		}
	})
	t.Run("less than 2 sets", func(t *testing.T) {
		coeffs := []ThreeHoleCoefficients{{K: 1.0, Cv: 0.8, Cp: 0.8}}
		got := CalculateThreeHoleCoefficientsStdDev(coeffs)
		if got != (ThreeHoleCoefficientsStdDev{}) {
			t.Errorf("got %+v, want zero value", got)
		}
	})
}

// ── CheckTemperatureStabilityWithResult ──

func TestCheckTemperatureStabilityWithResult(t *testing.T) {
	t.Run("stable", func(t *testing.T) {
		samples := []float64{25.0, 25.1, 25.05}
		r := CheckTemperatureStabilityWithResult(samples, 0.1)
		if !r.Stable {
			t.Error("expected stable=true")
		}
		if !nearlyEqual(r.StdDev, 0.05) {
			t.Errorf("StdDev=%.10f, want 0.05", r.StdDev)
		}
	})
	t.Run("unstable", func(t *testing.T) {
		samples := []float64{20.0, 30.0}
		r := CheckTemperatureStabilityWithResult(samples, 1.0)
		if r.Stable {
			t.Error("expected stable=false")
		}
		wantStdDev := math.Sqrt(50.0)
		if !nearlyEqual(r.StdDev, wantStdDev) {
			t.Errorf("StdDev=%.10f, want %.10f", r.StdDev, wantStdDev)
		}
	})
	t.Run("less than 2 samples", func(t *testing.T) {
		r := CheckTemperatureStabilityWithResult([]float64{25.0}, 0.1)
		if !r.Stable {
			t.Error("expected stable=true for <2 samples")
		}
		if r.StdDev != 0 {
			t.Errorf("StdDev=%.10f, want 0", r.StdDev)
		}
	})
}
