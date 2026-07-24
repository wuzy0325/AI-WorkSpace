package backend

import (
	"testing"
)

// TestGetAvailableProbes 验证启动选择页返回 3/5/7 三种探针且顺序固定。
// 顺序固定是因为前端卡片排列依赖此顺序，变更需显式改 SPEC。
func TestGetAvailableProbes(t *testing.T) {
	app := NewApp()
	got := app.GetAvailableProbes()

	if len(got) != 3 {
		t.Fatalf("期望返回 3 种探针，实际 %d", len(got))
	}

	expectedKinds := []ProbeKind{ProbeKindThree, ProbeKindFive, ProbeKindSeven}
	for i, want := range expectedKinds {
		if got[i].Kind != want {
			t.Errorf("第 %d 个探针类型期望 %q，实际 %q", i, want, got[i].Kind)
		}
		if got[i].Holes != 3 && got[i].Holes != 5 && got[i].Holes != 7 {
			t.Errorf("第 %d 个探针 Holes 字段异常: %d", i, got[i].Holes)
		}
		if got[i].Name == "" || got[i].Description == "" {
			t.Errorf("第 %d 个探针 Name/Description 不能为空", i)
		}
	}
}

// TestGetAvailableProbes_ReturnsCopy 验证返回的是切片副本，
// 调用方修改返回值不影响后端内部状态。
func TestGetAvailableProbes_ReturnsCopy(t *testing.T) {
	app := NewApp()
	got := app.GetAvailableProbes()
	got[0].Name = "被篡改的名称"

	got2 := app.GetAvailableProbes()
	if got2[0].Name == "被篡改的名称" {
		t.Error("GetAvailableProbes 返回了内部切片的引用，调用方修改会污染后端状态")
	}
}

// TestSetActiveProbe_FirstCallSucceeds 验证首次设置探针类型成功。
func TestSetActiveProbe_FirstCallSucceeds(t *testing.T) {
	app := NewApp()
	if err := app.SetActiveProbe(ProbeKindFive); err != nil {
		t.Fatalf("首次设置探针类型失败: %v", err)
	}
}

// TestSetActiveProbe_CanSwitch 验证 v0.1.1 起的可切换语义：
// 首次设置后再次调用（同类型或不同类型）均成功覆盖。
// 这是 SPEC § 探针类型切换 的核心约束——支持用户从工作区返回欢迎页再选其他探针。
func TestSetActiveProbe_CanSwitch(t *testing.T) {
	app := NewApp()

	if err := app.SetActiveProbe(ProbeKindFive); err != nil {
		t.Fatalf("首次设置五孔失败: %v", err)
	}

	// 同类型再次设置也应成功（覆盖式更新）
	if err := app.SetActiveProbe(ProbeKindFive); err != nil {
		t.Errorf("第二次设置同类型探针应成功覆盖，实际失败: %v", err)
	}

	// 换类型设置也应成功
	if err := app.SetActiveProbe(ProbeKindThree); err != nil {
		t.Errorf("切换到三孔应成功，实际失败: %v", err)
	}

	kind, err := app.GetActiveProbe()
	if err != nil || kind != ProbeKindThree {
		t.Errorf("切换失败：期望读回 %q，实际 %q (err=%v)", ProbeKindThree, kind, err)
	}
}

// TestSetActiveProbe_InvalidKind 验证传入未知类型字符串报错。
// 防止前端误传 "8" 或空字符串等无效值。
func TestSetActiveProbe_InvalidKind(t *testing.T) {
	app := NewApp()

	invalidKinds := []ProbeKind{"", "eight", "5", "THREE"}
	for _, kind := range invalidKinds {
		err := app.SetActiveProbe(kind)
		if err == nil {
			t.Errorf("传入无效探针类型 %q 应报错，实际成功", kind)
		}
	}
}

// TestGetActiveProbe_BeforeSelection 验证未选择时 GetActiveProbe 返回空字符串 + nil error。
// v0.1.1 起不再返回 error，前端按空字符串判定仍停留在选择页。
func TestGetActiveProbe_BeforeSelection(t *testing.T) {
	app := NewApp()
	kind, err := app.GetActiveProbe()
	if err != nil {
		t.Errorf("未选择探针时不应返回 error，实际: %v", err)
	}
	if kind != "" {
		t.Errorf("未选择时返回的 kind 应为空字符串，实际 %q", kind)
	}
}

// TestGetActiveProbe_AfterSelection 验证设置后能正确读回。
func TestGetActiveProbe_AfterSelection(t *testing.T) {
	app := NewApp()

	if err := app.SetActiveProbe(ProbeKindSeven); err != nil {
		t.Fatalf("设置七孔探针失败: %v", err)
	}

	kind, err := app.GetActiveProbe()
	if err != nil {
		t.Fatalf("设置后读取失败: %v", err)
	}
	if kind != ProbeKindSeven {
		t.Errorf("期望读回 %q，实际 %q", ProbeKindSeven, kind)
	}
}

// TestClearActiveProbe 验证 ClearActiveProbe 把激活状态清空为空字符串，
// 配合前端"返回欢迎页"按钮使用。
func TestClearActiveProbe(t *testing.T) {
	app := NewApp()

	if err := app.SetActiveProbe(ProbeKindFive); err != nil {
		t.Fatalf("设置五孔探针失败: %v", err)
	}

	app.ClearActiveProbe()

	kind, err := app.GetActiveProbe()
	if err != nil {
		t.Fatalf("Clear 后 GetActiveProbe 不应报错: %v", err)
	}
	if kind != "" {
		t.Errorf("Clear 后 kind 应为空字符串，实际 %q", kind)
	}
}

// TestClearActiveProbe_Idempotent 验证多次 Clear 不报错（幂等）。
// 防止前端重复触发返回按钮导致异常。
func TestClearActiveProbe_Idempotent(t *testing.T) {
	app := NewApp()

	app.ClearActiveProbe() // 未选择时 Clear
	app.ClearActiveProbe() // 再次 Clear

	kind, _ := app.GetActiveProbe()
	if kind != "" {
		t.Errorf("多次 Clear 后 kind 应仍为空字符串，实际 %q", kind)
	}
}

// TestSetActiveProbe_ConcurrentSafety 验证并发调用 SetActiveProbe 时，
// 不会出现数据竞争（race detector 会在 -race 模式下捕获）。
// 由于 v0.1.1 改为覆盖式更新，所有并发调用都会"成功"，
// 最终值由最后一个写入决定，这里只验证最终状态是三种合法值之一。
func TestSetActiveProbe_ConcurrentSafety(t *testing.T) {
	app := NewApp()

	kinds := []ProbeKind{ProbeKindThree, ProbeKindFive, ProbeKindSeven}
	const goroutines = 30
	done := make(chan struct{})

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			_ = app.SetActiveProbe(kinds[idx%len(kinds)])
			done <- struct{}{}
		}(i)
	}

	for i := 0; i < goroutines; i++ {
		<-done
	}

	// 最终值必须是三种合法值之一（race-free 的覆盖式更新）
	kind, err := app.GetActiveProbe()
	if err != nil {
		t.Fatalf("并发后 GetActiveProbe 不应报错: %v", err)
	}
	if !isValidProbeKind(kind) {
		t.Errorf("并发后 kind 应为合法值，实际 %q", kind)
	}
}
