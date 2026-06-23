package interpolation

import (
	"fmt"
	"math"
	"testing"
)

type customerRow struct {
	pa        float64
	ta        float64
	ptInlet   float64
	psInlet   float64
	p1        float64
	p2        float64
	p3        float64
	ptCust    float64
	psCust    float64
	maCust    float64
	alphaCust float64
}

func getCustomerData() []customerRow {
	return []customerRow{
		{101425, 293.98, 50047.0, -2207.70, 49544.2, 34010.8, 7804.1, 0, 0, 0, 0},
		{101425, 293.98, 50145.4, -2207.70, 49883, 37295.7, 9399, 0, 0, 0, 0},
		{101425, 293.98, 50057.8, -2207.60, 49075.7, 41869.9, 16029.7, 0, 0, 0, 0},
		{101425, 293.98, 50015.8, -2208.00, 47193, 45376.7, 22801.7, 0, 0, 0, 0},
		{101425, 293.98, 50022.2, -2208.40, 44785, 47882.3, 27803.8, 0, 0, 0, 0},
		{101425, 293.98, 49984.9, -2208.80, 41441.8, 49350.9, 32263.6, 0, 0, 0, 0},
		{101425, 293.98, 49948.5, -2208.90, 37227.7, 49908.4, 36503.7, 0, 0, 0, 0},
		{101425, 293.98, 49844.8, -2208.90, 32789, 49680.6, 40447.8, 0, 0, 0, 0},
		{101425, 293.98, 50014.6, -2209.30, 27987.7, 48849.9, 43880.2, 0, 0, 0, 0},
		{101425, 293.98, 49935.9, -2209.30, 23187, 47048.2, 46417.7, 0, 0, 0, 0},
		{101425, 293.98, 49951.3, -2209.20, 18133.8, 44248.6, 48234.2, 0, 0, 0, 0},
		{101425, 293.98, 50025.6, -2209.70, 12213.1, 40701.6, 49472.4, 0, 0, 0, 0},
		{101425, 293.98, 50118.1, -2210.00, 3900.7, 35640.6, 49907.8, 0, 0, 0, 0},
	}
}

type calibRow struct {
	alpha   float64
	ma      float64
	pt      float64
	ps      float64
	p1      float64
	p2      float64
	p3      float64
	kbTable float64
	k0Table float64
	kvTable float64
}

func getCalibData() []calibRow {
	return []calibRow{
		{-30, 0.802, 151472.0, 99217.3, 150969.2, 135435.8, 109229.1, -3.911, 1.502, 4.896},
		{-25, 0.802, 151570.4, 99217.3, 151308.0, 138720.7, 110824.0, -2.644, 0.839, 3.420},
		{-20, 0.802, 151482.8, 99217.4, 150500.7, 143294.9, 117454.7, -1.773, 0.439, 2.805},
		{-15, 0.801, 151373.3, 99217.0, 148618.0, 146801.7, 124226.7, -1.175, 0.220, 2.513},
		{-10, 0.801, 151447.2, 99216.6, 146210.0, 149307.3, 129228.8, -0.733, 0.092, 2.254},
		{-5, 0.801, 151409.9, 99216.2, 142866.8, 150775.9, 133688.6, -0.367, 0.025, 2.088},
		{0, 0.801, 151412.5, 99216.1, 138652.7, 151333.4, 137928.7, -0.028, 0.003, 2.001},
		{5, 0.800, 151269.8, 99216.8, 134214.0, 151105.6, 141872.8, 0.293, 0.006, 1.992},
		{10, 0.801, 151439.6, 99215.7, 129412.7, 150274.9, 145305.2, 0.615, 0.045, 2.022},
		{15, 0.801, 151360.9, 99215.7, 124612.0, 148473.2, 147842.7, 0.949, 0.118, 2.129},
		{20, 0.801, 151376.3, 99215.8, 119558.8, 145673.6, 149659.2, 1.360, 0.258, 2.357},
		{25, 0.801, 151450.6, 99215.3, 113638.1, 142126.6, 150897.4, 1.890, 0.473, 2.649},
		{30, 0.802, 151543.1, 99215.0, 105325.7, 137065.6, 151332.8, 2.633, 0.829, 2.995},
	}
}

func buildCalibInterpolator() *ThreeHoleInterpolator {
	calib := getCalibData()
	var prbLines []string
	prbLines = append(prbLines, "0.801")
	prbLines = append(prbLines, fmt.Sprintf("%d", len(calib)))
	for _, row := range calib {
		prbLines = append(prbLines, fmt.Sprintf("%.9f\t%.9f\t%.9f\t%.0f",
			row.kbTable, row.k0Table, row.kvTable, row.alpha))
	}

	interp := NewThreeHoleInterpolator()
	_, _ = interp.LoadPrbData([]PrbFileData{
		{FilePath: "0.8.prb", Lines: prbLines},
	})
	return interp
}

func TestCustomerData_KbAnalysis(t *testing.T) {
	rows := getCustomerData()

	t.Log("========== Customer data Kβ analysis ==========")
	t.Log("")
	t.Logf("%-4s %-10s %-10s %-10s | %-12s %-12s | %-10s %-10s",
		"Row", "P1", "P2", "P3", "deltaP", "Kβ calc", "Kβ range", "Out of bounds")

	kbMin := -3.911
	kbMax := 2.633

	for i, row := range rows {
		deltaP := 2*row.p2 - row.p1 - row.p3
		kb := (row.p3 - row.p1) / deltaP
		outOfRange := ""
		if kb < kbMin {
			outOfRange = fmt.Sprintf("← below min (%.3f)", kbMin)
		} else if kb > kbMax {
			outOfRange = fmt.Sprintf("← above max (%.3f)", kbMax)
		}

		t.Logf("%-4d %-10.1f %-10.1f %-10.1f | %-12.2f %-12.6f | [%.3f,%.3f] %s",
			i+1, row.p1, row.p2, row.p3, deltaP, kb, kbMin, kbMax, outOfRange)
	}
}

func TestCustomerData_OurInterpolation(t *testing.T) {
	rows := getCustomerData()
	interp := buildCalibInterpolator()

	t.Log("========== Our algorithm interpolation on customer data (P1/P2/P3 original order) ==========")
	t.Log("")
	t.Logf("%-4s %-8s %-8s | %-10s %-10s %-10s | %-10s %-10s | %-10s %-10s | %-4s %s",
		"Row", "Alpha", "Cust α", "Ma", "Cust Ma", "Ma err", "Pt", "Cust Pt", "Ps", "Cust Ps", "Iter", "Warning")

	for i, row := range rows {
		input := InterpolationInput{
			P1:   row.p1,
			P2:   row.p2,
			P3:   row.p3,
			PAtm: row.pa,
			TAtm: row.ta,
		}

		res, err := interp.Calculate(input)
		if err != nil {
			t.Errorf("Row %d: Calculate failed: %v", i+1, err)
			continue
		}

		maErr := math.Abs(res.MachNumber - row.maCust)

		t.Logf("%-4d %-8.3f %-8.2f | %-10.6f %-10.5f %-10.5f | %-10.2f %-10.2f | %-10.2f %-10.2f | %-4d %s",
			i+1, res.Alpha, row.alphaCust,
			res.MachNumber, row.maCust, maErr,
			res.TotalPressure, row.ptCust,
			res.StaticPressure, row.psCust,
			res.IterationCount, res.Warning)
	}
}

func TestCustomerData_SwappedP1P3(t *testing.T) {
	rows := getCustomerData()
	interp := buildCalibInterpolator()

	t.Log("========== Interpolation results after swapping P1/P3 ==========")
	t.Log("")

	t.Logf("%-4s %-10s %-10s | %-10s %-10s | %-10s %-10s %-10s | %-4s %s",
		"Row", "Alpha orig", "Alpha swap", "Ma orig", "Ma swap", "Pt orig", "Pt swap", "Ps swap", "Iter", "Warning")

	for i, row := range rows {
		inputOrig := InterpolationInput{
			P1:   row.p1,
			P2:   row.p2,
			P3:   row.p3,
			PAtm: row.pa,
			TAtm: row.ta,
		}
		inputSwap := InterpolationInput{
			P1:   row.p3,
			P2:   row.p2,
			P3:   row.p1,
			PAtm: row.pa,
			TAtm: row.ta,
		}

		resOrig, _ := interp.Calculate(inputOrig)
		resSwap, _ := interp.Calculate(inputSwap)

		alphaOrig := resOrig.Alpha
		maOrig := resOrig.MachNumber
		ptOrig := resOrig.TotalPressure

		t.Logf("%-4d %-10.3f %-10.3f | %-10.6f %-10.6f | %-10.2f %-10.2f %-10.2f | %-4d %s",
			i+1, alphaOrig, resSwap.Alpha,
			maOrig, resSwap.MachNumber,
			ptOrig, resSwap.TotalPressure, resSwap.StaticPressure,
			resSwap.IterationCount, resSwap.Warning)
	}
}

func TestCustomerData_PressureCompare(t *testing.T) {
	rows := getCustomerData()
	calib := getCalibData()
	pa := 101425.0

	t.Log("========== Customer hole pressure vs calibration hole pressure (gauge) comparison ==========")
	t.Log("")

	t.Logf("%-4s %-8s | %-10s %-10s %-8s | %-10s %-10s %-8s | %-10s %-10s %-8s",
		"Row", "Exp α", "P1 cust", "P1 calib", "Err%", "P2 cust", "P2 calib", "Err%", "P3 cust", "P3 calib", "Err%")

	for i, row := range rows {
		deltaP := 2*row.p2 - row.p1 - row.p3
		kb := (row.p3 - row.p1) / deltaP
		expectedAlpha := interpAlphaFromKb(calib, kb)

		calibP1 := getCalibP(calib, expectedAlpha, func(r calibRow) float64 { return r.p1 }) - pa
		calibP2 := getCalibP(calib, expectedAlpha, func(r calibRow) float64 { return r.p2 }) - pa
		calibP3 := getCalibP(calib, expectedAlpha, func(r calibRow) float64 { return r.p3 }) - pa

		p1Err := pctErr(row.p1, calibP1)
		p2Err := pctErr(row.p2, calibP2)
		p3Err := pctErr(row.p3, calibP3)

		t.Logf("%-4d %-8.1f | %-10.1f %-10.1f %-8.1f%% | %-10.1f %-10.1f %-8.1f%% | %-10.1f %-10.1f %-8.1f%%",
			i+1, expectedAlpha,
			row.p1, calibP1, p1Err,
			row.p2, calibP2, p2Err,
			row.p3, calibP3, p3Err)
	}
}

func TestCustomerData_Diagnosis(t *testing.T) {
	rows := getCustomerData()
	interp := buildCalibInterpolator()

	t.Log("========== Diagnosis: customer angle calculation error analysis ==========")
	t.Log("")

	t.Logf("%-4s %-10s %-10s %-10s | %-12s %-8s | %-8s %-8s | %-8s %-8s %-8s",
		"Row", "P1", "P2", "P3", "Kβ", "Kβ OOB?", "Alpha ours", "Alpha cust", "Pt ours", "Pt cust", "Pt inlet")

	for i, row := range rows {
		deltaP := 2*row.p2 - row.p1 - row.p3
		kb := (row.p3 - row.p1) / deltaP

		outOfRange := "no"
		if kb < -3.911 || kb > 2.633 {
			outOfRange = "YES!"
		}

		input := InterpolationInput{
			P1:   row.p1,
			P2:   row.p2,
			P3:   row.p3,
			PAtm: row.pa,
			TAtm: row.ta,
		}

		res, _ := interp.Calculate(input)
		ourAlpha := res.Alpha
		ourPt := res.TotalPressure

		t.Logf("%-4d %-10.1f %-10.1f %-10.1f | %-12.6f %-8s | %-8.3f %-8.2f | %-8.1f %-8.1f %-8.1f",
			i+1, row.p1, row.p2, row.p3, kb, outOfRange,
			ourAlpha, row.alphaCust,
			ourPt, row.ptCust, row.ptInlet)
	}
}

func interpAlphaFromKb(calib []calibRow, kb float64) float64 {
	if kb <= calib[0].kbTable {
		return calib[0].alpha
	}
	if kb >= calib[len(calib)-1].kbTable {
		return calib[len(calib)-1].alpha
	}
	for i := 0; i < len(calib)-1; i++ {
		if kb >= calib[i].kbTable && kb <= calib[i+1].kbTable {
			r := (kb - calib[i].kbTable) / (calib[i+1].kbTable - calib[i].kbTable)
			return calib[i].alpha + r*(calib[i+1].alpha-calib[i].alpha)
		}
	}
	return 0
}

func getCalibP(calib []calibRow, alpha float64, getVal func(calibRow) float64) float64 {
	if alpha <= calib[0].alpha {
		return getVal(calib[0])
	}
	if alpha >= calib[len(calib)-1].alpha {
		return getVal(calib[len(calib)-1])
	}
	for i := 0; i < len(calib)-1; i++ {
		if alpha >= calib[i].alpha && alpha <= calib[i+1].alpha {
			r := (alpha - calib[i].alpha) / (calib[i+1].alpha - calib[i].alpha)
			return getVal(calib[i]) + r*(getVal(calib[i+1])-getVal(calib[i]))
		}
	}
	return 0
}

func pctErr(actual, expected float64) float64 {
	if expected == 0 {
		return 0
	}
	return math.Abs(actual-expected) / math.Abs(expected) * 100
}
