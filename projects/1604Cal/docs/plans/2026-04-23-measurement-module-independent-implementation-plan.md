# Measurement Module Independent Workflow Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Rebuild `measurement` into an independent, full-featured measurement workflow module without reusing `calibration` business orchestration.

**Architecture:** Keep `measurement` and `calibration` fully isolated at the business layer. Implement measurement-owned config, points, workflow state, alarm flow, progress events, and UI. Share only low-level device/session access and pure utility logic where needed.

**Tech Stack:** Go HTTP API, Go application services, Vue 3, Pinia, Vitest, existing device/session infrastructure.

---

### Task 1: Lock the module boundary with tests and docs

**Files:**
- Modify: `AGENTS.md:24-31`
- Modify: `docs/plans/2026-04-23-measurement-module-business-review.md`
- Create: `docs/plans/2026-04-23-measurement-module-independent-design.md`
- Test: none

**Step 1: Update the review doc wording**

Add a short addendum under P0-1 stating that the previously mentioned “reuse calibration orchestration” option is no longer valid because `measurement` and `calibration` must remain business-isolated.

**Step 2: Save the confirmed design doc**

Document the final boundary, state model, API plan, and phased delivery strategy.

**Step 3: Verify docs are readable**

Run: manual review of changed markdown files  
Expected: no wording suggests `measurement` may call `calibrationService`

**Step 4: Commit**

```bash
git add AGENTS.md docs/plans/2026-04-23-measurement-module-business-review.md docs/plans/2026-04-23-measurement-module-independent-design.md
git commit -m "docs: define independent measurement workflow boundary"
```

### Task 2: Add measurement-owned config and point generation on the backend

**Files:**
- Modify: `internal/application/measurement/service.go`
- Create: `internal/application/measurement/points.go`
- Modify: `internal/api/http/measurement_handler.go`
- Modify: `internal/api/http/router.go`
- Test: `internal/application/measurement/service_test.go`
- Test: `internal/api/http/measurement_handler_test.go`

**Step 1: Write the failing service test**

```go
func TestGeneratePressurePointsUsesMeasurementConfig(t *testing.T) {
	svc, _ := setupMeasurementService()
	svc.SetConfig(measurement.Config{
		MinPressure: 0,
		MaxPressure: 100,
		PointCount: 5,
		Precision: 2,
		PressureMode: "roundTrip",
	})

	points, err := svc.GeneratePressurePoints()
	if err != nil {
		t.Fatalf("GeneratePressurePoints: %v", err)
	}

	if len(points) != 9 {
		t.Fatalf("expected 9 points, got %d", len(points))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/application/measurement -run TestGeneratePressurePointsUsesMeasurementConfig`  
Expected: FAIL because `SetConfig` / `GeneratePressurePoints` do not exist yet

**Step 3: Write minimal implementation**

Add measurement-owned structs:

```go
type Config struct {
	Channels       []int   `json:"channels"`
	MinPressure    float64 `json:"minPressure"`
	MaxPressure    float64 `json:"maxPressure"`
	PointCount     int     `json:"pointCount"`
	Precision      int     `json:"precision"`
	AverageCount   int     `json:"averageCount"`
	PrecisionLevel float64 `json:"precisionLevel"`
	StableWaitMs   int     `json:"stableWaitMs"`
	PressureMode   string  `json:"pressureMode"`
	ControlMode    string  `json:"controlMode"`
}

type Point struct {
	ID             string    `json:"id"`
	Index          int       `json:"index"`
	TargetPressure float64   `json:"targetPressure"`
	Direction      string    `json:"direction"`
	Status         string    `json:"status"`
	CollectedData  []float64 `json:"collectedData,omitempty"`
	ActualPressure float64   `json:"actualPressure,omitempty"`
	CollectTime    string    `json:"collectTime,omitempty"`
	ErrorMessage   string    `json:"errorMessage,omitempty"`
}
```

Add handlers for measurement config and point generation under `/api/v1/measurement/...` routes.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/application/measurement ./internal/api/http -run "TestGeneratePressurePointsUsesMeasurementConfig|TestMeasurementGeneratePointsEndpoint"`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/application/measurement/service.go internal/application/measurement/points.go internal/api/http/measurement_handler.go internal/api/http/router.go internal/application/measurement/service_test.go internal/api/http/measurement_handler_test.go
git commit -m "feat: add measurement config and point generation"
```

### Task 3: Replace continuous-only start with measurement-owned workflow session

**Files:**
- Modify: `internal/application/measurement/service.go`
- Create: `internal/application/measurement/workflow.go`
- Modify: `internal/api/http/measurement_handler.go`
- Test: `internal/application/measurement/service_test.go`
- Test: `internal/api/http/measurement_handler_test.go`

**Step 1: Write the failing workflow test**

```go
func TestStartMeasurementWorkflowUsesGeneratedPoints(t *testing.T) {
	svc, _ := setupMeasurementService()
	svc.SetConfig(measurement.Config{
		Channels: []int{1, 2},
		MinPressure: 0,
		MaxPressure: 20,
		PointCount: 3,
		AverageCount: 1,
		StableWaitMs: 100,
		ControlMode: "manual",
	})
	_, _ = svc.GeneratePressurePoints()

	err := svc.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if svc.State() != measurement.StateReady {
		t.Fatalf("expected ready, got %s", svc.State())
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/application/measurement -run TestStartMeasurementWorkflowUsesGeneratedPoints`  
Expected: FAIL because `Start` still requires channel arguments and jumps into continuous collection

**Step 3: Write minimal implementation**

Split the old behavior:

- `StartRealtimeSampling(ctx, channels []int)` for raw continuous sampling
- `Start(ctx)` for measurement-owned workflow session

`Start(ctx)` should:

```go
func (s *Service) Start(ctx context.Context) error {
	if err := s.ValidateStartPrerequisites(ctx); err != nil {
		return err
	}
	if len(s.points) == 0 {
		return fmt.Errorf("measurement points not generated")
	}
	s.session = newMeasurementSession(s.config, s.points)
	return s.enterReadyState()
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/application/measurement ./internal/api/http -run "TestStartMeasurementWorkflowUsesGeneratedPoints|TestMeasurementStartRequiresGeneratedPoints"`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/application/measurement/service.go internal/application/measurement/workflow.go internal/api/http/measurement_handler.go internal/application/measurement/service_test.go internal/api/http/measurement_handler_test.go
git commit -m "feat: create independent measurement workflow session"
```

### Task 4: Implement auto/manual point collection in measurement service

**Files:**
- Modify: `internal/application/measurement/workflow.go`
- Create: `internal/application/measurement/collector.go`
- Test: `internal/application/measurement/service_test.go`

**Step 1: Write the failing auto-mode test**

```go
func TestRunAutoCollectionAdvancesMeasurementPoints(t *testing.T) {
	svc, _ := setupMeasurementService()
	svc.SetDevices("m1", "p1")
	svc.SetConfig(measurement.Config{
		Channels: []int{1},
		MinPressure: 0,
		MaxPressure: 10,
		PointCount: 2,
		AverageCount: 1,
		StableWaitMs: 10,
		ControlMode: "auto",
	})
	_, _ = svc.GeneratePressurePoints()

	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := svc.RunAutoCollection(context.Background()); err != nil {
		t.Fatalf("RunAutoCollection: %v", err)
	}

	points := svc.GetPoints()
	if points[0].Status != "completed" {
		t.Fatalf("expected first point completed, got %s", points[0].Status)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/application/measurement -run TestRunAutoCollectionAdvancesMeasurementPoints`  
Expected: FAIL because auto collection does not exist

**Step 3: Write minimal implementation**

Implement measurement-owned methods:

- `RunAutoCollection(ctx)`
- `ManualPressurize(ctx, pointIndex int)`
- `ManualCollect(ctx, pointIndex int)`
- `updatePointStatus(pointIndex, status)`

Use only shared low-level device/session access, not `calibrationService`.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/application/measurement`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/application/measurement/workflow.go internal/application/measurement/collector.go internal/application/measurement/service_test.go
git commit -m "feat: add measurement auto and manual collection flow"
```

### Task 5: Implement measurement-owned alarm configuration and resolution

**Files:**
- Create: `internal/application/measurement/alarm.go`
- Modify: `internal/application/measurement/service.go`
- Modify: `internal/api/http/measurement_handler.go`
- Test: `internal/application/measurement/service_test.go`
- Test: `internal/api/http/measurement_handler_test.go`

**Step 1: Write the failing alarm test**

```go
func TestMeasurementAlarmBlocksUntilResolved(t *testing.T) {
	svc, _ := setupMeasurementService()
	svc.SetAlarmConfig(measurement.AlarmConfig{
		Enabled: true,
		EnabledChannels: []int{1},
		ConfirmOnAlarm: true,
		Threshold: 0.01,
	})

	decisionCh := make(chan error, 1)
	go func() {
		decisionCh <- svc.ResolveAlarm("continue")
	}()

	if err := <-decisionCh; err != nil {
		t.Fatalf("ResolveAlarm: %v", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/application/measurement -run TestMeasurementAlarmBlocksUntilResolved`  
Expected: FAIL because measurement alarm flow does not exist

**Step 3: Write minimal implementation**

Add measurement-owned alarm config and resolution methods:

```go
type AlarmConfig struct {
	Enabled         bool    `json:"enabled"`
	EnabledChannels []int   `json:"enabledChannels"`
	ConfirmOnAlarm  bool    `json:"confirmOnAlarm"`
	SoundEnabled    bool    `json:"soundEnabled"`
	Threshold       float64 `json:"threshold"`
}
```

Add routes:

- `GET /api/v1/config/measurement-alarm`
- `POST /api/v1/config/measurement-alarm`
- `POST /api/v1/measurement/alarm/resolve`

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/application/measurement ./internal/api/http -run "TestMeasurementAlarmBlocksUntilResolved|TestMeasurementAlarmConfigEndpoints"`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/application/measurement/alarm.go internal/application/measurement/service.go internal/api/http/measurement_handler.go internal/application/measurement/service_test.go internal/api/http/measurement_handler_test.go
git commit -m "feat: add independent measurement alarm flow"
```

### Task 6: Rebuild measurement frontend around backend-owned points and session state

**Files:**
- Modify: `web/src/api/measurement.ts`
- Modify: `web/src/stores/measurement/index.ts`
- Modify: `web/src/views/MeasurementView.vue`
- Modify: `web/src/components/measurement/MeasurementParamsPanel.vue`
- Modify: `web/src/components/measurement/MeasurementControl.vue`
- Modify: `web/src/components/measurement/MeasurementDataView.vue`
- Modify: `web/src/components/measurement/MeasurementSidebar.vue`
- Test: `web/src/views/__tests__/MeasurementView.test.ts`
- Test: `web/src/components/measurement/__tests__/MeasurementControl.test.ts`
- Test: `web/src/components/measurement/__tests__/MeasurementDataView.test.ts`

**Step 1: Write the failing store test**

```ts
it('loads backend-generated measurement points into the store', async () => {
  vi.mocked(measurementApi.fetchMeasurementPoints).mockResolvedValue([
    { id: 'p1', index: 1, targetPressure: 10, status: 'pending', direction: 'forward' }
  ])

  const store = useMeasurementStore()
  await store.loadPoints()

  expect(store.points).toHaveLength(1)
  expect(store.points[0].targetPressure).toBe(10)
})
```

**Step 2: Run test to verify it fails**

Run: `npm test -- src/stores/measurement/__tests__/index.test.ts`  
Expected: FAIL because `points` / `loadPoints` do not exist

**Step 3: Write minimal implementation**

Extend the store with:

```ts
const config = ref<MeasurementConfig | null>(null)
const alarmConfig = ref<MeasurementAlarmConfig | null>(null)
const points = ref<MeasurementPoint[]>([])
const progress = ref<MeasurementProgress | null>(null)
const session = ref<MeasurementSession | null>(null)
const rawRows = ref<CollectedRow[]>([])
```

Update the view so that:

- params panel drives backend config
- point generation comes from backend
- control bar shows backend point progress
- table renders backend point rows
- raw rows remain available as auxiliary data

**Step 4: Run tests to verify they pass**

Run: `npm test -- src/stores/measurement/__tests__/index.test.ts src/views/__tests__/MeasurementView.test.ts src/components/measurement/__tests__/MeasurementControl.test.ts src/components/measurement/__tests__/MeasurementDataView.test.ts`  
Expected: PASS

**Step 5: Commit**

```bash
git add web/src/api/measurement.ts web/src/stores/measurement/index.ts web/src/views/MeasurementView.vue web/src/components/measurement/MeasurementParamsPanel.vue web/src/components/measurement/MeasurementControl.vue web/src/components/measurement/MeasurementDataView.vue web/src/components/measurement/MeasurementSidebar.vue web/src/stores/measurement/__tests__/index.test.ts web/src/views/__tests__/MeasurementView.test.ts web/src/components/measurement/__tests__/MeasurementControl.test.ts web/src/components/measurement/__tests__/MeasurementDataView.test.ts
git commit -m "feat: rebuild measurement UI around independent workflow"
```

### Task 7: Add measurement-owned export and final verification

**Files:**
- Modify: `internal/report/report_service.go`
- Modify: `internal/api/http/measurement_handler.go`
- Modify: `web/src/views/MeasurementView.vue`
- Test: `internal/api/http/measurement_handler_test.go`
- Test: `web/src/views/__tests__/MeasurementView.test.ts`

**Step 1: Write the failing export test**

```go
func TestMeasurementReportExportEndpoint(t *testing.T) {
	router, _ := newSessionRouterWithMeasureDriverAndFakeDriver(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/measurement/report/export", bytes.NewReader([]byte(`{"path":"C:/tmp/measurement.xlsx"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api/http -run TestMeasurementReportExportEndpoint`  
Expected: FAIL because endpoint does not exist

**Step 3: Write minimal implementation**

Add a measurement-owned export handler that packages measurement session data and calls report/export utilities without routing through calibration page logic.

**Step 4: Run full verification**

Run: `go test ./internal/...`  
Expected: PASS

Run: `npm run typecheck`  
Expected: PASS

Run: `npm test -- src/stores/measurement/__tests__/index.test.ts src/views/__tests__/MeasurementView.test.ts src/components/measurement/__tests__/MeasurementControl.test.ts src/components/measurement/__tests__/MeasurementDataView.test.ts`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/report/report_service.go internal/api/http/measurement_handler.go internal/api/http/measurement_handler_test.go web/src/views/MeasurementView.vue web/src/views/__tests__/MeasurementView.test.ts
git commit -m "feat: complete measurement export and verification"
```

---

Plan complete and saved to `docs/plans/2026-04-23-measurement-module-independent-implementation-plan.md`.

Two execution options:

**1. Subagent-Driven (this session)** - I dispatch fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** - Open new session with executing-plans, batch execution with checkpoints
