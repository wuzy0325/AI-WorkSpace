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

	// 5) 二分边界增强向量（改善项 E §6.2 / review #4）
	// 用真实 0.2/0.4Ma PRB 构造多档插值器，对 Kb 输入覆盖：
	//   - 精确等于混合后内部节点、节点 ±1 ULP
	//   - 区间内部、边界外
	// 对 Ma 覆盖：ratio=0/1（精确落在某档）、两档中点（ratio=0.5）、超范围钳位。
	// 三档场景覆盖 0.2/0.4/0.6 组合与不同加载顺序。
	{
		// 合成三档，验证多档 + 加载顺序
		triSpec := []float64{0.2, 0.4, 0.6}
		triInterp := interp.NewThreeHoleInterpolator()
		var triFileData []interp.PrbFileData
		for _, cma := range triSpec {
			triFileData = append(triFileData, interp.PrbFileData{
				FilePath: fmt.Sprintf("%.1fMa.prb", cma),
				Lines:    threeSynthLines(cma),
			})
		}
		if _, err := triInterp.LoadPrbData(triFileData); err != nil {
			panic(err)
		}
		// 加载顺序反转（等价性校验：不同顺序应得相同结果）
		revInterp := interp.NewThreeHoleInterpolator()
		var revFileData []interp.PrbFileData
		for j := len(triFileData) - 1; j >= 0; j-- {
			revFileData = append(revFileData, triFileData[j])
		}
		if _, err := revInterp.LoadPrbData(revFileData); err != nil {
			panic(err)
		}

		// 构造各档混合后 Kb 的精确节点值。对合成 PRB，alpha 网格 -30..30，
		// Kb(alpha)=sin 曲线；在 Ma=0.3（ratio=0.5 于 0.2/0.4 之间）下混合。
		// 从 0.2/0.4 档取内部节点 Kb 做精确输入。
		kbNodes := []float64{}
		for alpha := -25.0; alpha <= 25; alpha += 5 {
			kbA := (4 + 2*0.2) * math.Sin(alpha*math.Pi/180)
			kbB := (4 + 2*0.4) * math.Sin(alpha*math.Pi/180)
			kbNodes = append(kbNodes, kbA+0.5*(kbB-kbA))
		}

		// 每档加载顺序各跑一组：triInterp（正序）与 revInterp（逆序）
		interpPairs := []struct {
			name      string
			ip        *interp.ThreeHoleInterpolator
			loadOrder []float64
		}{
			{"tri_order_abc", triInterp, []float64{0.2, 0.4, 0.6}},
			{"tri_order_cba", revInterp, []float64{0.6, 0.4, 0.2}},
		}
		for _, pair := range interpPairs {
			for _, kb := range kbNodes {
				// 精确节点
				in := inputFromKb(kb)
				refs = append(refs, runMultiOrdered(pair.ip, triSpec, pair.loadOrder, fmt.Sprintf("%s_node_kb_%.6f", pair.name, kb), goldenInput{
					P1: in.P1, P2: in.P2, P3: in.P3, PAtm: in.PAtm, TAtm: in.TAtm,
				}))
				// ±1 ULP
				ul := math.Nextafter(kb, math.Inf(1))
				dl := math.Nextafter(kb, math.Inf(-1))
				inU := inputFromKb(ul)
				inD := inputFromKb(dl)
				refs = append(refs, runMultiOrdered(pair.ip, triSpec, pair.loadOrder, fmt.Sprintf("%s_node_kb_ulp1_%.6f", pair.name, ul), goldenInput{
					P1: inU.P1, P2: inU.P2, P3: inU.P3, PAtm: inU.PAtm, TAtm: inU.TAtm,
				}))
				refs = append(refs, runMultiOrdered(pair.ip, triSpec, pair.loadOrder, fmt.Sprintf("%s_node_kb_ulpm1_%.6f", pair.name, dl), goldenInput{
					P1: inD.P1, P2: inD.P2, P3: inD.P3, PAtm: inD.PAtm, TAtm: inD.TAtm,
				}))
			}
		}

		// 两档 + ratio 边界：0.2/0.4 档，ma 精确 0.2 / 0.4 / 0.3 / 0.0 / 0.5
		twoSpec := []float64{0.2, 0.4}
		twoInterp := interp.NewThreeHoleInterpolator()
		var twoFileData []interp.PrbFileData
		for _, cma := range twoSpec {
			twoFileData = append(twoFileData, interp.PrbFileData{
				FilePath: fmt.Sprintf("%.1fMa.prb", cma),
				Lines:    threeSynthLines(cma),
			})
		}
		if _, err := twoInterp.LoadPrbData(twoFileData); err != nil {
			panic(err)
		}
		for _, kb := range []float64{-3.0, -1.5, 0.0, 1.5, 3.0} {
			// 两档 0.2/0.4 下，不同 Kb 输入触发不同插值区间（含边界外）。
			in := inputFromKb(kb)
			refs = append(refs, runMulti(twoInterp, twoSpec, fmt.Sprintf("two_ratio_kb_%.3f", kb), goldenInput{
				P1: in.P1, P2: in.P2, P3: in.P3, PAtm: in.PAtm, TAtm: in.TAtm,
			}))
		}
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
	return runMultiOrdered(ip, cmaList, nil, name, in)
}

// runMultiOrdered 与 runMulti 相同，但额外携带 loadOrder（cmaList 的实际加载顺序）。
// loadOrder 用于让 JS 端按相同顺序构造插值器，验证加载顺序不影响数值结果。
func runMultiOrdered(ip *interp.ThreeHoleInterpolator, cmaList []float64, loadOrder []float64, name string, in goldenInput) map[string]interface{} {
	res, err := ip.Calculate(interp.InterpolationInput{
		P1: in.P1, P2: in.P2, P3: in.P3, PAtm: in.PAtm, TAtm: in.TAtm,
	})
	if err != nil {
		panic(fmt.Sprintf("case %s: %v", name, err))
	}
	return baseRefOrdered(name, cmaList, loadOrder, in, res)
}

func baseRef(name string, cmaList []float64, in goldenInput, res interp.InterpolationResult) map[string]interface{} {
	return baseRefOrdered(name, cmaList, nil, in, res)
}

func baseRefOrdered(name string, cmaList, loadOrder []float64, in goldenInput, res interp.InterpolationResult) map[string]interface{} {
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
	if loadOrder != nil {
		ref["loadOrder"] = loadOrder
	}
	return ref
}

// 防止未使用导入告警
var _ = strconv.ParseFloat
