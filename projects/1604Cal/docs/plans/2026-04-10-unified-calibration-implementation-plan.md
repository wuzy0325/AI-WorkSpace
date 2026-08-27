# Unified Calibration MVP Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在新仓库中落地可运行的统一系统 MVP 骨架（Vue3 + Go API），打通“设备管理 -> 会话状态机 -> 事件推送 -> 报告模板选择”的最小闭环，并满足项目规范。

**Architecture:** 后端采用 `cmd + internal` 分层，REST 负责命令、SSE 负责实时事件；前端采用 Vue3 + Pinia + Vue Router 的工作台结构。设备驱动先用接口 + 内存桩实现，优先稳定边界与测试闭环，再接真实协议。

**Tech Stack:** Go 1.22+, chi/gin（二选一，默认 chi）, SQLite（先可选内存实现）, Vue3, Vite, TypeScript, Pinia, Vitest, Playwright（后续）

---

## Execution Notes

- 执行时必须遵循：`@test-driven-development`、`@verification-before-completion`。
- 遇到异常行为先进入：`@systematic-debugging`。
- 所有公开类型、关键逻辑、状态机迁移必须补中文注释。

---

### Task 1: 初始化 Go API 服务并打通健康检查

**Files:**
- Create: `go.mod`
- Create: `cmd/server/main.go`
- Create: `internal/api/http/router.go`
- Create: `internal/api/http/health_handler.go`
- Test: `internal/api/http/health_handler_test.go`

**Step 1: Write the failing test**

```go
package http_test

import (
    "net/http"
    "net/http/httptest"
    "testing"

    api "unified-calibration/internal/api/http"
)

func TestHealthHandler(t *testing.T) {
    r := api.NewRouter()
    req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
    rec := httptest.NewRecorder()

    r.ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", rec.Code)
    }
    if rec.Body.String() == "" {
        t.Fatal("expected non-empty body")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api/http -run TestHealthHandler -v`
Expected: FAIL with `package ... does not exist` or `undefined: NewRouter`

**Step 3: Write minimal implementation**

```go
// internal/api/http/router.go
package http

import "net/http"

func NewRouter() http.Handler {
    mux := http.NewServeMux()
    mux.HandleFunc("/api/v1/health", healthHandler)
    return mux
}
```

```go
// internal/api/http/health_handler.go
package http

import (
    "encoding/json"
    "net/http"
)

type healthResponse struct {
    Status string `json:"status"`
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(healthResponse{Status: "ok"})
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/api/http -run TestHealthHandler -v`
Expected: PASS

**Step 5: Commit**

```bash
git add go.mod cmd/server/main.go internal/api/http/router.go internal/api/http/health_handler.go internal/api/http/health_handler_test.go
git commit -m "feat: bootstrap go api and health endpoint"
```

---

### Task 2: 统一 API 响应结构与错误码映射

**Files:**
- Create: `internal/api/dto/response.go`
- Create: `internal/errors/codes.go`
- Create: `internal/api/http/error_mapper.go`
- Test: `internal/api/http/error_mapper_test.go`

**Step 1: Write the failing test**

```go
func TestWriteErrorResponse(t *testing.T) {
    rec := httptest.NewRecorder()
    writeError(rec, ErrUnitMismatch)

    if rec.Code != http.StatusBadRequest {
        t.Fatalf("want 400 got %d", rec.Code)
    }
    if !strings.Contains(rec.Body.String(), "UNIT_MISMATCH") {
        t.Fatal("expected UNIT_MISMATCH in response body")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api/http -run TestWriteErrorResponse -v`
Expected: FAIL with `undefined: writeError`

**Step 3: Write minimal implementation**

```go
// internal/errors/codes.go
package errors

import "errors"

var (
    ErrUnitMismatch = errors.New("unit mismatch")
)
```

```go
// internal/api/dto/response.go
package dto

type Response[T any] struct {
    Success bool   `json:"success"`
    Code    string `json:"code,omitempty"`
    Message string `json:"message,omitempty"`
    Data    T      `json:"data,omitempty"`
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/api/http -run TestWriteErrorResponse -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/dto/response.go internal/errors/codes.go internal/api/http/error_mapper.go internal/api/http/error_mapper_test.go
git commit -m "feat: add unified api response envelope and error mapping"
```

---

### Task 3: 建立设备领域模型与驱动接口

**Files:**
- Create: `internal/domain/device.go`
- Create: `internal/device/interfaces.go`
- Test: `internal/device/interfaces_test.go`

**Step 1: Write the failing test**

```go
func TestDeviceTypeValues(t *testing.T) {
    if domain.DeviceTypeMeasure == "" || domain.DeviceTypePressure == "" {
        t.Fatal("device type constants should not be empty")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/device -run TestDeviceTypeValues -v`
Expected: FAIL with missing package/type

**Step 3: Write minimal implementation**

```go
// internal/domain/device.go
package domain

type DeviceType string

const (
    DeviceTypePressure DeviceType = "pressure"
    DeviceTypeMeasure  DeviceType = "measure"
)

type Device struct {
    ID    string
    Name  string
    Type  DeviceType
    Model string
    Host  string
    Port  int
    Unit  string
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/device -run TestDeviceTypeValues -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/domain/device.go internal/device/interfaces.go internal/device/interfaces_test.go
git commit -m "feat: define device domain model and driver contracts"
```

---

### Task 4: 实现 DeviceManager（内存版）与单位一致性校验

**Files:**
- Create: `internal/device/manager/device_manager.go`
- Test: `internal/device/manager/device_manager_test.go`

**Step 1: Write the failing test**

```go
func TestCheckUnitConsistency(t *testing.T) {
    mgr := manager.NewDeviceManager()
    mgr.Upsert(domain.Device{ID: "m1", Type: domain.DeviceTypeMeasure, Unit: "kPa"})
    mgr.Upsert(domain.Device{ID: "p1", Type: domain.DeviceTypePressure, Unit: "kPa"})

    ok, _ := mgr.CheckUnitConsistency()
    if !ok {
        t.Fatal("expected units to be consistent")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/device/manager -run TestCheckUnitConsistency -v`
Expected: FAIL with `undefined: NewDeviceManager`

**Step 3: Write minimal implementation**

```go
type DeviceManager struct {
    mu      sync.RWMutex
    devices map[string]domain.Device
}

func (m *DeviceManager) CheckUnitConsistency() (bool, []string) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    // 返回是否一致和不一致设备ID列表
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/device/manager -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/device/manager/device_manager.go internal/device/manager/device_manager_test.go
git commit -m "feat: add in-memory device manager with unit consistency checks"
```

---

### Task 5: 实现会话状态机（自动/手动主流程）

**Files:**
- Create: `internal/domain/session_state.go`
- Create: `internal/workflow/session_machine.go`
- Test: `internal/workflow/session_machine_test.go`

**Step 1: Write the failing test**

```go
func TestSessionStateTransition(t *testing.T) {
    m := workflow.NewSessionMachine()
    if err := m.Transition(domain.SessionStateReady); err != nil {
        t.Fatalf("unexpected transition error: %v", err)
    }
    if got := m.State(); got != domain.SessionStateReady {
        t.Fatalf("want ready got %s", got)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/workflow -run TestSessionStateTransition -v`
Expected: FAIL with missing state machine implementation

**Step 3: Write minimal implementation**

```go
// SessionState 包含 idle/ready/pressurizing/stabilizing/collecting/...
// SessionMachine 使用白名单校验迁移合法性
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/workflow -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/domain/session_state.go internal/workflow/session_machine.go internal/workflow/session_machine_test.go
git commit -m "feat: add session state machine with transition whitelist"
```

---

### Task 6: 实现稳定判定与报警规则服务

**Files:**
- Create: `internal/workflow/stability_service.go`
- Create: `internal/workflow/alarm_service.go`
- Test: `internal/workflow/stability_service_test.go`
- Test: `internal/workflow/alarm_service_test.go`

**Step 1: Write the failing tests**

```go
func TestStabilityAccumulatorResetsOnDrift(t *testing.T) {
    // 超出阈值后累计时间应归零
}

func TestAlarmDecisionAllowsContinueOrRetryOnly(t *testing.T) {
    // 非 continue/retry 必须报错
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/workflow -run "TestStability|TestAlarm" -v`
Expected: FAIL with missing implementation

**Step 3: Write minimal implementation**

```go
// StabilityService:
// 输入 target/actual/tolerance/sampleInterval
// 输出是否稳定、累计稳定时长

// AlarmService:
// 输入目标值/实测值/level
// 输出是否超差与偏差百分比
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/workflow -run "TestStability|TestAlarm" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/workflow/stability_service.go internal/workflow/alarm_service.go internal/workflow/stability_service_test.go internal/workflow/alarm_service_test.go
git commit -m "feat: add stability accumulator and alarm decision rules"
```

---

### Task 7: 实现事件总线与 SSE 推送接口

**Files:**
- Create: `internal/events/bus.go`
- Create: `internal/api/http/events_handler.go`
- Modify: `internal/api/http/router.go`
- Test: `internal/api/http/events_handler_test.go`

**Step 1: Write the failing test**

```go
func TestSSEEndpointStreamsEvents(t *testing.T) {
    // 订阅后发布一条 session.state.changed 事件，应在响应流中读到
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api/http -run TestSSEEndpointStreamsEvents -v`
Expected: FAIL with missing handler

**Step 3: Write minimal implementation**

```go
// bus.go: Publish/Subscribe/Unsubscribe
// events_handler.go: SSE header + flush loop
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/api/http -run TestSSEEndpointStreamsEvents -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/events/bus.go internal/api/http/events_handler.go internal/api/http/router.go internal/api/http/events_handler_test.go
git commit -m "feat: add internal event bus and sse stream endpoint"
```

---

### Task 8: 实现报告模板选择器（2-11 点 s/m）

**Files:**
- Create: `internal/report/template_selector.go`
- Create: `internal/report/report_service.go`
- Test: `internal/report/template_selector_test.go`

**Step 1: Write the failing test**

```go
func TestSelectTemplate(t *testing.T) {
    got, err := report.SelectTemplate(5, "single")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if got != "5s.xlsx" {
        t.Fatalf("want 5s.xlsx got %s", got)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/report -run TestSelectTemplate -v`
Expected: FAIL with undefined function

**Step 3: Write minimal implementation**

```go
// SelectTemplate(points int, mode string) -> "{points}{s|m}.xlsx"
// points 范围必须是 2..11，超出返回业务错误
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/report -run TestSelectTemplate -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/report/template_selector.go internal/report/report_service.go internal/report/template_selector_test.go
git commit -m "feat: add report template selection with compatibility rules"
```

---

### Task 9: 初始化前端工程并建立工作台基础页

**Files:**
- Create: `web/package.json`
- Create: `web/vite.config.ts`
- Create: `web/src/main.ts`
- Create: `web/src/App.vue`
- Create: `web/src/router/index.ts`
- Create: `web/src/views/WorkbenchView.vue`
- Create: `web/src/stores/deviceStore.ts`
- Create: `web/src/services/apiClient.ts`
- Test: `web/src/views/__tests__/WorkbenchView.test.ts`

**Step 1: Write the failing test**

```ts
import { mount } from '@vue/test-utils'
import WorkbenchView from '../WorkbenchView.vue'

it('renders workbench title', () => {
  const wrapper = mount(WorkbenchView)
  expect(wrapper.text()).toContain('统一校准工作台')
})
```

**Step 2: Run test to verify it fails**

Run: `cd web && npm run test -- WorkbenchView.test.ts`
Expected: FAIL with missing component/setup

**Step 3: Write minimal implementation**

```vue
<template>
  <section>
    <h1>统一校准工作台</h1>
    <p>设备管理与采集流程将集成在本页。</p>
  </section>
</template>
```

**Step 4: Run test to verify it passes**

Run: `cd web && npm run test -- WorkbenchView.test.ts`
Expected: PASS

**Step 5: Commit**

```bash
git add web/package.json web/vite.config.ts web/src/main.ts web/src/App.vue web/src/router/index.ts web/src/views/WorkbenchView.vue web/src/stores/deviceStore.ts web/src/services/apiClient.ts web/src/views/__tests__/WorkbenchView.test.ts
git commit -m "feat: scaffold vue frontend and workbench base page"
```

---

### Task 10: 质量门禁与开发说明落地

**Files:**
- Create: `Makefile`
- Create: `scripts/check.ps1`
- Create: `docs/plans/implementation-checklist.md`
- Modify: `AGENTS.md`

**Step 1: Write the failing verification command expectation**

```text
目标：执行一次统一检查命令失败（因为脚本不存在），再补齐脚本并通过。
```

**Step 2: Run check command to verify it fails**

Run: `pwsh ./scripts/check.ps1`
Expected: FAIL with `script not found`

**Step 3: Write minimal implementation**

```powershell
# scripts/check.ps1
go test ./...
go vet ./...
npm --prefix web run typecheck
npm --prefix web run lint
```

**Step 4: Run check command to verify it passes**

Run: `pwsh ./scripts/check.ps1`
Expected: PASS (all checks green)

**Step 5: Commit**

```bash
git add Makefile scripts/check.ps1 docs/plans/implementation-checklist.md AGENTS.md
git commit -m "chore: add quality gates and implementation checklist"
```

---

## Final Verification Checklist

在执行完全部任务后，统一执行：

1. `go test ./...`
2. `go vet ./...`
3. `staticcheck ./...`（已安装时）
4. `npm --prefix web run typecheck`
5. `npm --prefix web run lint`
6. `npm --prefix web run test`

期望结果：所有命令通过，且工作台可在本机打开并调用健康检查 API。
