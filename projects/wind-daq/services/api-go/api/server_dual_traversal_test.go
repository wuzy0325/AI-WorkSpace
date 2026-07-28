package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/usecase"
)

// ---------------------------------------------------------------------------
// dual dispatcher 测试 fakes
// ---------------------------------------------------------------------------

// fakeDualManager 实现 traversalSharedManager：非生命周期方法计数/可注入结果；
// 生命周期方法（StartManaged 等）也计数——dual handler 不得调用它们（应走 registry）。
type fakeDualManager struct {
	mu sync.Mutex

	configRaw      json.RawMessage
	statusResponse map[string]any
	results        map[string]traversal.Status
	preconditions  map[string]any
	realtimeResult any

	getConfigRawCalls  int
	saveConfigRawCalls int
	statusCalls        int
	getResultCalls     int
	importPrbCalls     int
	importCsvCalls     int
	importMultiCalls   int
	import7PrbCalls    int
	import7CsvCalls    int
	clearInterpCalls   int
	calcRealtimeCalls  int
	preconditionCalls  int
	parseConfigCalls   int
	lastConfigRaw      json.RawMessage
	lastResultTaskID   string
	lastImportPath     string
	lastMultiPaths     []string
	lastClearProbeType string
	lastRealtimeProbe  string
	startManagedCalls  int
	resumeManagedCalls int
	runPointCalls      int
	pauseCalls         int
	resumeCalls        int
	stopCalls          int
}

func (m *fakeDualManager) ParseConfig(raw json.RawMessage) (traversal.Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.parseConfigCalls++
	var cfg traversal.Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return traversal.Config{}, err
	}
	return cfg, nil
}
func (m *fakeDualManager) StartManaged(traversal.Config, usecase.ManagedSessionOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startManagedCalls++
	return nil
}
func (m *fakeDualManager) ResumeManaged(cp traversal.Checkpoint, _ usecase.ManagedSessionOptions) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resumeManagedCalls++
	return cp.TaskID, nil
}
func (m *fakeDualManager) RunCurrentPoint() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runPointCalls++
	return nil
}
func (m *fakeDualManager) Pause() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pauseCalls++
	return nil
}
func (m *fakeDualManager) Resume() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resumeCalls++
	return nil
}
func (m *fakeDualManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopCalls++
	return nil
}
func (m *fakeDualManager) Done() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}
func (m *fakeDualManager) Status() traversal.Status { return traversal.Status{} }
func (m *fakeDualManager) GetResult(taskID string) (traversal.Status, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getResultCalls++
	m.lastResultTaskID = taskID
	st, ok := m.results[taskID]
	return st, ok
}
func (m *fakeDualManager) SaveConfigRaw(config json.RawMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saveConfigRawCalls++
	m.lastConfigRaw = config
	m.configRaw = config
}
func (m *fakeDualManager) GetConfigRaw() json.RawMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getConfigRawCalls++
	return m.configRaw
}
func (m *fakeDualManager) LoadCheckpoint() (*traversal.Checkpoint, error) { return nil, nil }
func (m *fakeDualManager) ClearCheckpoint()                               {}
func (m *fakeDualManager) BuildStatusResponse() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusCalls++
	return m.statusResponse
}
func (m *fakeDualManager) CheckPreconditions(*traversal.Config) map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.preconditionCalls++
	return m.preconditions
}
func (m *fakeDualManager) ImportPRB(filePath string) (*usecase.PrbImportResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.importPrbCalls++
	m.lastImportPath = filePath
	return &usecase.PrbImportResult{}, nil
}
func (m *fakeDualManager) ImportCalibrationCSV(filePath string) (*usecase.CalibrationCsvImportResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.importCsvCalls++
	m.lastImportPath = filePath
	return &usecase.CalibrationCsvImportResult{}, nil
}
func (m *fakeDualManager) ImportMultiPRB(filePaths []string, _ []float64, _ string) (*usecase.MultiPrbImportResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.importMultiCalls++
	m.lastMultiPaths = filePaths
	return &usecase.MultiPrbImportResult{}, nil
}
func (m *fakeDualManager) ImportSevenHolePRB(string, []string) (*usecase.SevenHoleImportResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.import7PrbCalls++
	return &usecase.SevenHoleImportResult{}, nil
}
func (m *fakeDualManager) ImportSevenHoleCalibrationCSV(string, []string) (*usecase.SevenHoleImportResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.import7CsvCalls++
	return &usecase.SevenHoleImportResult{}, nil
}
func (m *fakeDualManager) ClearProbeInterpolator(probeType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clearInterpCalls++
	m.lastClearProbeType = probeType
	return nil
}
func (m *fakeDualManager) CalculateRealtimeForAPI(probeType string, _ usecase.ProbePressureInput) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calcRealtimeCalls++
	m.lastRealtimeProbe = probeType
	return m.realtimeResult, nil
}

// lifecycleCalls 返回 manager 生命周期方法调用总数（dual handler 必须保持 0）。
func (m *fakeDualManager) lifecycleCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startManagedCalls + m.resumeManagedCalls + m.runPointCalls + m.pauseCalls + m.resumeCalls + m.stopCalls
}

// fakeTraversalRegistry 实现 api.TraversalRegistry：记录 façade 调用并按 probe 路由。
type fakeTraversalRegistry struct {
	mu sync.Mutex

	managers       map[usecase.ProbeID]*fakeDualManager
	getOrCreateErr error
	startErr       error
	stopErr        error
	closeErr       error
	loadCp         *traversal.Checkpoint
	loadCpErr      error
	resumeCpErr    error
	clearCpErr     error

	startCalls   int
	runPointHits int
	pauseHits    int
	resumeHits   int
	stopCalls    int
	closeCalls   int
	loadCpCalls  int
	resumeCpHits int
	clearCpHits  int

	lastStartProbe  usecase.ProbeID
	lastStartRaw    json.RawMessage
	lastActionProbe map[string]usecase.ProbeID
	stopByProbe     map[usecase.ProbeID]int
	startedTaskID   string
	lastResumeTask  usecase.ProbeID
	lastResumeID    string
	lastClearProbe  usecase.ProbeID
	lastClearID     string
}

func newFakeTraversalRegistry() *fakeTraversalRegistry {
	return &fakeTraversalRegistry{
		managers:        make(map[usecase.ProbeID]*fakeDualManager),
		lastActionProbe: make(map[string]usecase.ProbeID),
		stopByProbe:     make(map[usecase.ProbeID]int),
		startedTaskID:   "server-task-1",
	}
}

func (r *fakeTraversalRegistry) managerFor(probeID usecase.ProbeID) *fakeDualManager {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.managers[probeID] == nil {
		r.managers[probeID] = &fakeDualManager{}
	}
	return r.managers[probeID]
}

func (r *fakeTraversalRegistry) GetOrCreate(probeID usecase.ProbeID) (usecase.ManagedTraversalManager, error) {
	r.mu.Lock()
	err := r.getOrCreateErr
	r.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return r.managerFor(probeID), nil
}

func (r *fakeTraversalRegistry) Start(_ context.Context, probeID usecase.ProbeID, raw json.RawMessage) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.startCalls++
	r.lastStartProbe = probeID
	r.lastStartRaw = raw
	if r.startErr != nil {
		return "", r.startErr
	}
	return r.startedTaskID, nil
}

func (r *fakeTraversalRegistry) RunPoint(_ context.Context, probeID usecase.ProbeID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runPointHits++
	r.lastActionProbe["runPoint"] = probeID
	return r.stopErr
}
func (r *fakeTraversalRegistry) Pause(_ context.Context, probeID usecase.ProbeID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pauseHits++
	r.lastActionProbe["pause"] = probeID
	return r.stopErr
}
func (r *fakeTraversalRegistry) Resume(_ context.Context, probeID usecase.ProbeID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resumeHits++
	r.lastActionProbe["resume"] = probeID
	return r.stopErr
}
func (r *fakeTraversalRegistry) Stop(_ context.Context, probeID usecase.ProbeID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopCalls++
	r.stopByProbe[probeID]++
	r.lastActionProbe["stop"] = probeID
	return r.stopErr
}
func (r *fakeTraversalRegistry) CloseProbe(_ context.Context, probeID usecase.ProbeID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closeCalls++
	r.lastActionProbe["close"] = probeID
	return r.closeErr
}
func (r *fakeTraversalRegistry) LoadCheckpoint(_ context.Context, probeID usecase.ProbeID) (*traversal.Checkpoint, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loadCpCalls++
	r.lastActionProbe["loadCheckpoint"] = probeID
	return r.loadCp, r.loadCpErr
}
func (r *fakeTraversalRegistry) ResumeFromCheckpoint(_ context.Context, probeID usecase.ProbeID, taskID string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resumeCpHits++
	r.lastResumeTask = probeID
	r.lastResumeID = taskID
	if r.resumeCpErr != nil {
		return "", r.resumeCpErr
	}
	return taskID, nil
}
func (r *fakeTraversalRegistry) ClearCheckpoint(_ context.Context, probeID usecase.ProbeID, taskID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clearCpHits++
	r.lastClearProbe = probeID
	r.lastClearID = taskID
	return r.clearCpErr
}

func newDualRouter(reg TraversalRegistry) http.Handler {
	return NewRouter(Deps{TraversalRegistry: reg})
}

// ---------------------------------------------------------------------------
// Task 12: probe-scoped 路由解析与 dispatcher
// ---------------------------------------------------------------------------

func TestServer_DualTraversal_Routes(t *testing.T) {
	reg := newFakeTraversalRegistry()
	reg.managerFor(usecase.Probe1).statusResponse = map[string]any{"state": "running", "probe": "probe1"}
	reg.managerFor(usecase.Probe1).preconditions = map[string]any{"ok": true}
	reg.managerFor(usecase.Probe1).realtimeResult = map[string]any{"alpha": 1.5}
	reg.managerFor(usecase.Probe1).results = map[string]traversal.Status{
		"task-1": {TaskID: "task-1", State: traversal.StateStopped},
	}
	reg.managerFor(usecase.Probe1).configRaw = json.RawMessage(`{"probeType":"five-hole"}`)
	reg.loadCp = &traversal.Checkpoint{Version: 3, TaskID: "probe1-task-9", ProbeID: "probe1"}
	router := newDualRouter(reg)

	type expect struct {
		path       string
		method     string
		body       string
		wantStatus int
		assert     func(t *testing.T)
	}
	cases := []struct {
		name   string
		expect expect
	}{
		{"config GET", expect{"/api/traversal/probe1/config", http.MethodGet, "", 200, func(t *testing.T) {
			if reg.managerFor(usecase.Probe1).getConfigRawCalls != 1 {
				t.Fatal("config GET 应选择 probe1 manager")
			}
		}}},
		{"config POST", expect{"/api/traversal/probe1/config", http.MethodPost, `{"probeType":"seven-hole"}`, 200, func(t *testing.T) {
			m := reg.managerFor(usecase.Probe1)
			if m.saveConfigRawCalls != 1 || string(m.lastConfigRaw) != `{"probeType":"seven-hole"}` {
				t.Fatal("config POST 应写 probe1 manager")
			}
		}}},
		{"status", expect{"/api/traversal/probe1/status", http.MethodGet, "", 200, func(t *testing.T) {
			if reg.managerFor(usecase.Probe1).statusCalls != 1 {
				t.Fatal("status 应调 probe1 manager")
			}
		}}},
		{"result", expect{"/api/traversal/probe1/result?taskId=task-1", http.MethodGet, "", 200, func(t *testing.T) {
			m := reg.managerFor(usecase.Probe1)
			if m.getResultCalls != 1 || m.lastResultTaskID != "task-1" {
				t.Fatal("result 应按 taskId 查询 probe1 manager")
			}
		}}},
		{"importPrb", expect{"/api/traversal/probe1/importPrb", http.MethodPost, `{"filePath":"D:/a.prb"}`, 200, func(t *testing.T) {
			if reg.managerFor(usecase.Probe1).importPrbCalls != 1 {
				t.Fatal("importPrb 应调 probe1 manager")
			}
		}}},
		{"importCalibrationCsv", expect{"/api/traversal/probe1/importCalibrationCsv", http.MethodPost, `{"filePath":"D:/a.csv"}`, 200, func(t *testing.T) {
			if reg.managerFor(usecase.Probe1).importCsvCalls != 1 {
				t.Fatal("importCalibrationCsv 应调 probe1 manager")
			}
		}}},
		{"importMultiPrb", expect{"/api/traversal/probe1/importMultiPrb", http.MethodPost, `{"filePaths":["a.prb"],"machNumbers":[0.3]}`, 200, func(t *testing.T) {
			if reg.managerFor(usecase.Probe1).importMultiCalls != 1 {
				t.Fatal("importMultiPrb 应调 probe1 manager")
			}
		}}},
		{"importSevenHolePrb", expect{"/api/traversal/probe1/importSevenHolePrb", http.MethodPost, `{"innerFilePath":"7.prb","outerFilePaths":["1.prb"]}`, 200, func(t *testing.T) {
			if reg.managerFor(usecase.Probe1).import7PrbCalls != 1 {
				t.Fatal("importSevenHolePrb 应调 probe1 manager")
			}
		}}},
		{"importSevenHoleCalibrationCsv", expect{"/api/traversal/probe1/importSevenHoleCalibrationCsv", http.MethodPost, `{"innerFilePath":"7.csv","outerFilePaths":["1.csv"]}`, 200, func(t *testing.T) {
			if reg.managerFor(usecase.Probe1).import7CsvCalls != 1 {
				t.Fatal("importSevenHoleCalibrationCsv 应调 probe1 manager")
			}
		}}},
		{"clearInterpolator", expect{"/api/traversal/probe1/clearInterpolator", http.MethodPost, `{"probeType":"seven-hole"}`, 200, func(t *testing.T) {
			m := reg.managerFor(usecase.Probe1)
			if m.clearInterpCalls != 1 || m.lastClearProbeType != "seven-hole" {
				t.Fatal("clearInterpolator 应调 probe1 manager")
			}
		}}},
		{"calculateRealtime", expect{"/api/traversal/probe1/calculateRealtime", http.MethodPost, `{"probeType":"five-hole","pressures":{"P1":1}}`, 200, func(t *testing.T) {
			if reg.managerFor(usecase.Probe1).calcRealtimeCalls != 1 {
				t.Fatal("calculateRealtime 应调 probe1 manager")
			}
		}}},
		{"checkPreconditions", expect{"/api/traversal/probe1/checkPreconditions", http.MethodPost, `{"config":{"taskId":"t"}}`, 200, func(t *testing.T) {
			if reg.managerFor(usecase.Probe1).preconditionCalls != 1 {
				t.Fatal("checkPreconditions 应调 probe1 manager")
			}
		}}},
		{"start", expect{"/api/traversal/probe1/start", http.MethodPost, `{"taskId":"client-x"}`, 200, func(t *testing.T) {
			if reg.startCalls != 1 || reg.lastStartProbe != usecase.Probe1 {
				t.Fatal("start 必须只调 registry.Start façade")
			}
		}}},
		{"runPoint", expect{"/api/traversal/probe1/runPoint", http.MethodPost, "", 200, func(t *testing.T) {
			if reg.runPointHits != 1 {
				t.Fatal("runPoint 必须只调 registry façade")
			}
		}}},
		{"pause", expect{"/api/traversal/probe1/pause", http.MethodPost, "", 200, func(t *testing.T) {
			if reg.pauseHits != 1 {
				t.Fatal("pause 必须只调 registry façade")
			}
		}}},
		{"resume", expect{"/api/traversal/probe1/resume", http.MethodPost, "", 200, func(t *testing.T) {
			if reg.resumeHits != 1 {
				t.Fatal("resume 必须只调 registry façade")
			}
		}}},
		{"stop", expect{"/api/traversal/probe1/stop", http.MethodPost, "", 200, func(t *testing.T) {
			if reg.stopCalls != 1 {
				t.Fatal("stop 必须只调 registry façade")
			}
		}}},
		{"loadCheckpoint", expect{"/api/traversal/probe1/loadCheckpoint", http.MethodGet, "", 200, func(t *testing.T) {
			if reg.loadCpCalls != 1 {
				t.Fatal("loadCheckpoint 必须只调 registry façade")
			}
		}}},
		{"resumeFromCheckpoint", expect{"/api/traversal/probe1/resumeFromCheckpoint", http.MethodPost, `{"taskId":"probe1-task-9"}`, 200, func(t *testing.T) {
			if reg.resumeCpHits != 1 || reg.lastResumeID != "probe1-task-9" {
				t.Fatal("resumeFromCheckpoint 必须只调 registry façade 且只传 taskId")
			}
		}}},
		{"clearCheckpoint", expect{"/api/traversal/probe1/clearCheckpoint", http.MethodPost, `{"taskId":"probe1-task-9"}`, 200, func(t *testing.T) {
			if reg.clearCpHits != 1 || reg.lastClearID != "probe1-task-9" {
				t.Fatal("clearCheckpoint 必须只调 registry façade 且只传 taskId")
			}
		}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req *http.Request
			if tc.expect.body != "" {
				req = httptest.NewRequest(tc.expect.method, tc.expect.path, strings.NewReader(tc.expect.body))
			} else {
				req = httptest.NewRequest(tc.expect.method, tc.expect.path, nil)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tc.expect.wantStatus {
				t.Fatalf("%s %s → %d, want %d (body=%s)", tc.expect.method, tc.expect.path, w.Code, tc.expect.wantStatus, w.Body.String())
			}
			tc.expect.assert(t)
		})
	}
	// 生命周期 action 全程未触碰 manager 生命周期方法（仅 registry façade）
	for _, probeID := range []usecase.ProbeID{usecase.Probe1, usecase.Probe2} {
		if calls := reg.managerFor(probeID).lifecycleCalls(); calls != 0 {
			t.Fatalf("%s manager 生命周期方法被 handler 直接调用 %d 次（必须仅经 registry façade）", probeID, calls)
		}
	}
}

func TestServer_DualTraversal_RouteParsing(t *testing.T) {
	reg := newFakeTraversalRegistry()
	router := newDualRouter(reg)

	cases := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"未知 probe", http.MethodGet, "/api/traversal/probe3/status", "", 400},
		{"大小写 probe", http.MethodGet, "/api/traversal/PROBE1/status", "", 400},
		{"缺失 action", http.MethodGet, "/api/traversal/probe1/", "", 404},
		{"多余路径段", http.MethodPost, "/api/traversal/probe1/start/extra", `{}`, 404},
		{"未知 action", http.MethodGet, "/api/traversal/probe1/fly", "", 404},
		{"generateGrid 无 probe 路由保留", http.MethodPost, "/api/traversal/generateGrid", `{}`, 400}, // legacy 路径（manager nil → 400，非 404）
		{"generateGrid 无 probe 形式", http.MethodPost, "/api/traversal/probe1/generateGrid", `{}`, 404},
		{"status 错误 method", http.MethodPost, "/api/traversal/probe1/status", "", 405},
		{"start 错误 method", http.MethodGet, "/api/traversal/probe1/start", "", 405},
		{"close 错误 method", http.MethodGet, "/api/traversal/probe1/close", "", 405},
		{"三段路径", http.MethodGet, "/api/traversal/a/b/c", "", 404},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req *http.Request
			if tc.body != "" {
				req = httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			} else {
				req = httptest.NewRequest(tc.method, tc.path, nil)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tc.wantStatus {
				t.Fatalf("%s %s → %d, want %d (body=%s)", tc.method, tc.path, w.Code, tc.wantStatus, w.Body.String())
			}
		})
	}
}

func TestServer_DualTraversal_CloseStateContract(t *testing.T) {
	reg := newFakeTraversalRegistry()
	router := newDualRouter(reg)

	// 活动状态判定由 registry 在 admission gate 内完成，API 仅映射错误。
	reg.closeErr = usecase.ErrAlreadyRunning
	w := postJSON(t, router, "/api/traversal/probe1/close", "")
	if w.Code != http.StatusConflict {
		t.Fatalf("活动状态 close 应返回 409, got %d (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "already_running") {
		t.Fatalf("冲突错误应含 already_running: %s", w.Body.String())
	}
	if reg.closeCalls != 1 {
		t.Fatal("API 必须调用 CloseProbe，由 registry 原子判定活动状态")
	}
	// 终态（terminal / completion_failed）：幂等重试入口，调用 CloseProbe
	reg.closeErr = nil
	w = postJSON(t, router, "/api/traversal/probe1/close", "")
	if w.Code != http.StatusOK {
		t.Fatalf("终态 close 应返回 200, got %d (%s)", w.Code, w.Body.String())
	}
	if reg.closeCalls != 2 {
		t.Fatal("终态 close 应调用 CloseProbe（completion_failed 时内部幂等重试）")
	}
}

func TestServer_DualTraversal_ConcurrentProbesIsolated(t *testing.T) {
	reg := newFakeTraversalRegistry()
	reg.managerFor(usecase.Probe1).statusResponse = map[string]any{"probe": "probe1"}
	reg.managerFor(usecase.Probe2).statusResponse = map[string]any{"probe": "probe2"}
	router := newDualRouter(reg)

	// 两路并发 start/status/pause/resume/stop/result：barrier 编排，断言按 probe 路由互不串
	var ready sync.WaitGroup
	ready.Add(2)
	var wg sync.WaitGroup
	wg.Add(2)
	errs := make(chan string, 2)
	go func() {
		defer wg.Done()
		ready.Done()
		w := postJSON(t, router, "/api/traversal/probe1/start", `{"taskId":"c"}`)
		if w.Code != 200 {
			errs <- "probe1 start: " + w.Body.String()
			return
		}
		w = postJSON(t, router, "/api/traversal/probe1/pause", "")
		if w.Code != 200 {
			errs <- "probe1 pause: " + w.Body.String()
			return
		}
		w = postJSON(t, router, "/api/traversal/probe1/resume", "")
		if w.Code != 200 {
			errs <- "probe1 resume: " + w.Body.String()
			return
		}
		w = postJSON(t, router, "/api/traversal/probe1/stop", "")
		if w.Code != 200 {
			errs <- "probe1 stop: " + w.Body.String()
			return
		}
		req := httptest.NewRequest(http.MethodGet, "/api/traversal/probe1/status", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if !strings.Contains(rec.Body.String(), `"probe":"probe1"`) {
			errs <- "probe1 status 串状态: " + rec.Body.String()
		}
	}()
	go func() {
		defer wg.Done()
		ready.Done()
		w := postJSON(t, router, "/api/traversal/probe2/start", `{"taskId":"c"}`)
		if w.Code != 200 {
			errs <- "probe2 start: " + w.Body.String()
			return
		}
		w = postJSON(t, router, "/api/traversal/probe2/stop", "")
		if w.Code != 200 {
			errs <- "probe2 stop: " + w.Body.String()
			return
		}
		req := httptest.NewRequest(http.MethodGet, "/api/traversal/probe2/status", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if !strings.Contains(rec.Body.String(), `"probe":"probe2"`) {
			errs <- "probe2 status 串状态: " + rec.Body.String()
		}
	}()
	ready.Wait()
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	// 每个 façade 都按各自 probe 记录（两路 stop 各自到达）
	if reg.stopByProbe[usecase.Probe1] != 1 || reg.stopByProbe[usecase.Probe2] != 1 {
		t.Fatalf("stop 应按 probe 各调用一次: %v", reg.stopByProbe)
	}
	if reg.startCalls != 2 {
		t.Fatalf("两路 start 各一次, got %d", reg.startCalls)
	}
	if reg.managerFor(usecase.Probe1).statusCalls != 1 || reg.managerFor(usecase.Probe2).statusCalls != 1 {
		t.Fatal("status 应按 probe 各自调用一次")
	}
}

func TestServer_DualTraversal_StartPassesRawConfigAndReturnsServerTaskID(t *testing.T) {
	reg := newFakeTraversalRegistry()
	reg.startedTaskID = "probe1-task-42"
	router := newDualRouter(reg)

	w := postJSON(t, router, "/api/traversal/probe2/start", `{"taskId":"client-x","deviceId":"dev-1"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
	if reg.lastStartProbe != usecase.Probe2 {
		t.Fatalf("start 应路由到 probe2, got %v", reg.lastStartProbe)
	}
	if !strings.Contains(string(reg.lastStartRaw), `"deviceId":"dev-1"`) {
		t.Fatal("原始配置应原样传给 registry")
	}
	if !strings.Contains(w.Body.String(), `"taskId":"probe1-task-42"`) {
		t.Fatalf("响应应返回服务端权威 taskId: %s", w.Body.String())
	}
}
