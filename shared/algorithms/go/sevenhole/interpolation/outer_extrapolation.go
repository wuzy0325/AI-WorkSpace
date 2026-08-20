package interpolation

import (
	"fmt"
	"math"
)

const (
	extrapolationEpsilon   = 1e-12
	outerExtrapolationStep = 5.0
)

func outerThetaMax(sec *outerSector) float64 {
	return outerThetaMin + gridStep*float64(sec.thetaCount-1)
}

// outerZoneExtrapolate inverts the outermost theta cell and extrapolates its
// coefficient fields. The returned zoneCoefficients.a is the clamped theta;
// thetaRaw and phi preserve the inferred probe coordinates for presentation.
func (p *SevenHolePrbInterpolator) outerZoneExtrapolate(sector int, ka, kb float64) (zoneCoefficients, float64, float64, bool, error) {
	if sector < 1 || sector > outerSectorCount {
		return zoneCoefficients{}, 0, 0, false, fmt.Errorf("大角度模式孔号非法: %d", sector)
	}
	sec := p.outer[sector-1]
	if sec == nil || sec.thetaCount < 2 {
		return zoneCoefficients{}, 0, 0, false, fmt.Errorf("大角度模式孔%d: 外区校准网格不足", sector)
	}

	thetaMax := outerThetaMax(sec)
	thetaLo := thetaMax - gridStep
	best := extrapolationCandidate{}
	for ip := 0; ip < outerPhiCount-1; ip++ {
		candidate, ok := invertOuterExtrapolationCell(sec, ip, thetaLo, thetaMax, ka, kb)
		if !ok || candidate.theta <= thetaMax+gridEps {
			continue
		}
		if !best.ok || betterExtrapolationCandidate(candidate, best, sec.centerPhi) {
			best = candidate
		}
	}
	if !best.ok {
		return zoneCoefficients{}, 0, 0, false, nil
	}

	thetaClamp := math.Min(best.theta, thetaMax+outerExtrapolationStep)
	cpt, cps := extrapolatedOuterCptCps(sec, best.ip, thetaClamp, best.phi)
	return zoneCoefficients{a: thetaClamp, b: normalize360(best.phi), cpt: cpt, cps: cps}, best.theta, normalize360(best.phi), true, nil
}

func betterExtrapolationCandidate(candidate, current extrapolationCandidate, center float64) bool {
	if candidate.residual != current.residual {
		return candidate.residual < current.residual
	}
	return math.Abs(unwrapPhiNearCenter(candidate.phi, center)-center) <
		math.Abs(unwrapPhiNearCenter(current.phi, center)-center)
}

type extrapolationCandidate struct {
	theta, phi, residual float64
	ip                   int
	ok                   bool
}

func invertOuterExtrapolationCell(sec *outerSector, ip int, thetaLo, thetaHi, ka, kb float64) (extrapolationCandidate, bool) {
	a := sec.points[sec.thetaCount-2][ip]
	b := sec.points[sec.thetaCount-2][ip+1]
	c := sec.points[sec.thetaCount-1][ip]
	d := sec.points[sec.thetaCount-1][ip+1]
	dTheta := thetaHi - thetaLo
	dPhi := -gridStep
	a1 := (c.ka - a.ka) / dTheta
	b1 := (b.ka - a.ka) / dPhi
	a2 := (c.kb - a.kb) / dTheta
	b2 := (b.kb - a.kb) / dPhi
	den := a1*b2 - a2*b1
	if math.Abs(den) < extrapolationEpsilon {
		return extrapolationCandidate{}, false
	}
	dka, dkb := ka-a.ka, kb-a.kb
	dThetaValue := (dka*b2 - dkb*b1) / den
	dPhiValue := (a1*dkb - a2*dka) / den
	if dPhiValue < -gridStep-gridEps || dPhiValue > gridEps {
		return extrapolationCandidate{}, false
	}
	theta := thetaLo + dThetaValue
	phi := sec.points[sec.thetaCount-2][ip].b + dPhiValue
	phi = unwrapPhiNearCenter(phi, sec.centerPhi)
	if math.IsNaN(theta) || math.IsInf(theta, 0) || math.IsNaN(phi) || math.IsInf(phi, 0) {
		return extrapolationCandidate{}, false
	}
	kaFit, kbFit := bilinearOuterDirection(a, b, c, d, dThetaValue, dPhiValue)
	residual := math.Hypot(kaFit-ka, kbFit-kb)
	return extrapolationCandidate{theta: theta, phi: phi, residual: residual, ip: ip, ok: true}, true
}

func bilinearOuterDirection(a, b, c, d gridPoint, thetaOffset, phiOffset float64) (float64, float64) {
	u := thetaOffset / gridStep
	v := phiOffset / -gridStep
	ka := a.ka + v*(b.ka-a.ka) + u*((c.ka+v*(d.ka-c.ka))-(a.ka+v*(b.ka-a.ka)))
	kb := a.kb + v*(b.kb-a.kb) + u*((c.kb+v*(d.kb-c.kb))-(a.kb+v*(b.kb-a.kb)))
	return ka, kb
}

func extrapolatedOuterCptCps(sec *outerSector, ip int, theta, phi float64) (float64, float64) {
	a := sec.points[sec.thetaCount-2][ip]
	b := sec.points[sec.thetaCount-2][ip+1]
	c := sec.points[sec.thetaCount-1][ip]
	d := sec.points[sec.thetaCount-1][ip+1]
	phiOffset := phi - a.b
	if phiOffset > 180 {
		phiOffset -= 360
	} else if phiOffset < -180 {
		phiOffset += 360
	}
	u := (theta - a.a) / gridStep
	v := phiOffset / -gridStep
	return bilinearValue(a.cpt, b.cpt, c.cpt, d.cpt, u, v), bilinearValue(a.cps, b.cps, c.cps, d.cps, u, v)
}

func bilinearValue(a, b, c, d, u, v float64) float64 {
	lo := a + v*(b-a)
	hi := c + v*(d-c)
	return lo + u*(hi-lo)
}

func unwrapPhiNearCenter(phi, center float64) float64 {
	for phi-center > 180 {
		phi -= 360
	}
	for phi-center < -180 {
		phi += 360
	}
	return phi
}
