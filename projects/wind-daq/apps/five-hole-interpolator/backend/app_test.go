package backend

import (
	"fmt"
	"testing"

	wind_interp "wind-daq/services/api-go/pkg/interpolation"
)

func TestBatchCalculateReturnsDataPayload(t *testing.T) {
	interpolator := wind_interp.NewMultiPrbInterpolator()
	loadResult, err := interpolator.LoadPrbData([]wind_interp.PrbFileData{
		{FilePath: "synthetic-0.3Ma.prb", Lines: syntheticPrbLines(0.05, 0.01)},
	}, []float64{0.3})
	if err != nil {
		t.Fatalf("load PRB data: %v", err)
	}
	if len(loadResult.Warnings) > 1 {
		t.Fatalf("unexpected load warnings: %v", loadResult.Warnings)
	}

	app := &App{multiInterp: interpolator}
	response := app.BatchCalculate([]InterpolationInput{{
		P1: 100, P2: 300, P3: 100, P4: 100, P5: 100,
		Patm: 101325, Tatm: 20,
	}})

	if !response.Success {
		t.Fatalf("expected success, got error %q", response.Error)
	}
	if len(response.Data) != 1 {
		t.Fatalf("expected one result in data payload, got %d", len(response.Data))
	}
	if !response.Data[0].IsValid {
		t.Fatalf("expected valid interpolation result, got warning %q", response.Data[0].Warning)
	}
}

func syntheticPrbLines(cpt, cps float64) []string {
	lines := []string{"13 13"}
	for alpha := -30.0; alpha <= 30; alpha += 5 {
		for beta := -30.0; beta <= 30; beta += 5 {
			lines = append(lines, fmt.Sprintf("%.6f %.6f %.6f %.6f %.0f %.0f",
				alpha/100, beta/100, cpt, cps, alpha, beta))
		}
	}
	return lines
}
