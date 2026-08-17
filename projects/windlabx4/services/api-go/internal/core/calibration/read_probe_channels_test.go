package calibration

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// TestWaitForFreshDataWaitsWhenAcquisitionStopsAndResumes 回归测试：
// 采样过程中设备停止采集后重启采集，waitForFreshData 应当继续等待并最终完成，
// 而不是超时判失败。
//
// 测试前置：
//   - timestampReader：前 200ms 返回旧帧（ts=1），200ms 后返回新帧（ts=2）
//   - acquiringCheck：前 200ms 返回 false（用户停采集），200ms 后返回 true（恢复采集）
//   - timeout：50ms（远小于等待时长，验证超时后被 acquiringCheck 接管继续等待）
//
// 测试步骤：
//   - 异步调用 waitForFreshData（lastTimestamps 预置 dev-1=1，期望等到 ts>1 的新帧）
//   - 等待 200ms 让设备"恢复采集"
//
// 期待结果：
//   - 不返回错误（等到新帧后正常返回 nil）
//   - elapsed >= 200ms（确实在等待恢复，而非立即返回）
func TestWaitForFreshDataWaitsWhenAcquisitionStopsAndResumes(t *testing.T) {
	var ts int64 = 1
	var acquiring int32 = 0 // 0=未采集, 1=在采集

	tsReader := func(deviceID string) (int64, bool) {
		return atomic.LoadInt64(&ts), true
	}
	acquiringCheck := func(deviceID string) bool {
		return atomic.LoadInt32(&acquiring) == 1
	}

	// 200ms 后恢复采集 + 推送新帧
	go func() {
		time.Sleep(200 * time.Millisecond)
		atomic.StoreInt32(&acquiring, 1)
		atomic.StoreInt64(&ts, 2)
	}()

	last := map[string]int64{"dev-1": 1}
	start := time.Now()
	err := waitForFreshData(
		[]string{"dev-1"},
		tsReader,
		last,
		50*time.Millisecond, // 短超时：验证超时后被 acquiringCheck 接管
		nil,
		acquiringCheck,
	)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected waitForFreshData to wait and succeed, got error: %v", err)
	}
	if elapsed < 200*time.Millisecond {
		t.Fatalf("elapsed = %v, want >= 200ms (should have waited for acquisition resume)", elapsed)
	}
}

func TestWaitForFreshDataDoesNotCountStoppedTimeBeforeDeadline(t *testing.T) {
	start := time.Now()
	tsReader := func(string) (int64, bool) {
		if time.Since(start) >= 140*time.Millisecond {
			return 2, true
		}
		return 1, true
	}
	acquiringCheck := func(string) bool {
		return time.Since(start) >= 90*time.Millisecond
	}

	err := waitForFreshData(
		[]string{"dev-1"},
		tsReader,
		map[string]int64{"dev-1": 1},
		100*time.Millisecond,
		nil,
		acquiringCheck,
	)

	if err != nil {
		t.Fatalf("expected stopped time to be excluded from timeout, got error: %v", err)
	}
}

// TestWaitForFreshDataFailsWhenAcquiringButNoFreshData 回归测试：
// 设备持续在采集（acquiringCheck=true）但帧不更新时，waitForFreshData 应在超时后返回错误。
// 验证"真异常"路径不被"等待恢复"逻辑破坏。
//
// 测试前置：
//   - timestampReader：恒返回 ts=1（旧帧，不更新）
//   - acquiringCheck：恒返回 true（设备在采集）
//   - timeout：50ms
//
// 测试步骤：
//   - 同步调用 waitForFreshData（lastTimestamps 预置 dev-1=1）
//
// 期待结果：
//   - 返回非 nil 错误（超时）
//   - elapsed >= 50ms（确实等到了超时）
func TestWaitForFreshDataFailsWhenAcquiringButNoFreshData(t *testing.T) {
	tsReader := func(deviceID string) (int64, bool) {
		return 1, true // 恒返回旧帧
	}
	acquiringCheck := func(deviceID string) bool {
		return true // 恒在采集
	}

	last := map[string]int64{"dev-1": 1}
	start := time.Now()
	err := waitForFreshData(
		[]string{"dev-1"},
		tsReader,
		last,
		50*time.Millisecond,
		nil,
		acquiringCheck,
	)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed < 50*time.Millisecond {
		t.Fatalf("elapsed = %v, want >= 50ms (should have waited until timeout)", elapsed)
	}
}

// TestWaitForFreshDataReturnsAbortedWhenCheckAbortFiresDuringWait 回归测试：
// 等待恢复期间 checkAbort 返回 true 时，应立即返回 ErrPointAborted，不无限挂起。
//
// 测试前置：
//   - timestampReader：恒返回旧帧
//   - acquiringCheck：恒返回 false（设备持续未采集，模拟用户停采集后未恢复）
//   - checkAbort：100ms 后返回 true（模拟用户停止校准）
//   - timeout：50ms
//
// 测试步骤：
//   - 同步调用 waitForFreshData
//
// 期待结果：
//   - 返回 ErrPointAborted
//   - elapsed 在 100ms 附近（checkAbort 触发后立即返回，不等待 stallDeadline）
func TestWaitForFreshDataReturnsAbortedWhenCheckAbortFiresDuringWait(t *testing.T) {
	tsReader := func(deviceID string) (int64, bool) {
		return 1, true
	}
	acquiringCheck := func(deviceID string) bool {
		return false // 持续未采集
	}

	var aborted int32 = 0
	checkAbort := func() bool {
		return atomic.LoadInt32(&aborted) == 1
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		atomic.StoreInt32(&aborted, 1)
	}()

	last := map[string]int64{"dev-1": 1}
	start := time.Now()
	err := waitForFreshData(
		[]string{"dev-1"},
		tsReader,
		last,
		50*time.Millisecond,
		checkAbort,
		acquiringCheck,
	)
	elapsed := time.Since(start)

	if !errors.Is(err, ErrPointAborted) {
		t.Fatalf("expected ErrPointAborted, got %v", err)
	}
	if elapsed < 100*time.Millisecond {
		t.Fatalf("elapsed = %v, want >= 100ms (should wait for checkAbort)", elapsed)
	}
	if elapsed >= 1*time.Second {
		t.Fatalf("elapsed = %v, want < 1s (should return promptly after checkAbort)", elapsed)
	}
}

// TestWaitForFreshDataReturnsCtxCancelledDuringWait 回归测试：
// 等待恢复期间 ctx 取消时，应立即返回 ctx.Err()，不无限挂起。
//
// 测试前置：
//   - timestampReader：恒返回旧帧
//   - acquiringCheck：恒返回 false（持续未采集）
//   - ctx：100ms 后取消
//   - timeout：50ms
//
// 测试步骤：
//   - 同步调用 waitForFreshDataContext
//
// 期待结果：
//   - 返回 context.Canceled
//   - elapsed 在 100ms 附近
func TestWaitForFreshDataReturnsCtxCancelledDuringWait(t *testing.T) {
	tsReader := func(deviceID string) (int64, bool) {
		return 1, true
	}
	acquiringCheck := func(deviceID string) bool {
		return false
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	last := map[string]int64{"dev-1": 1}
	start := time.Now()
	err := waitForFreshDataContext(
		ctx,
		[]string{"dev-1"},
		tsReader,
		last,
		50*time.Millisecond,
		nil,
		acquiringCheck,
	)
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if elapsed < 100*time.Millisecond {
		t.Fatalf("elapsed = %v, want >= 100ms (should wait for ctx cancel)", elapsed)
	}
	if elapsed >= 1*time.Second {
		t.Fatalf("elapsed = %v, want < 1s (should return promptly after ctx cancel)", elapsed)
	}
}
