package calibration

import (
	"strings"
	"testing"
)

func TestParseFiveHolePointFilePreservesOrderAndDuplicates(t *testing.T) {
	content := "\ufeffbeta,alpha\r\n-10,5\r\n\r\n0 0\r\n-10,5\r\n"

	points, err := ParseFiveHolePointFile(content)
	if err != nil {
		t.Fatalf("ParseFiveHolePointFile: %v", err)
	}

	want := [][2]float64{{5, -10}, {0, 0}, {5, -10}}
	if len(points) != len(want) {
		t.Fatalf("expected %d points, got %d", len(want), len(points))
	}
	for index, expected := range want {
		if points[index].ID != index+1 {
			t.Errorf("point %d: expected id %d, got %d", index, index+1, points[index].ID)
		}
		if points[index].Coordinates["α"] != expected[0] || points[index].Coordinates["β"] != expected[1] {
			t.Errorf("point %d: expected α=%v β=%v, got %#v", index, expected[0], expected[1], points[index].Coordinates)
		}
	}
}

func TestParseFiveHolePointFileAcceptsHeaderCaseAndGreekLetters(t *testing.T) {
	for _, content := range []string{
		"Beta,Alpha\n-2.5,3.5\n",
		"β α\n-2.5 3.5\n",
	} {
		points, err := ParseFiveHolePointFile(content)
		if err != nil {
			t.Fatalf("ParseFiveHolePointFile(%q): %v", content, err)
		}
		if len(points) != 1 || points[0].Coordinates["α"] != 3.5 || points[0].Coordinates["β"] != -2.5 {
			t.Fatalf("unexpected points for %q: %#v", content, points)
		}
	}
}

func TestParseFiveHolePointFileAcceptsUpTo60Degrees(t *testing.T) {
	content := "0,34.921\n-60,60\n"
	points, err := ParseFiveHolePointFile(content)
	if err != nil {
		t.Fatalf("ParseFiveHolePointFile: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(points))
	}
}

func TestParseFiveHolePointFileRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "empty", content: "\n\r\n", want: "至少包含一个点"},
		{name: "wrong columns", content: "0,1,2\n", want: "第 1 行"},
		{name: "empty csv column", content: "0,,1\n", want: "第 1 行"},
		{name: "non numeric", content: "beta,alpha\n0,nope\n", want: "第 2 行"},
		{name: "alpha out of range", content: "0,60.1\n", want: "第 1 行"},
		{name: "beta out of range", content: "-60.1,0\n", want: "第 1 行"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseFiveHolePointFile(test.content)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}
