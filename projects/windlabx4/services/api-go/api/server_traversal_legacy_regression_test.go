package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"windlabx4/services/api-go/internal/usecase"
)

// Task 13：legacy single 路由字节级回归。
//
// 对现有 handler 输出做录制式断言：dual 路由改造（server.go 分流）不得改变
// 单段 /api/traversal/{action} 的请求/响应/错误码字节。任何行为漂移都会在此红灯。
//
// 注意：本文件只断言既有行为，不修改既有测试期望。

func legacyRouter() http.Handler {
	mgr := usecase.NewTraversalManager(nil, nil, nil, nil, nil)
	return NewRouter(Deps{TraversalManager: mgr})
}

func do(t *testing.T, router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func assertResponse(t *testing.T, w *httptest.ResponseRecorder, wantStatus int, wantBody string) {
	t.Helper()
	if w.Code != wantStatus {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, wantStatus, w.Body.String())
	}
	if got := w.Body.String(); got != wantBody {
		t.Fatalf("body = %q, want %q", got, wantBody)
	}
}

// config GET/POST 字节级回归。
func TestServer_LegacyTraversal_Config(t *testing.T) {
	router := legacyRouter()

	// GET 空配置：null
	assertResponse(t, do(t, router, http.MethodGet, "/api/traversal/config", ""), 200, "null\n")

	// POST 保存：success true
	cfg := `{"taskId":"t-1","probeType":"five-hole"}`
	assertResponse(t, do(t, router, http.MethodPost, "/api/traversal/config", cfg), 200, `{"success":true}`+"\n")

	// GET 读回：与 POST 字节完全一致
	assertResponse(t, do(t, router, http.MethodGet, "/api/traversal/config", ""), 200, cfg)

	// PUT：405 空 body
	assertResponse(t, do(t, router, http.MethodPut, "/api/traversal/config", ""), 405, "")
}

// start / status / pause / resume / stop / result 字节级回归（错误路径）。
func TestServer_LegacyTraversal_LifecycleErrors(t *testing.T) {
	router := legacyRouter()

	// start：空配置（无布点）→ 400 精确错误（当前行为录制）
	assertResponse(t, do(t, router, http.MethodPost, "/api/traversal/start", `{}`),
		400, `{"error":"invalid layout: no points generated for pattern \"\"","success":false}`+"\n")

	// start：非法 JSON → 400
	w := do(t, router, http.MethodPost, "/api/traversal/start", `{invalid`)
	if w.Code != 400 {
		t.Fatalf("非法 JSON start 应 400, got %d", w.Code)
	}

	// start：错误 method → 405
	assertResponse(t, do(t, router, http.MethodGet, "/api/traversal/start", ""), 405, "")

	// pause / resume / runPoint / stop（idle）→ 400 精确错误
	assertResponse(t, do(t, router, http.MethodPost, "/api/traversal/pause", ""),
		400, `{"error":"traversal is not running","success":false}`+"\n")
	assertResponse(t, do(t, router, http.MethodPost, "/api/traversal/resume", ""),
		400, `{"error":"traversal is not paused","success":false}`+"\n")
	assertResponse(t, do(t, router, http.MethodPost, "/api/traversal/runPoint", ""),
		400, `{"error":"traversal is not running","success":false}`+"\n")
	// stop：幂等（未运行返回 nil → success true，既有行为）
	assertResponse(t, do(t, router, http.MethodPost, "/api/traversal/stop", ""), 200, `{"success":true}`+"\n")

	// result：缺 taskId → 400；未知 taskId → 404
	assertResponse(t, do(t, router, http.MethodGet, "/api/traversal/result", ""),
		400, `{"error":"taskId query parameter is required","success":false}`+"\n")
	assertResponse(t, do(t, router, http.MethodGet, "/api/traversal/result?taskId=nope", ""),
		404, `{"error":"traversal result not found","success":false}`+"\n")
}

// 未完成测试检测和恢复功能已移除，相关路由统一返回 404。
func TestServer_LegacyTraversal_CheckpointRoutesRemoved(t *testing.T) {
	router := legacyRouter()

	assertResponse(t, do(t, router, http.MethodGet, "/api/traversal/loadCheckpoint", ""), 404, "")
	assertResponse(t, do(t, router, http.MethodPost, "/api/traversal/resumeFromCheckpoint", `{}`), 404, "")
	assertResponse(t, do(t, router, http.MethodPost, "/api/traversal/clearCheckpoint", ""), 404, "")
}

// import / clearInterpolator / calculateRealtime / checkPreconditions 字节级回归。
func TestServer_LegacyTraversal_ImportAndCalcRoutes(t *testing.T) {
	router := legacyRouter()

	// importPrb：不存在文件 → 400（错误前缀稳定）
	w := do(t, router, http.MethodPost, "/api/traversal/importPrb", `{"filePath":"D:/nonexistent-x.prb"}`)
	if w.Code != 400 || !strings.Contains(w.Body.String(), `"success":false`) {
		t.Fatalf("importPrb 错误路径: %d %s", w.Code, w.Body.String())
	}

	// clearInterpolator：缺 probeType → 400 精确错误
	assertResponse(t, do(t, router, http.MethodPost, "/api/traversal/clearInterpolator", `{}`),
		400, `{"error":"probeType 必填（five-hole / seven-hole）","success":false}`+"\n")

	// calculateRealtime：未加载插值器 → 400
	w = do(t, router, http.MethodPost, "/api/traversal/calculateRealtime", `{"pressures":{"P1":1,"P2":2,"P3":3,"P4":4,"P5":5,"Patm":101325,"Tatm":288}}`)
	if w.Code != 400 || !strings.Contains(w.Body.String(), `"success":false`) {
		t.Fatalf("calculateRealtime 错误路径: %d %s", w.Code, w.Body.String())
	}

	// checkPreconditions：空 body → 200（map 响应）
	w = do(t, router, http.MethodPost, "/api/traversal/checkPreconditions", "")
	if w.Code != 200 || !strings.HasPrefix(w.Body.String(), "{") {
		t.Fatalf("checkPreconditions: %d %s", w.Code, w.Body.String())
	}

	// 未知 action → 404
	assertResponse(t, do(t, router, http.MethodGet, "/api/traversal/unknownAction", ""), 404, "")
}

// status 字节级回归（fresh manager 的确定响应）。
func TestServer_LegacyTraversal_Status(t *testing.T) {
	router := legacyRouter()
	w := do(t, router, http.MethodGet, "/api/traversal/status", "")
	if w.Code != 200 {
		t.Fatalf("status: %d", w.Code)
	}
	// fresh manager：idle 状态响应（录制式断言关键字段存在且为 idle）
	body := w.Body.String()
	if !strings.Contains(body, `"state":"idle"`) {
		t.Fatalf("fresh manager 应为 idle: %s", body)
	}
	// 错误 method → 405
	assertResponse(t, do(t, router, http.MethodPost, "/api/traversal/status", ""), 405, "")
}
