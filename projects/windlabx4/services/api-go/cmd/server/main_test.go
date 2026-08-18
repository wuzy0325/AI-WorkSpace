package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestShutdown 有序关停序列：先 registry 后 HTTP；registry 失败时
// 记录 fatal、跳过 HTTP 优雅关闭（共享服务 Close 不被调用）并返回非零退出码。

type fakeRegistryShutter struct {
	calls int
	err   error
}

func (f *fakeRegistryShutter) Shutdown(context.Context) error {
	f.calls++
	return f.err
}

type fakeHTTPShutter struct {
	calls int
}

func (f *fakeHTTPShutter) Shutdown(context.Context) error {
	f.calls++
	return nil
}

func TestShutdown_RegistryThenHTTP(t *testing.T) {
	registry := &fakeRegistryShutter{}
	httpServer := &fakeHTTPShutter{}

	code := runShutdown(registry, httpServer, time.Second)
	if code != 0 {
		t.Fatalf("成功路径应返回 0, got %d", code)
	}
	if registry.calls != 1 {
		t.Fatalf("registry.Shutdown 应调用一次, got %d", registry.calls)
	}
	if httpServer.calls != 1 {
		t.Fatalf("registry 成功后应优雅关闭 HTTP, got %d", httpServer.calls)
	}
}

func TestShutdown_RegistryFailureSkipsSharedClose(t *testing.T) {
	registry := &fakeRegistryShutter{err: errors.New("shutdown_timeout: probe1/task-1")}
	httpServer := &fakeHTTPShutter{}

	code := runShutdown(registry, httpServer, time.Second)
	if code == 0 {
		t.Fatal("registry 失败应返回非零退出码")
	}
	if httpServer.calls != 0 {
		t.Fatal("registry 失败时不得继续关闭共享服务（HTTP 优雅关闭应被跳过）")
	}
}

func TestShutdown_NilRegistryStillClosesHTTP(t *testing.T) {
	httpServer := &fakeHTTPShutter{}
	if code := runShutdown(nil, httpServer, time.Second); code != 0 {
		t.Fatalf("无 registry 时应正常关闭 HTTP, got %d", code)
	}
	if httpServer.calls != 1 {
		t.Fatal("无 registry 时 HTTP 应被关闭")
	}
}
