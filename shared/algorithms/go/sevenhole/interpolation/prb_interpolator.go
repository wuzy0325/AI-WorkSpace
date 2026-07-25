package interpolation

import (
	"fmt"
	"math"
	"strings"
)

// IsLoaded reports whether the full 7-file .prb set (inner zone + all six
// outer sectors) has been loaded.
func (p *SevenHolePrbInterpolator) IsLoaded() bool {
	if p.inner == nil {
		return false
	}
	for _, sec := range p.outer {
		if sec == nil {
			return false
		}
	}
	return true
}

// GetValidRange returns the inner-zone angular range read from the 7.prb
// grid corners (+/-30 deg for the standard grid). It is for UI display only
// and MUST NOT be used for post-hoc validity rejection (spec section 2.2):
// out-of-zone decisions are made by the polygon tests during mode selection,
// and valid outer-zone results may exceed this range after the coordinate
// transform.
func (p *SevenHolePrbInterpolator) GetValidRange() PrbValidRange {
	if p.inner == nil {
		return PrbValidRange{}
	}
	g := p.inner
	return PrbValidRange{
		AlphaMin: g.points[0][0].a,
		AlphaMax: g.points[innerGridSide-1][0].a,
		BetaMin:  g.points[0][0].b,
		BetaMax:  g.points[0][innerGridSide-1].b,
		MachMin:  0,
		MachMax:  0,
	}
}

// Identity returns a stable identifier containing the inner-zone source and
// the six sector sources (optional capability consumed via
// interface{ Identity() string }, spec section 5.1). It is deliberately NOT
// part of the Interpolator interface.
func (p *SevenHolePrbInterpolator) Identity() string {
	if p.inner == nil {
		return "seven-hole-prb(unloaded)"
	}
	parts := make([]string, 0, 7)
	parts = append(parts, "7="+p.innerSource)
	for i, s := range p.outerSources {
		parts = append(parts, fmt.Sprintf("%d=%s", i+1, s))
	}
	return "seven-hole-prb(" + strings.Join(parts, ";") + ")"
}

// GetInnerPointCount 返回内区实际加载的网格点数（13×13=169，固定）。
// 未加载时返回 0。供上层 metadata 报告真实点数，避免 usecase/api 层硬编码。
func (p *SevenHolePrbInterpolator) GetInnerPointCount() int {
	if p.inner == nil {
		return 0
	}
	return innerPointCount // 物理设计固定 169 点
}

// GetOuterPointCount 返回指定扇区（1..6）外区实际加载的网格点数
// （= thetaCount × outerPhiCount，动态）。未加载该扇区时返回 0。
// 供上层 metadata 报告真实点数，支持 4×13=52、7×13=91 等不同校准集。
func (p *SevenHolePrbInterpolator) GetOuterPointCount(sector int) int {
	if sector < 1 || sector > outerSectorCount {
		return 0
	}
	sec := p.outer[sector-1]
	if sec == nil {
		return 0
	}
	return sec.thetaCount * outerPhiCount
}

// validateFiniteInput rejects NaN/Inf in any input field. Negative gauge
// pressures and zero values are legal inputs (spec section 7.2); atmosphere
// plausibility is guarded inside calVelocityMach.
func validateFiniteInput(in InterpolationInput) error {
	fields := []struct {
		name string
		v    float64
	}{
		{"P1", in.P1}, {"P2", in.P2}, {"P3", in.P3}, {"P4", in.P4},
		{"P5", in.P5}, {"P6", in.P6}, {"P7", in.P7},
		{"Patm", in.PAtm}, {"Tatm", in.TAtm},
	}
	for _, f := range fields {
		if math.IsNaN(f.v) || math.IsInf(f.v, 0) {
			return fmt.Errorf("输入字段 %s 为非有限数值: %v", f.name, f.v)
		}
	}
	return nil
}

// solveInnerPtPs solves total/static pressure from cpt/cps for the inner
// zone (Python little_cal_ptps sympy system, SKILL.md section 2.5, closed
// form): cpt=(p7-pt)/(pt-ps), cps=(ps-pAvg)/(pt-ps), pAvg=mean(p1..p6).
// Linearizing in (pt,ps) yields the shared denominator D=1+cpt+cps.
func solveInnerPtPs(in InterpolationInput, cpt, cps float64) (pt, ps float64, err error) {
	pAvg := (in.P1 + in.P2 + in.P3 + in.P4 + in.P5 + in.P6) / 6
	d := 1 + cpt + cps
	if math.Abs(d) < 1e-12 {
		return 0, 0, fmt.Errorf("小角度模式: |1+cpt+cps|=%.6e < 1e-12", d)
	}
	pt = (in.P7*(1+cps) + cpt*pAvg) / d
	ps = (in.P7*cps + pAvg*(1+cpt)) / d
	return pt, ps, nil
}

// solveOuterPtPs solves total/static pressure for outer sector n (Python
// big_cal_ptps sympy system, SKILL.md section 3.6, closed form):
// cpt=(pc-pt)/(pt-ps), cps=(ps-(pl+pr)/2)/(pt-ps) with wrap-around
// neighbors.
func solveOuterPtPs(in InterpolationInput, n int, cpt, cps float64) (pt, ps float64, err error) {
	pc := holePressure(in, n)
	pMid := (holePressure(in, n-1) + holePressure(in, n+1)) / 2
	d := 1 + cpt + cps
	if math.Abs(d) < 1e-12 {
		return 0, 0, fmt.Errorf("大角度模式孔%d: |1+cpt+cps|=%.6e < 1e-12", n, d)
	}
	pt = (pc*(1+cps) + cpt*pMid) / d
	ps = (pc*cps + pMid*(1+cpt)) / d
	return pt, ps, nil
}

// Calculate runs one seven-hole interpolation (Python cal_ab orchestration,
// SKILL.md section 4): inner-zone try first, then the two candidate outer
// sectors in descending-pressure order. Out-of-grid inputs return
// IsValid=false with a warning (no extrapolation, spec section 4); guard
// violations return errors. Panics are recovered into errors (aligned with
// the five-hole PrbInterpolator.Calculate).
func (p *SevenHolePrbInterpolator) Calculate(input InterpolationInput) (result InterpolationResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			result = InterpolationResult{}
			err = fmt.Errorf("七孔插值计算内部panic: %v", r)
		}
	}()
	if err := validateFiniteInput(input); err != nil {
		return InterpolationResult{}, err
	}
	if !p.IsLoaded() {
		return InterpolationResult{}, fmt.Errorf("七孔PRB校准数据未加载")
	}

	// Inner-zone try (Python cal_ab branch order: inner first).
	ka, kb, err := innerKaKb(input)
	if err != nil {
		return InterpolationResult{}, err
	}
	zc, inZone, err := p.innerZoneInterpolate(ka, kb)
	if err != nil {
		return InterpolationResult{}, err
	}
	if inZone {
		pt, ps, err := solveInnerPtPs(input, zc.cpt, zc.cps)
		if err != nil {
			return InterpolationResult{}, err
		}
		return assembleResult(input, zc.a, zc.b, pt, ps)
	}

	// Large-angle mode: try the largest-pressure hole's sector first, then
	// the second-largest (Python first/second candidate logic).
	first, second := maxPressureHoles(input)
	for _, sector := range [2]int{first, second} {
		kaO, kbO, err := outerKaKb(input, sector)
		if err != nil {
			return InterpolationResult{}, err
		}
		zc, hit, err := p.outerZoneTrySector(sector, kaO, kbO)
		if err != nil {
			return InterpolationResult{}, err
		}
		if !hit {
			continue
		}
		pt, ps, err := solveOuterPtPs(input, sector, zc.cpt, zc.cps)
		if err != nil {
			return InterpolationResult{}, err
		}
		// zc.a/zc.b are grid coordinates (theta,phi); transform to output
		// angles (Python big_ab_convert, SKILL.md section 3.7).
		alpha, beta := convertThetaPhiToAlphaBeta(zc.a, zc.b)
		return assembleResult(input, alpha, beta, pt, ps)
	}

	return InterpolationResult{
		IsValid: false,
		Warning: "压力系数超出七孔PRB校准网格范围，不支持外推",
	}, nil
}

// assembleResult computes V/Ma and packs the final result.
func assembleResult(input InterpolationInput, alpha, beta, pt, ps float64) (InterpolationResult, error) {
	v, ma, err := calVelocityMach(pt, ps, input.PAtm, input.TAtm)
	if err != nil {
		return InterpolationResult{}, err
	}
	return InterpolationResult{
		Alpha:           alpha,
		Beta:            beta,
		MachNumber:      ma,
		Velocity:        v,
		TotalPressure:   pt,
		StaticPressure:  ps,
		DynamicPressure: pt - ps,
		IsValid:         true,
	}, nil
}
