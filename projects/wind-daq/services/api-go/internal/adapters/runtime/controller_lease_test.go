package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"wind-daq/services/api-go/internal/core/resourcelock"
)

// waitFor 以短间隔轮询条件直至满足或超时（TTL 到期类断言用轮询而非固定 sleep 判定时序）。
func waitFor(t *testing.T, what string, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("等待条件超时: %s", what)
}

func TestWorkflowLease_FixedHolderLifecycle(t *testing.T) {
	svc := resourcelock.New()
	lease := NewWorkflowLease(svc)
	ctx := context.Background()

	if err := lease.Acquire(ctx, "registry", time.Minute); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	// 固定 holder 续约成功
	if err := lease.Renew(ctx, "registry", time.Minute); err != nil {
		t.Fatalf("Renew 同 holder: %v", err)
	}
	// 其它 holder 续约/获取被拒绝
	if err := lease.Renew(ctx, "other", time.Minute); !errors.Is(err, ErrLeaseNotHeld) {
		t.Fatalf("Renew 其它 holder 应返回 ErrLeaseNotHeld, got %v", err)
	}
	if err := lease.Acquire(ctx, "other", time.Minute); !errors.Is(err, resourcelock.ErrLockHeld) {
		t.Fatalf("Acquire 其它 holder 应返回 ErrLockHeld, got %v", err)
	}
	// 错误 holder 释放拒绝
	if err := lease.Release(ctx, "other"); err == nil {
		t.Fatal("Release 错误 holder 应返回错误")
	}
	// 正确 holder 释放后可重新获取
	if err := lease.Release(ctx, "registry"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if err := lease.Acquire(ctx, "registry", time.Minute); err != nil {
		t.Fatalf("释放后重新 Acquire: %v", err)
	}
	// 未持有时续约不得隐式重建 lease
	if err := lease.Release(ctx, "registry"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if err := lease.Renew(ctx, "registry", time.Minute); !errors.Is(err, ErrLeaseNotHeld) {
		t.Fatalf("未持有时 Renew 应返回 ErrLeaseNotHeld, got %v", err)
	}
	if held, _ := svc.IsHeld(WorkflowTraversalResource); held {
		t.Fatal("Renew 不得隐式重建 workflow lease")
	}
}

func TestControllerLease_AcquireContention(t *testing.T) {
	svc := resourcelock.New()
	lease := NewControllerLease(svc)
	ctx := context.Background()

	const goroutines = 16
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan error, goroutines)
	tokens := make(chan string, goroutines)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			token, err := lease.Acquire(ctx, "motion-1", "probe-session", time.Minute)
			if err == nil {
				tokens <- token
			}
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(tokens)

	var successes int
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, resourcelock.ErrLockHeld) {
			t.Fatalf("争抢失败应返回 ErrLockHeld, got %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("并发争抢同一控制器应恰好一个成功, got %d", successes)
	}
	winner := <-tokens
	held, holder := svc.IsHeld(controllerResourceKey("motion-1"))
	if !held || holder != winner {
		t.Fatalf("底层锁 holder 应为胜出 token, held=%v holder=%q", held, holder)
	}
}

func TestControllerLease_RenewSameToken(t *testing.T) {
	svc := resourcelock.New()
	lease := NewControllerLease(svc)
	ctx := context.Background()

	token, err := lease.Acquire(ctx, "motion-1", "probe1", 80*time.Millisecond)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	// 同 token 续约延长 TTL；超过原始 TTL 后 lease 仍然有效
	time.Sleep(50 * time.Millisecond)
	if err := lease.Renew(ctx, token, 200*time.Millisecond); err != nil {
		t.Fatalf("Renew 同 token: %v", err)
	}
	time.Sleep(60 * time.Millisecond) // 已超原始 80ms TTL
	if held, holder := svc.IsHeld(controllerResourceKey("motion-1")); !held || holder != token {
		t.Fatalf("续约后 lease 应仍由同 token 持有, held=%v holder=%q", held, holder)
	}
	if err := lease.Release(ctx, token); err != nil {
		t.Fatalf("Release: %v", err)
	}
}

func TestControllerLease_ReleaseWrongOrOldToken(t *testing.T) {
	svc := resourcelock.New()
	lease := NewControllerLease(svc)
	ctx := context.Background()

	tokenA, err := lease.Acquire(ctx, "motion-1", "probe1", time.Minute)
	if err != nil {
		t.Fatalf("Acquire A: %v", err)
	}
	tokenB, err := lease.Acquire(ctx, "motion-2", "probe2", time.Minute)
	if err != nil {
		t.Fatalf("Acquire B: %v", err)
	}
	// 未知 token 释放/续约被拒绝（Release 只认 token，伪造 token 无法释放他人 lease）
	if err := lease.Release(ctx, "cleas-forged"); !errors.Is(err, ErrUnknownLeaseToken) {
		t.Fatalf("未知 token Release 应返回 ErrUnknownLeaseToken, got %v", err)
	}
	if err := lease.Renew(ctx, "cleas-forged", time.Minute); !errors.Is(err, ErrUnknownLeaseToken) {
		t.Fatalf("未知 token Renew 应返回 ErrUnknownLeaseToken, got %v", err)
	}
	// 正确 token 可释放；释放后再次释放视为未知 token
	if err := lease.Release(ctx, tokenA); err != nil {
		t.Fatalf("Release A: %v", err)
	}
	if err := lease.Release(ctx, tokenA); !errors.Is(err, ErrUnknownLeaseToken) {
		t.Fatalf("重复 Release 应返回 ErrUnknownLeaseToken, got %v", err)
	}
	// motion-2 的 lease 不受 motion-1 释放影响
	if held, holder := svc.IsHeld(controllerResourceKey("motion-2")); !held || holder != tokenB {
		t.Fatalf("motion-2 lease 不应受影响, held=%v holder=%q", held, holder)
	}
	if err := lease.Release(ctx, tokenB); err != nil {
		t.Fatalf("Release B: %v", err)
	}
}

func TestControllerLease_TTLExpiryTakeover(t *testing.T) {
	svc := resourcelock.New()
	lease := NewControllerLease(svc)
	ctx := context.Background()

	oldToken, err := lease.Acquire(ctx, "motion-1", "probe1", 40*time.Millisecond)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	// 等待 TTL 到期（轮询而非固定时序断言）
	waitFor(t, "旧 lease TTL 到期", 2*time.Second, func() bool {
		held, _ := svc.IsHeld(controllerResourceKey("motion-1"))
		return !held
	})
	// 新 session 接管
	newToken, err := lease.Acquire(ctx, "motion-1", "probe1-gen2", time.Minute)
	if err != nil {
		t.Fatalf("接管 Acquire: %v", err)
	}
	if newToken == oldToken {
		t.Fatal("新 lease token 必须与旧 token 不同")
	}
	// 旧 token 不能续约新 session 的 lease
	if err := lease.Renew(ctx, oldToken, time.Minute); !errors.Is(err, ErrLeaseNotHeld) {
		t.Fatalf("旧 token Renew 应返回 ErrLeaseNotHeld, got %v", err)
	}
	// 旧 token 不能释放新 session 的 lease
	if err := lease.Release(ctx, oldToken); err == nil {
		t.Fatal("旧 token Release 应返回错误")
	}
	if held, holder := svc.IsHeld(controllerResourceKey("motion-1")); !held || holder != newToken {
		t.Fatalf("新 lease 不应受旧 token 影响, held=%v holder=%q", held, holder)
	}
}

func TestControllerLease_DifferentControllersParallel(t *testing.T) {
	svc := resourcelock.New()
	lease := NewControllerLease(svc)
	ctx := context.Background()

	const goroutines = 16
	start := make(chan struct{})
	var wg sync.WaitGroup
	tokens := make(chan string, goroutines)
	errs := make(chan error, goroutines)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(controllerID string) {
			defer wg.Done()
			<-start
			token, err := lease.Acquire(ctx, controllerID, "holder-"+controllerID, time.Minute)
			if err != nil {
				errs <- err
				return
			}
			tokens <- token
		}(fmt.Sprintf("motion-%d", g))
	}
	close(start)
	wg.Wait()
	close(tokens)
	close(errs)

	for err := range errs {
		t.Fatalf("不同控制器并行 Acquire 不应冲突: %v", err)
	}
	seen := make(map[string]bool)
	for token := range tokens {
		if seen[token] {
			t.Fatalf("token 重复: %s", token)
		}
		seen[token] = true
	}
	if len(seen) != goroutines {
		t.Fatalf("应全部成功, got %d", len(seen))
	}
	// 任意释放一个 lease 不影响其它控制器
	for token := range seen {
		if err := lease.Release(ctx, token); err != nil {
			t.Fatalf("Release %s: %v", token, err)
		}
	}
}

func TestControllerLease_TokenOpaque(t *testing.T) {
	svc := resourcelock.New()
	lease := NewControllerLease(svc)
	ctx := context.Background()

	token1, err := lease.Acquire(ctx, "motion-1", "probe1", time.Minute)
	if err != nil {
		t.Fatalf("Acquire 1: %v", err)
	}
	token2, err := lease.Acquire(ctx, "motion-2", "probe1", time.Minute)
	if err != nil {
		t.Fatalf("Acquire 2: %v", err)
	}
	// token 不可由 controllerID 推导，且两次签发互不相同
	for _, token := range []string{token1, token2} {
		if strings.Contains(token, "motion-") {
			t.Fatalf("token 不得包含 controller ID 片段: %s", token)
		}
		if !strings.HasPrefix(token, "cleas-") || len(token) != len("cleas-")+32 {
			t.Fatalf("token 格式异常: %s", token)
		}
	}
	if token1 == token2 {
		t.Fatal("两次签发的 token 必须不同")
	}
}
