package monitor

import (
	"errors"
	"strings"
	"testing"
	"time"

	"shared.local/device-sdk/go/motion/core"
)

// TestControllerStatusSnapshotFields 验证 ControllerStatusSnapshot 包含 spec Data Model
// 要求的全部字段。任何字段缺失会导致编译失败。
func TestControllerStatusSnapshotFields(t *testing.T) {
	now := time.Now()
	validUntil := now.Add(500 * time.Millisecond)
	snap := ControllerStatusSnapshot{
		ControllerID: "b140-1",
		Generation:   1,
		Sequence:     5,
		AttemptedAt:  now,
		SucceededAt:  now,
		ValidUntil:   validUntil,
		Status: core.ControllerStatus{
			ID:        "b140-1",
			Name:      "B140",
			Connected: true,
			Axes:      []core.AxisStatus{{Name: core.AxisX, Position: 10, Moving: true}},
		},
		Err: nil,
	}

	if snap.ControllerID != "b140-1" {
		t.Fatalf("ControllerID = %q, want %q", snap.ControllerID, "b140-1")
	}
	if snap.Generation != 1 {
		t.Fatalf("Generation = %d, want 1", snap.Generation)
	}
	if snap.Sequence != 5 {
		t.Fatalf("Sequence = %d, want 5", snap.Sequence)
	}
	if !snap.ValidUntil.Equal(validUntil) {
		t.Fatalf("ValidUntil = %v, want %v", snap.ValidUntil, validUntil)
	}
	if snap.Status.Axes[0].Position != 10 {
		t.Fatalf("Status.Axes[0].Position = %v, want 10", snap.Status.Axes[0].Position)
	}
}

// TestStatusSnapshotFields 验证聚合视图 StatusSnapshot 包含 spec 要求的全部字段。
func TestStatusSnapshotFields(t *testing.T) {
	now := time.Now()
	snap := StatusSnapshot{
		Sequence:    3,
		PublishedAt: now,
		Controllers: []ControllerStatusSnapshot{
			{ControllerID: "b140-1", Sequence: 5},
		},
	}
	if snap.Sequence != 3 {
		t.Fatalf("Sequence = %d, want 3", snap.Sequence)
	}
	if len(snap.Controllers) != 1 {
		t.Fatalf("len(Controllers) = %d, want 1", len(snap.Controllers))
	}
}

// TestErrGenerationChangedImplementsError 验证 ErrGenerationChanged 实现 error 接口，
// 且 Error() 方法包含 ControllerID 与 Old/New Generation 信息——这是排查 generation 切换
// 问题的关键诊断信息。
func TestErrGenerationChangedImplementsError(t *testing.T) {
	err := ErrGenerationChanged{ControllerID: "b140-1", OldGen: 2, NewGen: 3}
	var asError error = &err
	if asError.Error() == "" {
		t.Fatal("ErrGenerationChanged.Error() is empty")
	}
	// 必须包含 controller ID 和 generation 信息
	msg := asError.Error()
	if !strings.Contains(msg, "b140-1") {
		t.Errorf("Error() = %q, want substring %q", msg, "b140-1")
	}
	if !strings.Contains(msg, "2") || !strings.Contains(msg, "3") {
		t.Errorf("Error() = %q, want OldGen=2 and NewGen=3", msg)
	}

	// 验证 1：errors.As 能识别类型并提取字段
	var target *ErrGenerationChanged
	if !errors.As(asError, &target) {
		t.Fatal("errors.As failed to identify ErrGenerationChanged")
	}
	if target.ControllerID != "b140-1" || target.OldGen != 2 || target.NewGen != 3 {
		t.Fatalf("errors.As extracted = %+v, want {b140-1, 2, 3}", target)
	}

	// 验证 2：Is 方法按 ControllerID 匹配（同 ID 视为同类错误）
	// ErrGenerationChanged.Is 只比较 ControllerID，因此同 ID 的零值 target 也应匹配。
	if !errors.Is(asError, &ErrGenerationChanged{ControllerID: "b140-1"}) {
		t.Fatal("errors.Is should match same ControllerID")
	}
	// 不同 ControllerID 不应匹配
	if errors.Is(asError, &ErrGenerationChanged{ControllerID: "other"}) {
		t.Fatal("errors.Is should not match different ControllerID")
	}
}

// TestCommandKindConstants 验证 CommandKind 枚举包含 spec Interface Contract 要求的三类命令，
// 用于 NotifyCommandExecuted 触发不同 refresh 策略。
func TestCommandKindConstants(t *testing.T) {
	if CmdKindMove == CmdKindStop || CmdKindMove == CmdKindConfig || CmdKindStop == CmdKindConfig {
		t.Fatal("CmdKindMove, CmdKindStop, CmdKindConfig must be distinct values")
	}
}

// TestFreshnessPolicyDefaultImplementation 验证默认 FreshnessPolicy 实现：
// - Age = now - SucceededAt
// - IsStale = now > ValidUntil
// - Err != nil 但 SucceededAt 新鲜时 IsStale=false（保留最后可信状态）
// 这是 spec Decision 4 "新鲜度是安全契约" 的最小可执行实现。
func TestFreshnessPolicyDefaultImplementation(t *testing.T) {
	policy := DefaultFreshnessPolicy{
		StaleThreshold: 500 * time.Millisecond,
	}
	now := time.Now()
	snap := ControllerStatusSnapshot{
		ControllerID: "b140-1",
		Generation:   1,
		Sequence:     5,
		SucceededAt:  now.Add(-100 * time.Millisecond), // 100ms 前成功
		ValidUntil:   now.Add(400 * time.Millisecond),  // 400ms 后过期
		Err:          nil,
	}

	f := policy.Freshness(now, snap)
	if f.Age != 100*time.Millisecond {
		t.Fatalf("Age = %v, want 100ms", f.Age)
	}
	if f.IsStale {
		t.Fatal("IsStale = true, want false (within ValidUntil)")
	}

	// 超过 ValidUntil：应判定为 stale
	staleSnap := snap
	staleSnap.ValidUntil = now.Add(-50 * time.Millisecond) // 50ms 前已过期
	f = policy.Freshness(now, staleSnap)
	if !f.IsStale {
		t.Fatal("IsStale = false, want true (now > ValidUntil)")
	}

	// Err != nil 但 SucceededAt 仍新鲜：IsStale=false（保留最后可信状态用于诊断）
	errSnap := snap
	errSnap.Err = errors.New("transient io error")
	f = policy.Freshness(now, errSnap)
	if f.IsStale {
		t.Fatal("IsStale = true, want false (SucceededAt still fresh despite Err)")
	}
}

// TestFreshnessPolicyEmptySnapshot 验证零值快照（首帧未产生）被判定为 stale，
// 防止消费者把"未启动"状态当作实时状态。
func TestFreshnessPolicyEmptySnapshot(t *testing.T) {
	policy := DefaultFreshnessPolicy{StaleThreshold: 500 * time.Millisecond}
	now := time.Now()
	empty := ControllerStatusSnapshot{ControllerID: "b140-1"}

	f := policy.Freshness(now, empty)
	if !f.IsStale {
		t.Fatal("IsStale = false for empty snapshot, want true (no SucceededAt)")
	}
}
