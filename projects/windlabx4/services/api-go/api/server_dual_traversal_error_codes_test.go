package api

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"windlabx4/services/api-go/internal/usecase"
)

// Task 13：错误码白盒测试（spec FR4 / Task 12 映射表全覆盖）。
//
// 每个 sentinel 从对应 façade 注入，断言 HTTP 状态码与响应体中的错误码字符串：
// unknown probe → 400 invalid_probe_id；manager 创建失败 → 503 manager_creation_failed；
// 资源冲突 → 409 resource_conflict；同 probe 已运行 → 409 already_running；
// registry closing → 503 registry_closing；registry_transitioning → 503。

func TestServer_DualTraversal_ErrorCodes(t *testing.T) {
	cases := []struct {
		name       string
		inject     func(reg *fakeTraversalRegistry)
		method     string
		path       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{"invalid_probe_id", func(reg *fakeTraversalRegistry) {},
			http.MethodGet, "/api/traversal/probe9/status", "", 400, "invalid_probe_id"},
		{"manager_creation_failed", func(reg *fakeTraversalRegistry) {
			reg.getOrCreateErr = errors.New("factory boom")
		}, http.MethodGet, "/api/traversal/probe1/status", "", 503, "manager_creation_failed"},
		{"resource_conflict", func(reg *fakeTraversalRegistry) {
			reg.startErr = usecase.ErrResourceConflict
		}, http.MethodPost, "/api/traversal/probe1/start", `{}`, 409, "resource_conflict"},
		{"already_running", func(reg *fakeTraversalRegistry) {
			reg.startErr = usecase.ErrAlreadyRunning
		}, http.MethodPost, "/api/traversal/probe1/start", `{}`, 409, "already_running"},
		{"registry_closing", func(reg *fakeTraversalRegistry) {
			reg.startErr = usecase.ErrRegistryClosing
		}, http.MethodPost, "/api/traversal/probe1/start", `{}`, 503, "registry_closing"},
		{"registry_transitioning", func(reg *fakeTraversalRegistry) {
			reg.startErr = usecase.ErrRegistryTransitioning
		}, http.MethodPost, "/api/traversal/probe1/start", `{}`, 503, "registry_transitioning"},
		{"probe_closing", func(reg *fakeTraversalRegistry) {
			reg.getOrCreateErr = usecase.ErrProbeClosing
		}, http.MethodGet, "/api/traversal/probe1/status", "", 409, "probe_closing"},
		{"stop façade 错误传播", func(reg *fakeTraversalRegistry) {
			reg.stopErr = usecase.ErrResourceConflict
		}, http.MethodPost, "/api/traversal/probe1/stop", "", 409, "resource_conflict"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := newFakeTraversalRegistry()
			tc.inject(reg)
			router := newDualRouter(reg)
			w := do(t, router, tc.method, tc.path, tc.body)
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body=%s)", w.Code, tc.wantStatus, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tc.wantCode) {
				t.Fatalf("响应应包含错误码 %q: %s", tc.wantCode, w.Body.String())
			}
		})
	}
}
