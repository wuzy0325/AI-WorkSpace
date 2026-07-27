// Package interpolation implements the seven-hole probe legacy traversal
// interpolation algorithm. The single authoritative reference is the Python
// implementation (device-lab/skills/seven-hole-probe/seven_hole.py) together
// with SKILL.md in the same directory; behavioral equivalence is proven by
// golden cross-check tests, not by reading this code.
package interpolation

// InterpolationInput carries one sample of seven-hole probe pressures plus
// atmosphere conditions. All probe pressures are gauge pressures in Pa
// (SKILL.md section 1.1); PAtm is absolute pressure in Pa and TAtm is degC
// (SKILL.md section 5). P7 is the center hole; P1..P6 are the outer ring
// holes in 60-degree spacing.
type InterpolationInput struct {
	P1   float64 `json:"P1"`   // outer hole 1 gauge pressure (Pa)
	P2   float64 `json:"P2"`   // outer hole 2 gauge pressure (Pa)
	P3   float64 `json:"P3"`   // outer hole 3 gauge pressure (Pa)
	P4   float64 `json:"P4"`   // outer hole 4 gauge pressure (Pa)
	P5   float64 `json:"P5"`   // outer hole 5 gauge pressure (Pa)
	P6   float64 `json:"P6"`   // outer hole 6 gauge pressure (Pa)
	P7   float64 `json:"P7"`   // center hole gauge pressure (Pa)
	PAtm float64 `json:"Patm"` // atmosphere pressure, absolute (Pa)
	TAtm float64 `json:"Tatm"` // atmosphere temperature (degC)
}

// InterpolationResult is the output of one seven-hole interpolation.
//
// Field semantics differ from the five-hole package: for the seven-hole
// probe Alpha is the sideslip angle and Beta is the angle of attack
// (spec-seven-hole-traversal section 2.2 / appendix A). The field names and
// JSON tags intentionally match the five-hole InterpolationResult so the
// traversal pipeline (API responses, CSV computed columns, frontend views)
// can be reused unchanged.
//
// Theta/Phi 是 PRB 网格原始角度坐标（deg），供前端展示与诊断使用：
//   - 内区（小角度）模式：网格坐标系就是 (alpha, beta)，故 Theta=Alpha、Phi=Beta
//   - 外区（大角度）模式：网格坐标 (theta, phi) 是探头坐标系下的俯仰角与方位角，
//     需经 convertThetaPhiToAlphaBeta 投影到风洞坐标系的 (alpha, beta)；
//     即 Theta/Phi 反映探头感受到的真实气流偏角，Alpha/Beta 是其在风洞坐标系下的分量
//
// 对仅需 Alpha/Beta 的旧调用方（如 wind-daq）而言为向后兼容的字段新增，无需改动。
type InterpolationResult struct {
	Alpha           float64 `json:"alpha"`           // sideslip angle (deg); five-hole meaning: angle of attack
	Beta            float64 `json:"beta"`            // angle of attack (deg); five-hole meaning: sideslip angle
	Theta           float64 `json:"theta"`           // PRB 网格俯仰角（deg）：内区=Alpha，外区=原始 theta
	Phi             float64 `json:"phi"`             // PRB 网格方位角（deg）：内区=Beta，外区=原始 phi
	MachNumber      float64 `json:"machNumber"`      // Mach number
	Velocity        float64 `json:"velocity"`        // flow velocity (m/s)
	DynamicPressure float64 `json:"dynamicPressure"` // dynamic pressure Pt-Ps (Pa)
	TotalPressure   float64 `json:"P0"`              // total pressure, gauge (Pa)
	StaticPressure  float64 `json:"Ps"`              // static pressure, gauge (Pa)
	IsValid         bool    `json:"isValid"`         // whether the result is usable
	Warning         string  `json:"warning,omitempty"`
}

// PrbValidRange describes the angular coverage of the loaded grid.
type PrbValidRange struct {
	AlphaMin float64 `json:"alphaMin"`
	AlphaMax float64 `json:"alphaMax"`
	BetaMin  float64 `json:"betaMin"`
	BetaMax  float64 `json:"betaMax"`
	MachMin  float64 `json:"machMin"`
	MachMax  float64 `json:"machMax"`
}

// Interpolator is the seven-hole interpolation contract, intentionally
// identical in shape to the five-hole package interface (three methods only).
type Interpolator interface {
	// IsLoaded reports whether the full 7-file .prb set has been loaded.
	IsLoaded() bool

	// GetValidRange returns the inner-zone angular range (+/-30 deg from
	// 7.prb). It is for UI display only and MUST NOT be used for post-hoc
	// validity rejection: out-of-zone decisions are made by the polygon
	// tests during mode selection (spec-seven-hole-traversal section 2.2).
	GetValidRange() PrbValidRange

	// Calculate runs one interpolation for the given input.
	Calculate(input InterpolationInput) (InterpolationResult, error)
}

// SevenHolePrbInterpolator is the concrete seven-hole interpolator backed by
// the 7-file .prb set (7.prb inner zone + 1.prb..6.prb outer sectors).
// Loading lands in prb_loader.go; orchestration lands in prb_interpolator.go.
type SevenHolePrbInterpolator struct {
	// inner is the loaded small-angle grid (7.prb); nil until LoadInnerPrbLines.
	inner *innerGrid
	// outer holds the six large-angle sector grids (1.prb..6.prb);
	// outer[i] stays nil until LoadOuterPrbLines(i+1, ...) succeeds.
	outer [outerSectorCount]*outerSector
	// Precomputed inner-zone geometry (spec section 3.4: built once at load
	// time, Calculate never rebuilds): boundary polygon and 144 distorted
	// quadrilateral cells in (ka,kb) space.
	innerPolygon []point2D
	innerQuads   []distortedQuad
	// Precomputed outer-zone geometry per sector: boundary polygon and 36
	// distorted quadrilateral cells each.
	outerPolygons [outerSectorCount][]point2D
	outerQuads    [outerSectorCount][]distortedQuad
	// innerSource / outerSources record the load-time source labels (usually
	// file paths) for the Identity() snapshot identifier. 仅供诊断展示，
	// 不得作为分支开关——历史曾试图用 dataSource 字段区分 PRB/CSV 走不同
	// 插值分支，但会导致相同网格因来源不同得到不同结果，已删除（参见
	// prb_interpolator.go Calculate 注释）。
	innerSource  string
	outerSources [outerSectorCount]string
}
