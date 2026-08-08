package interpolation

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"testing"
)

// =====================================================================
// 改善项 E 回归测试（docs/three-hole-algorithm-improvements.md §6）
//
// E 移除 interpolateWithWarning 中的运行时 sort.Slice，改为二分查找。
// 依据：两档严格递增 Kb 序列的 [0,1] 凸组合仍严格递增，entries 在
// 构建后已按 Kb 有序，无需再排序。
//
// 测试覆盖：
//   - 凸组合保持严格递增（属性测试，含浮点边界）
//   - 生产 interpolateWithWarning 与冻结的旧排序+线性扫描 oracle 逐位等价
//   - 多档、不同加载顺序、ratio=0/1、ratio 被钳位、±1 ULP 边界
//   - 多文件 Calculate 性能基准
// =====================================================================

// ==================== 冻结的旧实现 oracle ====================

// interpolateWithWarningOld 是改善项 E 落地前的旧实现（git HEAD 版本）：
// 混合 entries 后执行 sort.Slice，再线性扫描。冻结为测试 oracle，
// 用于验证新二分实现与其逐位一致。禁止修改（任何修改都需重新 review）。
func (t *ThreeHoleInterpolator) interpolateWithWarningOld(kbMeasured, ma float64) (*calibrationItem, bool) {
	kbExtrapolated := false

	if len(t.calib) == 0 {
		return nil, false
	}

	var calib1, calib2 *calibrationData
	d1, d2 := math.MaxFloat64, math.MaxFloat64
	for i := range t.calib {
		c := &t.calib[i]
		d := math.Abs(c.CMa - ma)
		if d < d1 {
			calib2, d2 = calib1, d1
			calib1, d1 = c, d
		} else if d < d2 {
			calib2, d2 = c, d
		}
	}
	if calib2 == nil {
		calib2 = calib1
	}

	var entries []kbAlphaEntry
	ratio := 0.0
	if math.Abs(calib2.CMa-calib1.CMa) > 1e-6 {
		ratio = (ma - calib1.CMa) / (calib2.CMa - calib1.CMa)
		if ratio < 0 || ratio > 1 {
			kbExtrapolated = true
		}
		ratio = math.Max(0, math.Min(1, ratio))
	}

	for i := 0; i < len(t.alphaSeq); i++ {
		kb := calib1.Items[i].Kb + ratio*(calib2.Items[i].Kb-calib1.Items[i].Kb)
		k0 := calib1.Items[i].K0 + ratio*(calib2.Items[i].K0-calib1.Items[i].K0)
		kv := calib1.Items[i].Kv + ratio*(calib2.Items[i].Kv-calib1.Items[i].Kv)
		entries = append(entries, kbAlphaEntry{
			Kb:    kb,
			Alpha: t.alphaSeq[i],
			K0:    k0,
			Kv:    kv,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Kb < entries[j].Kb
	})

	if kbMeasured <= entries[0].Kb {
		if kbMeasured < entries[0].Kb {
			kbExtrapolated = true
		}
		return &calibrationItem{
			Kb: entries[0].Kb, K0: entries[0].K0,
			Kv: entries[0].Kv, Alpha: entries[0].Alpha,
		}, kbExtrapolated
	}
	if kbMeasured >= entries[len(entries)-1].Kb {
		if kbMeasured > entries[len(entries)-1].Kb {
			kbExtrapolated = true
		}
		last := entries[len(entries)-1]
		return &calibrationItem{
			Kb: last.Kb, K0: last.K0,
			Kv: last.Kv, Alpha: last.Alpha,
		}, kbExtrapolated
	}

	for j := 0; j < len(entries)-1; j++ {
		if kbMeasured >= entries[j].Kb && kbMeasured <= entries[j+1].Kb {
			r := (kbMeasured - entries[j].Kb) / (entries[j+1].Kb - entries[j].Kb)
			return &calibrationItem{
				Kb:    kbMeasured,
				K0:    entries[j].K0 + r*(entries[j+1].K0-entries[j].K0),
				Kv:    entries[j].Kv + r*(entries[j+1].Kv-entries[j].Kv),
				Alpha: entries[j].Alpha + r*(entries[j+1].Alpha-entries[j].Alpha),
			}, kbExtrapolated
		}
	}

	return nil, false
}

// ==================== 测试数据构造 ====================

// synthPrbLines 生成合成三孔 PRB 行（alpha 网格 -30~30 步长 5，13 点）。
// kb 基准量级与单调性可由调用方指定，用于构造不同浮点边界场景。
type synthSpec struct {
	cma      float64
	kbBase   func(alpha float64) float64 // Kb(alpha) 生成器
	k0Base   func(alpha float64) float64
	kvBase   func(alpha float64) float64
	alphaSeq []float64
}

func defaultAlphaSeq() []float64 {
	var a []float64
	for v := -30.0; v <= 30.0; v += 5 {
		a = append(a, v)
	}
	return a
}

func defaultSynth(cma float64) synthSpec {
	return synthSpec{
		cma:      cma,
		kbBase:   func(a float64) float64 { return (4 + 2*cma) * math.Sin(a*math.Pi/180) },
		k0Base:   func(a float64) float64 { return 0.5 + 0.01*a + 0.1*cma },
		kvBase:   func(a float64) float64 { return 2.0 + 0.02*a + 0.05*cma },
		alphaSeq: defaultAlphaSeq(),
	}
}

func (s synthSpec) lines() []string {
	lines := []string{fmt.Sprintf("%.6f", s.cma), fmt.Sprintf("%d", len(s.alphaSeq))}
	for _, a := range s.alphaSeq {
		lines = append(lines, fmt.Sprintf("%.10f %.10f %.10f %.0f",
			s.kbBase(a), s.k0Base(a), s.kvBase(a), a))
	}
	return lines
}

// buildInterp 从 specs 构造已加载插值器。
func buildInterp(specs ...synthSpec) *ThreeHoleInterpolator {
	t := NewThreeHoleInterpolator()
	var fd []PrbFileData
	for i, s := range specs {
		fd = append(fd, PrbFileData{FilePath: fmt.Sprintf("synth%d.prb", i), Lines: s.lines()})
	}
	if _, err := t.LoadPrbData(fd); err != nil {
		panic(fmt.Sprintf("LoadPrbData: %v", err))
	}
	return t
}

// buildInterpRaw 直接构造 calibrationData 而不经过文本 lines()/LoadPrbData 往返。
// 用于需要保留 1-ULP 级 Kb 间距的浮点边界测试（text 序列化 %.10f 会丢失 ULP）。
func buildInterpRaw(kbSeqs ...[]float64) *ThreeHoleInterpolator {
	n := len(kbSeqs[0])
	alphaSeq := make([]float64, n)
	for i := range alphaSeq {
		alphaSeq[i] = float64(-25 + 5*i)
	}
	t := &ThreeHoleInterpolator{
		loaded:   true,
		alphaSeq: alphaSeq,
	}
	var calib []calibrationData
	var sumMa float64
	for i, kbs := range kbSeqs {
		items := make([]calibrationItem, n)
		for j := range kbs {
			items[j] = calibrationItem{Kb: kbs[j], K0: 0.5, Kv: 2.0, Alpha: alphaSeq[j]}
		}
		cd := calibrationData{CMa: 0.3 + 0.1*float64(i), Nalpha: n, Items: items}
		cd.kbSorted = append([]calibrationItem(nil), items...)
		sort.Slice(cd.kbSorted, func(a, b int) bool { return cd.kbSorted[a].Kb < cd.kbSorted[b].Kb })
		calib = append(calib, cd)
		sumMa += cd.CMa
	}
	t.calib = calib
	t.initMa = sumMa / float64(len(calib))
	t.minMa, t.maxMa = calib[0].CMa, calib[len(calib)-1].CMa
	return t
}

// ==================== 逐位比较辅助 ====================

// assertItemBits 用 math.Float64bits 逐位比较两个 calibrationItem 与 kbExtrapolated。
func assertItemBits(t *testing.T, name string, got, want *calibrationItem, gotExt, wantExt bool) {
	t.Helper()
	if got == nil || want == nil {
		if got != want {
			t.Fatalf("%s: nil 不一致 got=%v want=%v", name, got, want)
		}
		return
	}
	fields := []struct {
		fname string
		g, w  float64
	}{
		{"Kb", got.Kb, want.Kb},
		{"K0", got.K0, want.K0},
		{"Kv", got.Kv, want.Kv},
		{"Alpha", got.Alpha, want.Alpha},
	}
	for _, f := range fields {
		if math.Float64bits(f.g) != math.Float64bits(f.w) {
			t.Fatalf("%s: %s 逐位不一致 got=%v(0x%x) want=%v(0x%x)",
				name, f.fname, f.g, math.Float64bits(f.g), f.w, math.Float64bits(f.w))
		}
	}
	if gotExt != wantExt {
		t.Fatalf("%s: kbExtrapolated 不一致 got=%v want=%v", name, gotExt, wantExt)
	}
}

// ==================== 属性测试：凸组合严格递增 ====================

// genMonotonicKb 生成 N 个严格递增的 Kb（固定种子）。
func genMonotonicKb(rng *rand.Rand, n int) []float64 {
	kb := make([]float64, n)
	cur := -5.0
	for i := 0; i < n; i++ {
		kb[i] = cur
		cur += 0.05 + rng.Float64()*0.4
	}
	return kb
}

// genAdjacentKb 生成相邻值只差 1 ULP 的严格递增序列（浮点最坏场景）。
func genAdjacentKb(start float64, n int) []float64 {
	kb := make([]float64, n)
	cur := start
	for i := 0; i < n; i++ {
		kb[i] = cur
		cur = math.Nextafter(cur, math.Inf(1))
	}
	return kb
}

func assertStrictlyIncreasing(t *testing.T, name string, v []float64) {
	t.Helper()
	for i := 1; i < len(v); i++ {
		if v[i] <= v[i-1] {
			t.Fatalf("%s 在索引 %d 非严格递增: v[%d]=%.17g, v[%d]=%.17g",
				name, i, i-1, v[i-1], i, v[i])
		}
	}
}

// TestE_ConvexCombinationMonotonic 属性测试：普通量级 + 随机 ratio。
func TestE_ConvexCombinationMonotonic(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for iter := 0; iter < 50; iter++ {
		n := 5 + rng.Intn(10)
		ka := genMonotonicKb(rng, n)
		kb := genMonotonicKb(rng, n)
		ratio := rng.Float64()
		mix := make([]float64, n)
		for i := 0; i < n; i++ {
			mix[i] = ka[i] + ratio*(kb[i]-ka[i])
		}
		assertStrictlyIncreasing(t, fmt.Sprintf("iter=%d ratio=%.6f", iter, ratio), mix)
	}
}

// TestE_ConvexCombination_FloatEdgeCases 属性测试：浮点边界。
// 覆盖 math.Nextafter 相邻值、不同数量级、次正规、接近最大有限值、随机 ratio。
func TestE_ConvexCombination_FloatEdgeCases(t *testing.T) {
	rng := rand.New(rand.NewSource(11))

	// 1) 1 ULP 相邻值混合
	ka := genAdjacentKb(1.0, 16)
	kb := genAdjacentKb(1.0+math.Nextafter(0, 1), 16)
	for iter := 0; iter < 200; iter++ {
		ratio := rng.Float64()
		mix := make([]float64, len(ka))
		for i := range ka {
			mix[i] = ka[i] + ratio*(kb[i]-ka[i])
		}
		assertStrictlyIncreasing(t, fmt.Sprintf("adjacent iter=%d ratio=%.17g", iter, ratio), mix)
	}

	// 2) 不同数量级：小量级 + 大量级
	ka = genMonotonicKb(rng, 12)                    // ~ -5..0 量级
	kb = make([]float64, 12)                        // 1e6 量级
	for i := range kb {
		kb[i] = 1e6 + float64(i)
	}
	for iter := 0; iter < 100; iter++ {
		ratio := rng.Float64()
		mix := make([]float64, len(ka))
		for i := range ka {
			mix[i] = ka[i] + ratio*(kb[i]-ka[i])
		}
		assertStrictlyIncreasing(t, fmt.Sprintf("scale iter=%d ratio=%.6f", iter, ratio), mix)
	}

	// 3) 次正规数
	subNormal := []float64{math.SmallestNonzeroFloat64}
	for i := 0; i < 8; i++ {
		subNormal = append(subNormal, math.Nextafter(subNormal[i], math.Inf(1)))
	}
	kb = subNormal
	for i := range kb {
		kb[i] = math.Nextafter(kb[i], math.Inf(1))
	}
	for iter := 0; iter < 100; iter++ {
		ratio := rng.Float64()
		mix := make([]float64, len(subNormal))
		for i := range subNormal {
			mix[i] = subNormal[i] + ratio*(kb[i]-subNormal[i])
		}
		assertStrictlyIncreasing(t, fmt.Sprintf("subnormal iter=%d ratio=%.6f", iter, ratio), mix)
	}

	// 4) 接近最大有限值
	big := []float64{math.MaxFloat64 - 1e12}
	for i := 0; i < 8; i++ {
		big = append(big, math.Nextafter(big[i], math.Inf(1)))
	}
	kb = big
	for i := range kb {
		kb[i] = math.Nextafter(kb[i], math.Inf(1))
	}
	for iter := 0; iter < 50; iter++ {
		ratio := rng.Float64()
		mix := make([]float64, len(big))
		for i := range big {
			mix[i] = big[i] + ratio*(kb[i]-big[i])
		}
		assertStrictlyIncreasing(t, fmt.Sprintf("big iter=%d ratio=%.6f", iter, ratio), mix)
	}
}

// TestE_ConvexCombination_ZeroToOneBoundary 边界 ratio 0 与 1 精确还原。
func TestE_ConvexCombination_ZeroToOneBoundary(t *testing.T) {
	rng := rand.New(rand.NewSource(8))
	for iter := 0; iter < 20; iter++ {
		ka := genMonotonicKb(rng, 8)
		kb := genMonotonicKb(rng, 8)
		for i := range ka {
			mix0 := ka[i] + 0*(kb[i]-ka[i])
			mix1 := ka[i] + 1*(kb[i]-ka[i])
			if mix0 != ka[i] || mix1 != kb[i] {
				t.Fatalf("边界 ratio 未精确还原: iter=%d i=%d mix0=%.17g ka=%.17g mix1=%.17g kb=%.17g",
					iter, i, mix0, ka[i], mix1, kb[i])
			}
		}
	}
}

// ==================== 生产实现 vs 冻结 oracle 等价性 ====================

// TestE_ProductionVsOldOracle 直接调用生产 interpolateWithWarning，与冻结的
// 旧排序+线性扫描实现 interpolateWithWarningOld 逐位比较。
// 覆盖：多档、不同加载顺序、ratio=0/1、ratio 被钳位、内部节点与 ±1 ULP、越界。
func TestE_ProductionVsOldOracle(t *testing.T) {
	// 构造 3 档合成 PRB，验证多档 + 不同加载顺序
	specs := []synthSpec{defaultSynth(0.2), defaultSynth(0.4), defaultSynth(0.6)}
	interp := buildInterp(specs...)

	// 覆盖的 kbMeasured 与 ma 组合：
	//   - kb：越界两端、每个内部节点、节点 ±1 ULP、区间内部
	//   - ma：精确落在某档（ratio=0/1）、两档中点（ratio=0.5）、
	//         超出范围（ratio 被钳位）
	kbs := []float64{-2.5, -2.0, -1.0, 0.0, 0.1, 1.0, 2.0, 2.5}
	for _, kb := range kbs {
		// 每个 kb 取几个近邻的 ULP 测试
		cands := []float64{kb}
		for d := 0; d < 3; d++ {
			cands = append(cands, math.Nextafter(kb, math.Inf(1)))
			cands = append(cands, math.Nextafter(kb, math.Inf(-1)))
		}
		for _, c := range cands {
			for _, ma := range []float64{0.2, 0.3, 0.4, 0.5, 0.6, 0.0, 0.7} {
				name := fmt.Sprintf("kb=%.17g ma=%.3f", c, ma)
				got, gotExt := interp.interpolateWithWarning(c, ma)
				want, wantExt := interp.interpolateWithWarningOld(c, ma)
				assertItemBits(t, name, got, want, gotExt, wantExt)
			}
		}
	}
}

// TestE_ProductionVsOldOracle_LoadOrder 验证 PRB 加载顺序不影响等价性。
func TestE_ProductionVsOldOracle_LoadOrder(t *testing.T) {
	base := []synthSpec{defaultSynth(0.2), defaultSynth(0.4), defaultSynth(0.6)}
	reversed := []synthSpec{base[2], base[0], base[1]}
	a := buildInterp(base...)
	b := buildInterp(reversed...)

	for _, kb := range []float64{-1.5, -0.5, 0.0, 0.7, 1.5} {
		for _, ma := range []float64{0.25, 0.4, 0.55, 0.7} {
			got, gotExt := a.interpolateWithWarning(kb, ma)
			want, wantExt := b.interpolateWithWarningOld(kb, ma)
			assertItemBits(t, fmt.Sprintf("loadorder kb=%.3f ma=%.3f", kb, ma), got, want, gotExt, wantExt)
		}
	}
}

// TestE_ProductionVsOldOracle_TwoFiles 两档场景（ratio 精确 0/1/0.5）。
func TestE_ProductionVsOldOracle_TwoFiles(t *testing.T) {
	interp := buildInterp(defaultSynth(0.2), defaultSynth(0.4))
	for _, kb := range []float64{-3.0, -1.0, 0.0, 0.3, 1.0, 3.0} {
		for _, ma := range []float64{0.2, 0.3, 0.4, 0.15, 0.45} {
			name := fmt.Sprintf("kb=%.3f ma=%.3f", kb, ma)
			got, gotExt := interp.interpolateWithWarning(kb, ma)
			want, wantExt := interp.interpolateWithWarningOld(kb, ma)
			assertItemBits(t, name, got, want, gotExt, wantExt)
		}
	}
}

// ==================== 防御性校验（review #2） ====================

// TestE_NonMonotonicMixed_ReturnsNil 构造混合后 Kb 非严格递增的场景，验证
// 生产实现返回 nil（明确失败）而不是继续二分产生未定义行为。
// 构造法：两档 Kb 数量级差异悬殊（1.0 与 1e16）+ 相邻 1 ULP 递增，使凸组合
// 在 IEEE-754 舍入下产生相邻相等。直接构造 calibrationData（绕过文本往返）。
func TestE_NonMonotonicMixed_ReturnsNil(t *testing.T) {
	kbStep := func(base float64) []float64 {
		out := make([]float64, 6)
		cur := base
		for i := 0; i < 6; i++ {
			out[i] = cur
			cur = math.Nextafter(cur, math.Inf(1))
		}
		return out
	}
	// calib1 在 1.0 附近、calib2 在 1e16 附近，ratio≈0.1 时混合出现相邻相等。
	kbs := kbStep(1.0)
	big := kbStep(1e16)
	interp := buildInterpRaw(kbs, big)

	// 独立验证混合确实非单调（ratio 由 ma=0.31 附近触发 ≈0.1）。
	// 0.3 + 0.1*ratio=0.1 -> ma=0.31；calib CMa 为 0.3/0.4。
	mixAt := func(ratio float64) []float64 {
		m := make([]float64, len(kbs))
		for i := range kbs {
			m[i] = kbs[i] + ratio*(big[i]-kbs[i])
		}
		return m
	}
	nonMono := false
	for _, r := range []float64{0.05, 0.1, 0.15, 0.2} {
		m := mixAt(r)
		for i := 1; i < len(m); i++ {
			if m[i] <= m[i-1] {
				nonMono = true
			}
		}
	}
	if !nonMono {
		t.Fatal("测试前提失效：构造的 Kb 序列在所有 ratio 下都严格递增，无法覆盖防御路径")
	}

	// 无论混合结果是否严格递增，interpolateWithWarning 都不应 panic，
	// 且返回值要么与 oracle 一致，要么（非递增时）为 nil 明确失败。
	triggered := false
	check := func(kb, ma float64) {
		t.Helper()
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("kb=%v ma=%v 不应 panic: %v", kb, ma, r)
			}
		}()
		got, _ := interp.interpolateWithWarning(kb, ma)
		want, _ := interp.interpolateWithWarningOld(kb, ma)
		// 若生产返回 nil，说明防御校验生效（明确失败）；否则必须与 oracle 一致。
		if got == nil {
			triggered = true
			return // 防御校验触发，明确失败路径合法
		}
		if want == nil {
			t.Fatalf("kb=%v ma=%v: 生产=%v oracle=nil，不一致", kb, ma, got)
		}
		if math.Float64bits(got.Kb) != math.Float64bits(want.Kb) ||
			math.Float64bits(got.Alpha) != math.Float64bits(want.Alpha) {
			t.Fatalf("kb=%v ma=%v: 生产 Kb=%v Alpha=%v 与 oracle 不一致",
				kb, ma, got.Kb, got.Alpha)
		}
	}

	// Kb 输入取落在混合区间内部的点（1.0 附近），Ma 取触发 ratio≈0.1~0.2 的值。
	for _, kb := range []float64{1.0, math.Nextafter(1.0, math.Inf(1)), 1.0 + 1e-15} {
		for _, ma := range []float64{0.305, 0.31, 0.32, 0.34} {
			check(kb, ma)
		}
	}
	if !triggered {
		t.Fatal("生产防御校验从未触发（始终 got!=nil），无法证明 nil 失败路径被覆盖")
	}
}

func BenchmarkCalculateMultiFile(b *testing.B) {
	interp := buildInterp(defaultSynth(0.2), defaultSynth(0.4), defaultSynth(0.6))
	input := InterpolationInput{P1: 5000, P2: 8000, P3: 6000, PAtm: 101325, TAtm: 20}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := interp.Calculate(input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCalculateMultiFile_KbVaried(b *testing.B) {
	interp := buildInterp(defaultSynth(0.2), defaultSynth(0.4), defaultSynth(0.6))
	rng := rand.New(rand.NewSource(3))
	inputs := make([]InterpolationInput, 64)
	for i := range inputs {
		kb := -2.5 + 5.0*float64(i)/float64(len(inputs)-1)
		inputs[i] = InterpolationInput{P1: 5000 + rng.Float64()*100, P2: 8000, P3: 6000, PAtm: 101325, TAtm: 20}
		_ = kb
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		in := inputs[i%len(inputs)]
		if _, err := interp.Calculate(in); err != nil {
			b.Fatal(err)
		}
	}
}
