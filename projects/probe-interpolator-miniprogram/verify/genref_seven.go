package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	seven "ai-workspace/shared/algorithms/go/sevenhole/interpolation"
)

type gInput struct {
	P1 float64 `json:"p1"`
	P2 float64 `json:"p2"`
	P3 float64 `json:"p3"`
	P4 float64 `json:"p4"`
	P5 float64 `json:"p5"`
	P6 float64 `json:"p6"`
	P7 float64 `json:"p7"`
	Pa float64 `json:"pa"`
	T  float64 `json:"t"`
}

type gEntry struct {
	Index    int    `json:"index"`
	Mode     string `json:"mode"`
	Sector   int    `json:"sector"`
	Fallback bool   `json:"fallback"`
	Input    gInput `json:"input"`
}

type jsInput struct {
	P1   float64 `json:"P1"`
	P2   float64 `json:"P2"`
	P3   float64 `json:"P3"`
	P4   float64 `json:"P4"`
	P5   float64 `json:"P5"`
	P6   float64 `json:"P6"`
	P7   float64 `json:"P7"`
	PAtm float64 `json:"PAtm"`
	TAtm float64 `json:"TAtm"`
}

type goResult struct {
	Alpha           float64 `json:"alpha"`
	Beta            float64 `json:"beta"`
	Theta           float64 `json:"theta"`
	Phi             float64 `json:"phi"`
	MachNumber      float64 `json:"machNumber"`
	Velocity        float64 `json:"velocity"`
	TotalPressure   float64 `json:"totalPressure"`
	StaticPressure  float64 `json:"staticPressure"`
	DynamicPressure float64 `json:"dynamicPressure"`
	IsValid         bool    `json:"isValid"`
	Warning         string  `json:"warning"`
}

type caseOut struct {
	Index    int      `json:"index"`
	Mode     string   `json:"mode"`
	Sector   int      `json:"sector"`
	Fallback bool     `json:"fallback"`
	Input    jsInput  `json:"input"`
	Go       goResult `json:"go"`
}

type outerStore struct {
	Sector int      `json:"sector"`
	Name   string   `json:"name"`
	Lines  []string `json:"lines"`
}

type prbStore struct {
	Inner     string       `json:"inner"`
	InnerLines []string    `json:"innerLines"`
	Outer     []outerStore `json:"outer"`
}

type refOut struct {
	Prb   prbStore `json:"prb"`
	Cases []caseOut `json:"cases"`
}

func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

func main() {
	if len(os.Args) < 5 {
		fmt.Fprintln(os.Stderr, "usage: genref_seven <prbDir> <goldenFile> <boundaryFile> <outFile>")
		os.Exit(2)
	}
	prbDir := os.Args[1]
	goldenFile := os.Args[2]
	boundaryFile := os.Args[3]
	outFile := os.Args[4]

	p := seven.NewSevenHolePrbInterpolator()
	innerLines, err := readLines(filepath.Join(prbDir, "7.prb"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "read 7.prb:", err)
		os.Exit(1)
	}
	if err := p.LoadInnerPrbLines(innerLines, "7.prb"); err != nil {
		fmt.Fprintln(os.Stderr, "load 7.prb:", err)
		os.Exit(1)
	}
	var outer []outerStore
	for s := 1; s <= 6; s++ {
		name := fmt.Sprintf("%d.prb", s)
		lines, err := readLines(filepath.Join(prbDir, name))
		if err != nil {
			fmt.Fprintln(os.Stderr, "read", name, err)
			os.Exit(1)
		}
		if err := p.LoadOuterPrbLines(s, lines, name); err != nil {
			fmt.Fprintln(os.Stderr, "load", name, err)
			os.Exit(1)
		}
		outer = append(outer, outerStore{Sector: s, Name: name, Lines: lines})
	}

	entries := []gEntry{}
	for _, f := range []string{goldenFile, boundaryFile} {
		data, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, "read", f, err)
			os.Exit(1)
		}
		var es []gEntry
		if err := json.Unmarshal(data, &es); err != nil {
			fmt.Fprintln(os.Stderr, "parse", f, err)
			os.Exit(1)
		}
		entries = append(entries, es...)
	}

	cases := make([]caseOut, 0, len(entries))
	for _, e := range entries {
		in := seven.InterpolationInput{
			P1: e.Input.P1, P2: e.Input.P2, P3: e.Input.P3, P4: e.Input.P4,
			P5: e.Input.P5, P6: e.Input.P6, P7: e.Input.P7,
			PAtm: e.Input.Pa, TAtm: e.Input.T,
		}
		res, calcErr := p.Calculate(in)
		gr := goResult{}
		if calcErr != nil {
			gr.Warning = calcErr.Error()
			gr.IsValid = false
		} else {
			gr = goResult{
				Alpha: res.Alpha, Beta: res.Beta, Theta: res.Theta, Phi: res.Phi,
				MachNumber: res.MachNumber, Velocity: res.Velocity,
				TotalPressure: res.TotalPressure, StaticPressure: res.StaticPressure,
				DynamicPressure: res.DynamicPressure, IsValid: res.IsValid, Warning: res.Warning,
			}
		}
		cases = append(cases, caseOut{
			Index: e.Index, Mode: e.Mode, Sector: e.Sector, Fallback: e.Fallback,
			Input: jsInput{
				P1: e.Input.P1, P2: e.Input.P2, P3: e.Input.P3, P4: e.Input.P4,
				P5: e.Input.P5, P6: e.Input.P6, P7: e.Input.P7, PAtm: e.Input.Pa, TAtm: e.Input.T,
			},
			Go: gr,
		})
	}

	out := refOut{
		Prb: prbStore{
			Inner:     "7.prb",
			InnerLines: innerLines,
			Outer:     outer,
		},
		Cases: cases,
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "marshal:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(outFile, b, 0644); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s, cases=%d\n", outFile, len(cases))
}
