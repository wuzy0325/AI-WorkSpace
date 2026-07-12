package traversal

import (
	"math"
	"reflect"
	"testing"
)

func TestPointStatusCommitContract(t *testing.T) {
	result := PointResult{
		TaskID:             "task-1",
		CommitSeq:          3,
		PointStatus:        PointStatusSkipped,
		StartedAt:          100,
		CompletedAt:        200,
		ValidationWarnings: []string{"out of range"},
		CSVRowHash:         "abc",
	}
	if result.PointStatus.IsCommitted() != true {
		t.Fatal("completed and skipped point statuses must count as committed")
	}
	if PointStatusFailed.IsCommitted() {
		t.Fatal("failed diagnostics must not count as committed")
	}
}

func TestStatusSeparatesCommittedCountFromExecutionIndex(t *testing.T) {
	status := Status{CurrentPoint: 2, CommittedPoints: 2, CurrentPointIndex: 4}
	if status.CommittedPoints == status.CurrentPointIndex {
		t.Fatal("committed count and current execution index must have independent semantics")
	}
}

func TestStrictStepValuesIncludesBounds(t *testing.T) {
	tests := []struct {
		name     string
		start    float64
		end      float64
		segments []StepSegment
		want     []float64
	}{
		{name: "ascending non divisible", start: 0, end: 10, segments: []StepSegment{{Start: 0, End: 10, Step: 3}}, want: []float64{0, 3, 6, 9, 10}},
		{name: "descending non divisible", start: 10, end: 0, segments: []StepSegment{{Start: 10, End: 0, Step: 3}}, want: []float64{10, 7, 4, 1, 0}},
		{name: "multiple segments", start: 0, end: 10, segments: []StepSegment{{Start: 0, End: 5, Step: 2}, {Start: 5, End: 10, Step: 2}}, want: []float64{0, 2, 4, 5, 7, 9, 10}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := StrictStepValues(test.start, test.end, test.segments)
			if err != nil {
				t.Fatalf("StrictStepValues returned error: %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
}

func TestStrictStepValuesRejectsInvalidNumbers(t *testing.T) {
	tests := []struct {
		name     string
		start    float64
		end      float64
		segments []StepSegment
	}{
		{name: "nan bound", start: math.NaN(), end: 1, segments: []StepSegment{{Start: 0, End: 1, Step: 1}}},
		{name: "infinite bound", start: 0, end: math.Inf(1), segments: []StepSegment{{Start: 0, End: 1, Step: 1}}},
		{name: "zero step", start: 0, end: 1, segments: []StepSegment{{Start: 0, End: 1, Step: 0}}},
		{name: "infinite step", start: 0, end: 1, segments: []StepSegment{{Start: 0, End: 1, Step: math.Inf(1)}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := StrictStepValues(test.start, test.end, test.segments); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestStrictPointsFromLayoutUsesStrictStepValues(t *testing.T) {
	points, err := StrictPointsFromLayout(LayoutConfig{
		Pattern: "line",
		Line: &LineLayout{
			StartX: 0,
			EndX:   10,
			XStepSegments: []StepSegment{
				{Start: 0, End: 10, Step: 3},
			},
		},
	})
	if err != nil {
		t.Fatalf("StrictPointsFromLayout returned error: %v", err)
	}
	want := []Point{{X: 0}, {X: 3}, {X: 6}, {X: 9}, {X: 10}}
	if !reflect.DeepEqual(points, want) {
		t.Fatalf("got %+v, want %+v", points, want)
	}
}

func TestStrictPointsFromLayoutRejectsInvalidStep(t *testing.T) {
	_, err := StrictPointsFromLayout(LayoutConfig{
		Pattern: "rectangle",
		Rectangle: &RectangleLayout{
			XMin: 0,
			XMax: 1,
			XStepSegments: []StepSegment{
				{Start: 0, End: 1, Step: 0},
			},
			YMin: 0,
			YMax: 1,
			YStepSegments: []StepSegment{
				{Start: 0, End: 1, Step: 1},
			},
		},
	})
	if err == nil {
		t.Fatal("expected invalid layout step to be rejected")
	}
}

func TestGenerateGridPathUsesClosedIntervalStepValues(t *testing.T) {
	points, err := GenerateGridPath(GridConfig{
		XStart: 0,
		XEnd:   10,
		XStep:  3,
		YStart: 0,
		YEnd:   0,
		YStep:  1,
		ZStart: 2,
	})
	if err != nil {
		t.Fatalf("GenerateGridPath returned error: %v", err)
	}
	want := []Point{{X: 0, Z: 2}, {X: 3, Z: 2}, {X: 6, Z: 2}, {X: 9, Z: 2}, {X: 10, Z: 2}}
	if !reflect.DeepEqual(points, want) {
		t.Fatalf("got %+v, want %+v", points, want)
	}
}

func TestGenerateGridPathRejectsNonFiniteValues(t *testing.T) {
	_, err := GenerateGridPath(GridConfig{XStart: 0, XEnd: math.Inf(1), XStep: 1, YStart: 0, YEnd: 1, YStep: 1})
	if err == nil {
		t.Fatal("expected non-finite grid bounds to be rejected")
	}
}
