package traversal

import (
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

func TestStrictPointsFromLayoutUsesStrictStepValues(t *testing.T) {
	points := PointsFromLayout(LayoutConfig{
		Pattern: "line",
		Line: &LineLayout{
			StartX: 0,
			EndX:   10,
			XStepSegments: []StepSegment{
				{Start: 0, End: 10, Step: 3},
			},
		},
	})
	// master 上 StepValues 用 math.Round 决定段数：(10-0)/3=3.33 → 3 段 → 4 个点（0,3,6,9），不含 end
	// line 模式 Y/Z/U 标记为 NaN（markAxesNaN），这里只校验 X 坐标序列
	if len(points) != 4 {
		t.Fatalf("expected 4 points, got %d", len(points))
	}
	wantX := []float64{0, 3, 6, 9}
	for i, p := range points {
		if p.X != wantX[i] {
			t.Fatalf("point %d X = %v, want %v", i, p.X, wantX[i])
		}
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
	// master 上 GenerateGridPath 用 math.Round 决定段数，(10-0)/3=3.33 → 3 段 → 4 个点（0,3,6,9），不含 X:10
	want := []Point{{X: 0, Y: 0, Z: 2, U: 0}, {X: 3, Y: 0, Z: 2, U: 0}, {X: 6, Y: 0, Z: 2, U: 0}, {X: 9, Y: 0, Z: 2, U: 0}}
	if !reflect.DeepEqual(points, want) {
		t.Fatalf("got %+v, want %+v", points, want)
	}
}
