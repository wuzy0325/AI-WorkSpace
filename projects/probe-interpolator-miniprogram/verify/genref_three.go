// genref_three.go —— 用 Go 原版三孔插值器生成数值参考（reference_three.json）
// 运行：GOWORK=off go run genref_three.go <golden_threehole_dir> <reference_three.json> <0.8.prb>
//
// 用例来源：
//  1) testdata/golden/threehole/*.json 的真实校准黄金用例（输入来自校准表可逆性）
//  2) 对 Kb 做扫描（插值 + 外推），覆盖三孔 Kb→Alpha 全区间
//  3) 多马赫合成用例：两张合成 .prb（CMa=0.3 / 0.6）测试 ThreeHoleInterpolator 内部的
//     双最近马赫表线性插值
//
// 合成用例与 verify_three.js 中的合成生成器（threeSynthLines）逐字节一致。
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	interp "ai-workspace/shared/algorithms/go/threehole/interpolation"
)

// threeSynthLines 合成三孔 .prb 文本行（与 verify_three.js 的 threeSynthLines 一致）
func threeSynthLines(cma float64) []string {
	lines := []string{fmt.Sprintf("%.6f", cma), "13"}
	for alpha := -30.0; alpha <= 30; alpha += 5 {
		kb := (4 + 2*cma) * math.Sin(alpha*math.Pi/180)
		k0 := 0.5 + 0.01*alpha + 0.1*cma
		kv := 2.0 + 0.02*alpha + 0.05*cma
		lines = append(lines, fmt.Sprintf("%.10f %.10f %.10f %.0f", kb, k0, kv, alpha))
	}
	return lines
}

// 由目标 Kb 反推孔压（构造可解的输入）
func inputFromKb(kb float64) interp.InterpolationInput {
	p2 := 200.0
	deltaP := 100.0
	p1 := (300.0 - kb*deltaP) / 2.0
	p3 := (300.0 + kb*deltaP) / 2.0
	return interp.InterpolationInput{P1: p1, P2: p2, P3: p3, PAtm: 101325, TAtm: 20}
}

type goldenInput struct {
	P1, P2, P3, PAtm, TAtm float64
}

type goldenCase struct {
	Name  string      `json:"name"`
	Input goldenInput `json:"input"`
}

func main() {
	if len(os.Args) < 4 {
		fmt.Println("usage: genref_three <golden_threehole_dir> <reference_three.json> <0.8.prb>")
		os.Exit(2)
	}
	goldenDir := os.Args[1]
	outPath := os.Args[2]
	prbPath := os.Args[3]

	// 1) 真实 0.8.prb 校准插值器
	prbData, err := os.ReadFile(prbPath)
	if err != nil {
		panic(err)
	}
	prbLines := strings.Split(string(prbData), "\n")
	realInterp := interp.NewThreeHoleInterpolator()
	if _, err := realInterp.LoadPrbData([]interp.PrbFileData{
		{FilePath: "0.8.prb", Lines: prbLines},
	}); err != nil {
		panic(err)
	}

	var refs []map[string]interface{}

	// 2) 真实校准黄金用例
	entries, err := os.ReadDir(goldenDir)
	if err != nil {
		panic(err)
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(goldenDir, e.Name()))
		var gc goldenCase
		if json.Unmarshal(data, &gc) != nil {
			continue
		}
		refs = append(refs, runReal(realInterp, "golden:"+gc.Name, gc.Input))
	}

	// 3) Kb 扫描（插值 + 外推）
	for i := 0; i <= 32; i++ {
		kb := -4.0 + 8.0*float64(i)/32.0
		in := inputFromKb(kb)
		refs = append(refs, runReal(realInterp, fmt.Sprintf("sweep_kb_%.3f", kb), goldenInput{
			P1: in.P1, P2: in.P2, P3: in.P3, PAtm: in.PAtm, TAtm: in.TAtm,
		}))
	}

	// 4) 多马赫合成用例（CMa=0.3 / 0.6）
	cmaList := []float64{0.3, 0.6}
	multiInterp := interp.NewThreeHoleInterpolator()
	var multiFileData []interp.PrbFileData
	for _, cma := range cmaList {
		multiFileData = append(multiFileData, interp.PrbFileData{
			FilePath: fmt.Sprintf("%.1fMa.prb", cma),
			Lines:    threeSynthLines(cma),
		})
	}
	if _, err := multiInterp.LoadPrbData(multiFileData); err != nil {
		panic(err)
	}
	for i := 0; i <= 8; i++ {
		kb := -4.0 + 8.0*float64(i)/8.0
		in := inputFromKb(kb)
		refs = append(refs, runMulti(multiInterp, cmaList, fmt.Sprintf("multima_kb_%.3f", kb), goldenInput{
			P1: in.P1, P2: in.P2, P3: in.P3, PAtm: in.PAtm, TAtm: in.TAtm,
		}))
	}

	b, _ := json.MarshalIndent(refs, "", "  ")
	if err := os.WriteFile(outPath, b, 0o644); err != nil {
		panic(err)
	}
	fmt.Printf("wrote %s, cases=%d\n", outPath, len(refs))
}

func runReal(ip *interp.ThreeHoleInterpolator, name string, in goldenInput) map[string]interface{} {
	res, err := ip.Calculate(interp.InterpolationInput{
		P1: in.P1, P2: in.P2, P3: in.P3, PAtm: in.PAtm, TAtm: in.TAtm,
	})
	if err != nil {
		panic(fmt.Sprintf("case %s: %v", name, err))
	}
	return baseRef(name, nil, in, res)
}

func runMulti(ip *interp.ThreeHoleInterpolator, cmaList []float64, name string, in goldenInput) map[string]interface{} {
	res, err := ip.Calculate(interp.InterpolationInput{
		P1: in.P1, P2: in.P2, P3: in.P3, PAtm: in.PAtm, TAtm: in.TAtm,
	})
	if err != nil {
		panic(fmt.Sprintf("case %s: %v", name, err))
	}
	return baseRef(name, cmaList, in, res)
}

func baseRef(name string, cmaList []float64, in goldenInput, res interp.InterpolationResult) map[string]interface{} {
	ref := map[string]interface{}{
		"name": name,
		"input": map[string]interface{}{
			"P1": in.P1, "P2": in.P2, "P3": in.P3,
			"PAtm": in.PAtm, "TAtm": in.TAtm,
		},
		"go": map[string]interface{}{
			"alpha":      res.Alpha,
			"machNumber": res.MachNumber,
			"velocity":   res.Velocity,
			"P0":         res.TotalPressure,
			"Ps":         res.StaticPressure,
			"calculated": res.Calculated,
			"isValid":    res.IsValid,
			"warning":    res.Warning,
		},
	}
	if cmaList != nil {
		ref["cmaList"] = cmaList
	}
	return ref
}

// 防止未使用导入告警
var _ = strconv.ParseFloat
