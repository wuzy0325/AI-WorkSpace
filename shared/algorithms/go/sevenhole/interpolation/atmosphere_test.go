package interpolation

import (
	"math"
	"strings"
	"testing"
)

func TestCalVelocityMach(t *testing.T) {
	// Reference point: pt=1000 Pa gauge, ps=0, standard atmosphere.
	v, ma, err := calVelocityMach(1000, 0, 101325, 20)
	if err != nil {
		t.Fatalf("calVelocityMach: %v", err)
	}
	wantV := math.Sqrt(2 * 1000 * 287.06 * 293.15 / 101325)
	if math.Abs(v-wantV)/wantV > 1e-12 {
		t.Errorf("v = %v, want %v", v, wantV)
	}
	ratio := (1000 + 101325.0) / 101325.0
	wantMa := math.Sqrt(5 * (math.Pow(ratio, 0.4/1.4) - 1))
	if math.Abs(ma-wantMa) > 1e-12 {
		t.Errorf("ma = %v, want %v", ma, wantMa)
	}
}

func TestCalVelocityMachGuards(t *testing.T) {
	tests := []struct {
		name              string
		pt, ps, pa, tatm  float64
		wantSub           string
	}{
		{"pt below ps", 100, 200, 101325, 20, "总压低于静压"},
		{"zero atmosphere", 100, 0, 0, 20, "大气压力非法"},
		{"negative atmosphere", 100, 0, -1, 20, "大气压力非法"},
		{"NaN atmosphere", 100, 0, math.NaN(), 20, "大气压力非法"},
		{"kelvin below zero", 100, 0, 101325, -300, "大气温度非法"},
		{"absolute static non-positive", 100, -200000, 101325, 20, "绝对静压非正"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := calVelocityMach(tc.pt, tc.ps, tc.pa, tc.tatm)
			if err == nil {
				t.Fatal("expected guard error")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q missing %q", err.Error(), tc.wantSub)
			}
		})
	}
}

// TestSolveInnerPtPs verifies the closed-form solve against the original
// coefficient equations (residual check, sympy-equivalent, SKILL.md
// section 2.5), including a negative-pressure case mirroring the dataset.
func TestSolveInnerPtPs(t *testing.T) {
	tests := []struct {
		name      string
		in        InterpolationInput
		cpt, cps  float64
	}{
		{
			name: "uniform ring",
			in:   InterpolationInput{P1: 100, P2: 100, P3: 100, P4: 100, P5: 100, P6: 100, P7: 110},
			cpt:  1.1, cps: -0.5,
		},
		{
			name: "mixed pressures",
			in:   InterpolationInput{P1: 10, P2: 60, P3: -20, P4: 130, P5: 0, P6: 45, P7: 88},
			cpt:  0.7, cps: 0.25,
		},
		{
			name: "negative pressures",
			in:   InterpolationInput{P1: -2771, P2: 100, P3: 50, P4: -30, P5: 200, P6: -10, P7: -400},
			cpt:  0.9, cps: -0.2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pt, ps, err := solveInnerPtPs(tc.in, tc.cpt, tc.cps)
			if err != nil {
				t.Fatalf("solveInnerPtPs: %v", err)
			}
			pAvg := (tc.in.P1 + tc.in.P2 + tc.in.P3 + tc.in.P4 + tc.in.P5 + tc.in.P6) / 6
			if got := (tc.in.P7 - pt) / (pt - ps); math.Abs(got-tc.cpt) > 1e-9 {
				t.Errorf("cpt residual: got %v, want %v", got, tc.cpt)
			}
			if got := (ps - pAvg) / (pt - ps); math.Abs(got-tc.cps) > 1e-9 {
				t.Errorf("cps residual: got %v, want %v", got, tc.cps)
			}
		})
	}
}

func TestSolveInnerPtPsGuard(t *testing.T) {
	// D = 1+cpt+cps == 0 triggers the 1e-12 guard (spec section 3.2).
	in := InterpolationInput{P1: 1, P7: 2}
	if _, _, err := solveInnerPtPs(in, 0.5, -1.5); err == nil {
		t.Fatal("expected denominator guard error")
	}
}

// TestSolveOuterPtPs verifies the outer closed-form solve by residuals
// (Python big_cal_ptps, SKILL.md section 3.6), including wrap neighbors.
func TestSolveOuterPtPs(t *testing.T) {
	in := InterpolationInput{P1: 500, P2: 120, P3: 10, P4: 20, P5: 30, P6: 80, P7: 40}
	for _, n := range []int{1, 3, 6} {
		cpt, cps := 0.8, -0.3
		pt, ps, err := solveOuterPtPs(in, n, cpt, cps)
		if err != nil {
			t.Fatalf("sector %d: %v", n, err)
		}
		pc := holePressure(in, n)
		pMid := (holePressure(in, n-1) + holePressure(in, n+1)) / 2
		if got := (pc - pt) / (pt - ps); math.Abs(got-cpt) > 1e-9 {
			t.Errorf("sector %d cpt residual: got %v, want %v", n, got, cpt)
		}
		if got := (ps - pMid) / (pt - ps); math.Abs(got-cps) > 1e-9 {
			t.Errorf("sector %d cps residual: got %v, want %v", n, got, cps)
		}
	}
	if _, _, err := solveOuterPtPs(in, 1, -0.5, -0.5); err == nil {
		t.Fatal("expected denominator guard error")
	}
}
