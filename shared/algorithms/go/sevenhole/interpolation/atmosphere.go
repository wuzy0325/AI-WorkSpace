package interpolation

import (
	"fmt"
	"math"
)

// Physical constants (SKILL.md section 7).
const (
	gasConstantR    = 287.06 // air gas constant, J/(kg*K)
	gammaAir        = 1.4    // air specific heat ratio
	celsiusToKelvin = 273.15
)

// calVelocityMach computes flow velocity and Mach number (Python
// cal_velocity_mach, SKILL.md section 5):
//
//	V  = sqrt(2*(pt-ps)*R*(t+273.15)/pa)
//	Ma = sqrt(5*(((pt+pa)/(ps+pa))^(0.4/1.4) - 1))
//
// Python wraps both radicands in math.fabs; Go deliberately does not
// replicate that defect (SKILL.md section 5, spec section 4): pt<ps, a
// non-positive absolute denominator, a pressure ratio < 1, or a negative
// radicand all return an error instead of being silently absolved.
func calVelocityMach(pt, ps, patm, tatm float64) (v, ma float64, err error) {
	if math.IsNaN(patm) || math.IsInf(patm, 0) || patm <= 0 {
		return 0, 0, fmt.Errorf("大气压力非法: pa=%.6g", patm)
	}
	tempK := tatm + celsiusToKelvin
	if math.IsNaN(tempK) || math.IsInf(tempK, 0) || tempK <= 0 {
		return 0, 0, fmt.Errorf("大气温度非法: t=%.6g degC", tatm)
	}
	delta := pt - ps
	if delta < 0 {
		return 0, 0, fmt.Errorf("总压低于静压 (pt < ps): pt=%.6g, ps=%.6g", pt, ps)
	}
	vSq := 2 * delta * gasConstantR * tempK / patm
	if vSq < 0 {
		return 0, 0, fmt.Errorf("速度根号内为负: %.6e", vSq)
	}
	v = math.Sqrt(vSq)

	psAbs := ps + patm
	if psAbs <= 0 {
		return 0, 0, fmt.Errorf("绝对静压非正: ps+pa=%.6g (ps=%.6g, pa=%.6g)", psAbs, ps, patm)
	}
	ratio := (pt + patm) / psAbs
	if ratio < 1 {
		return 0, 0, fmt.Errorf("压力比 %.6g < 1 (pt=%.6g, ps=%.6g, pa=%.6g)", ratio, pt, ps, patm)
	}
	maSq := 5 * (math.Pow(ratio, 0.4/gammaAir) - 1)
	if maSq < 0 {
		return 0, 0, fmt.Errorf("马赫数根号内为负: %.6e", maSq)
	}
	ma = math.Sqrt(maSq)
	return v, ma, nil
}
