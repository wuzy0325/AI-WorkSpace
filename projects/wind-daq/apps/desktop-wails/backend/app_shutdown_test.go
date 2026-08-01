package backend

import (
	"context"
	"errors"
	"testing"

	"wind-daq/services/api-go/pkg/appcontext"
)

// ServiceShutdown 双探针 registry 关停顺序与 fatal seam（spec FR9 / Task 14）：
//   - registry Shutdown 在 relay/motion window/calibration/共享服务 cleanup 之前；
//   - 失败时记录 fatal、走非零 exit seam、跳过全部后续共享服务 Close；
//   - 成功时按既有顺序继续清理，不触发 exit seam。

func TestServiceShutdown_RegistryFailureFatalExitSeam(t *testing.T) {
	var exitCodes []int
	relayStopped := false
	app := &App{
		appContext: &appcontext.AppContext{},
		traversalShutdown: func(context.Context) error {
			return errors.New("shutdown_timeout: probe1/probe1-task-1")
		},
		hostExit:  func(code int) { exitCodes = append(exitCodes, code) },
		relayStop: func() { relayStopped = true },
	}

	err := app.ServiceShutdown()
	if err == nil {
		t.Fatal("registry shutdown 失败应返回错误")
	}
	if len(exitCodes) != 1 || exitCodes[0] != 2 {
		t.Fatalf("fatal 路径应走非零 exit seam(2): %v", exitCodes)
	}
	if relayStopped {
		t.Fatal("registry shutdown 失败时不得继续执行共享服务 cleanup（relayStop 应被跳过）")
	}
}

func TestServiceShutdown_RegistrySuccessContinuesCleanup(t *testing.T) {
	exitCalled := false
	registryCalled := false
	relayStopped := false
	app := &App{
		appContext: &appcontext.AppContext{},
		traversalShutdown: func(context.Context) error {
			registryCalled = true
			return nil
		},
		hostExit:  func(int) { exitCalled = true },
		relayStop: func() { relayStopped = true },
	}

	if err := app.ServiceShutdown(); err != nil {
		t.Fatalf("registry 成功时不应返回错误: %v", err)
	}
	if !registryCalled {
		t.Fatal("registry Shutdown 应在共享服务 cleanup 前调用")
	}
	if !relayStopped {
		t.Fatal("registry 成功时应继续执行共享服务 cleanup")
	}
	if exitCalled {
		t.Fatal("registry 成功时不得触发 fatal exit seam")
	}
}

func TestServiceShutdown_NoRegistryProceeds(t *testing.T) {
	// 无 registry（旧装配/ nil appContext）：跳过 registry 阶段，按既有顺序清理。
	relayStopped := false
	app := &App{relayStop: func() { relayStopped = true }}
	if err := app.ServiceShutdown(); err != nil {
		t.Fatalf("无 registry 时不应报错: %v", err)
	}
	if !relayStopped {
		t.Fatal("无 registry 时共享服务 cleanup 应照常执行")
	}
}

func TestRequestExit_StartsHostExitWatchdog(t *testing.T) {
	watchdogStarted := false
	app := &App{
		exitWatchdog: func() { watchdogStarted = true },
	}

	response := app.RequestExit()

	if !response.Success {
		t.Fatalf("退出请求应立即成功: %+v", response)
	}
	if !watchdogStarted {
		t.Fatal("退出请求必须启动宿主退出 watchdog，避免 Wails/WebView2 清理卡住后进程残留")
	}
}
