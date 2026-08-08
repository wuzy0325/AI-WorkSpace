package interpolation

import (
	"strings"
	"testing"
)

func TestRecomputeSevenHoleInnerCoeffsRejectsDegeneratePressureRows(t *testing.T) {
	tests := []struct {
		name   string
		record []string
		want   string
	}{
		{
			name:   "direction denominator",
			record: calibrationRecord("100", "0", "10", "10", "10", "10", "10", "10", "10"),
			want:   "p7-pAverage",
		},
		{
			name:   "total-static denominator",
			record: calibrationRecord("100", "100", "0", "0", "0", "20", "10", "0", "100"),
			want:   "pt-ps",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, _, _, err := recomputeSevenHoleInnerCoeffs(tc.record); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want error containing %q", err, tc.want)
			}
		})
	}
}

func TestRecomputeSevenHoleOuterCoeffsRejectsDegeneratePressureRows(t *testing.T) {
	tests := []struct {
		name   string
		record []string
		want   string
	}{
		{
			name:   "direction denominator",
			record: calibrationRecord("100", "0", "10", "10", "0", "0", "0", "10", "0"),
			want:   "pcenter-(pleft+pright)/2",
		},
		{
			name:   "total-static denominator",
			record: calibrationRecord("100", "100", "20", "10", "0", "0", "0", "0", "0"),
			want:   "pt-ps",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, _, _, err := recomputeSevenHoleOuterCoeffs(tc.record, 1); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want error containing %q", err, tc.want)
			}
		})
	}
}

func TestResolveSevenHoleCSVColumnsAcceptsPressureOnlySchema(t *testing.T) {
	header := []string{"angle1", "angle2", "ma", "pt", "ps", "p1", "p2", "p3", "p4", "p5", "p6", "p7"}
	cols, _, err := resolveSevenHoleCSVColumns("pressure-only.csv", header, true)
	if err != nil {
		t.Fatalf("12-column pressure schema must be accepted: %v", err)
	}
	if cols.a != sevenHoleCSVAngle1Column || cols.b != sevenHoleCSVAngle2Column {
		t.Fatalf("angle columns = (%d,%d), want (%d,%d)", cols.a, cols.b, sevenHoleCSVAngle1Column, sevenHoleCSVAngle2Column)
	}
}

func calibrationRecord(pt, ps, p1, p2, p3, p4, p5, p6, p7 string) []string {
	return []string{"0", "0", "0", pt, ps, p1, p2, p3, p4, p5, p6, p7}
}
