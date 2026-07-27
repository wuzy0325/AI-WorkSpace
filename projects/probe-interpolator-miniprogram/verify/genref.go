// genref.go —— 用 Go 原版插值器生成数值参考（reference.json）
// 运行：GOWORK=off go run genref.go <golden_prb_dir> <reference.json>
//
// 1) 加载 testdata/golden/prb/*.json 的 4 个黄金用例输入
// 2) 额外对中心区网格 (-25..25 步长5) 做 121 点扫描
// 均使用 syntheticPrbLines(0.05, 0.01) 合成 PRB 校准数据，与小程序 TS 端口完全一致。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	interp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
)

// 与 Go 测试一致的合成 PRB 生成
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

type goldenInput struct {
	P1, P2, P3, P4, P5, PAtm, TAtm float64
}

type goldenCase struct {
	Name  string      `json:"name"`
	Input goldenInput `json:"input"`
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: genref <golden_prb_dir> <reference.json>")
		os.Exit(2)
	}
	goldenDir := os.Args[1]
	outPath := os.Args[2]

	// 构造合成 PRB 并加载
	ip := interp.NewPrbInterpolator()
	if err := ip.LoadPrbLines(syntheticPrbLines(0.05, 0.01), "0.5Ma.prb"); err != nil {
		panic(err)
	}

	var refs []map[string]interface{}

	// 1) 黄金用例
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
		refs = append(refs, runCase(ip, gc.Name, gc.Input))
	}

	// 2) 中心区网格扫描 (-25..25 步长5)
	for a := -25.0; a <= 25; a += 5 {
		for b := -25.0; b <= 25; b += 5 {
			in := goldenInput{
				P1: 100 - b/2, P2: 200, P3: 100 + b/2, P4: 100 + a/2, P5: 100 - a/2,
				PAtm: 101325, TAtm: 20,
			}
			refs = append(refs, runCase(ip, fmt.Sprintf("sweep_a%.0f_b%.0f", a, b), in))
		}
	}

	b, _ := json.MarshalIndent(refs, "", "  ")
	if err := os.WriteFile(outPath, b, 0o644); err != nil {
		panic(err)
	}
	fmt.Printf("wrote %s, cases=%d\n", outPath, len(refs))
}

func runCase(ip *interp.PrbInterpolator, name string, in goldenInput) map[string]interface{} {
	res, err := ip.Calculate(interp.InterpolationInput{
		P1: in.P1, P2: in.P2, P3: in.P3, P4: in.P4, P5: in.P5,
		PAtm: in.PAtm, TAtm: in.TAtm,
	})
	if err != nil {
		panic(fmt.Sprintf("case %s: %v", name, err))
	}
	return map[string]interface{}{
		"name": name,
		"input": map[string]interface{}{
			"P1": in.P1, "P2": in.P2, "P3": in.P3, "P4": in.P4, "P5": in.P5,
			"PAtm": in.PAtm, "TAtm": in.TAtm,
		},
		"go": map[string]interface{}{
			"alpha":           res.Alpha,
			"beta":            res.Beta,
			"machNumber":      res.MachNumber,
			"v":               res.V,
			"vx":              res.Vx,
			"vy":              res.Vy,
			"vz":              res.Vz,
			"cas":             res.CAS,
			"sat":             res.SAT,
			"dynamicPressure": res.DynamicPressure,
			"density":         res.Density,
			"P0":              res.TotalPressure,
			"Ps":              res.StaticPressure,
			"isValid":         res.IsValid,
			"warning":         res.Warning,
		},
	}
}
