# PACE1000 Integration Implementation Plan

> **For OpenCode:** REQUIRED SUB-SKILLS: use `executing-plans`, `test-driven-development`, `incremental-implementation`, and `verification-before-completion` to implement this plan task-by-task.

**Goal:** Add PACE1000 as a serial-only, single-channel atmospheric-pressure acquisition device in WindLabX4.

**Architecture:** Put the command parser and serial driver in `shared/device-sdk/go`, then expose it to WindLabX4 through a thin hardware adapter. Reuse the existing `ports.Device`, profile persistence, acquisition hub, generic CSV long format, and device-management UI; do not add a PACE-specific API or Wails binding.

**Tech Stack:** Go 1.25, `go.bug.st/serial`, shared device SDK, WindLabX4 hexagonal backend, Vue 3, TypeScript, Vitest, Wails v3.

**Source spec:** `projects/windlabx4/docs/specs/spec-pace1000-integration.md`

---

## Impact Summary

GitNexus pre-change analysis identified these risks:

| Symbol | Risk | Direct dependents | Plan response |
|---|---|---:|---|
| `serialport.DefaultConfig` | LOW | 0 | Reuse unchanged for `9600-8-N-1` |
| `serialport.Port.Read` / `Close` | LOW | 0 indexed | Add cancellation regression test before changing lock behavior |
| `NewDefaultProfile` | **CRITICAL** | 24 | Add one isolated `DevicePACE1000` switch branch and run all config/startup/integration tests |
| `IsAtmosphericChannel` | LOW | 3 | Add PACE1000 index 0 case with focused calibration tests |
| `createBlankProfile` | LOW | 3 | Add a serial-only branch and focused UI tests where practical |

The CRITICAL result is caused by `NewDefaultProfile` being a shared startup/configuration function, not by complex PACE behavior. No implementation should proceed past Task 4 if the complete existing profile test suite regresses.

## Architecture Decisions

1. **Canonical type:** `PACE1000` in shared SDK, backend, persisted JSON, and TypeScript.
2. **Transport:** Serial only. User selects `serialPort`; baud is fixed at 9600 and hidden or read-only.
3. **Data shape:** Exactly one channel: index 0, name `大气压力`, unit `Pa`, precision 1.
4. **Command:** Exact bytes `:sens?\r` (`3A 73 65 6E 73 3F 0D`), no LF.
5. **Conversion:** Parse one confirmed device value and publish `raw * 1000` Pa.
6. **Cadence:** A transaction cannot run faster than the confirmed 500 ms post-write wait; default profile is 2 Hz and missed periods are not queued.
7. **Cancellation:** `Disconnect` closes the serial handle independently of a blocked read. This requires fixing the existing serial wrapper's lock scope first.
8. **Failure policy:** Invalid/empty/timeout responses publish nothing. Three consecutive failures end acquisition and notify `DeviceManager` through `ErrorNotifiable`.
9. **No new API surface:** PACE1000 implements only existing device interfaces.
10. **Protocol evidence:** LabVIEW `%s%f` defines the parser contract; the real response terminator remains a HIL verification item.

## Dependency Graph

```text
Task 1: serial Close cancellation
  |
  +--> Task 2: `%s%f` response parser contract
  |      |
  |      +--> Task 3: shared PACE1000 driver
  |               |
Task 4: backend type/profile ----+--> Task 5: WindLabX4 adapter + factories
                                         |
                                         +--> Task 6: frontend serial-only integration
                                         |
                                         +--> Task 7: storage/calibration integration
                                                        |
                                                        +--> Task 8: full verification + HIL
```

Tasks 1 and 4 can be implemented independently after plan approval. Task 2 is blocked only by the missing RX sample. Tasks 6 and 7 may run in parallel after Task 5.

---

### Task 1: Make Serial Close Cancel a Blocked Read

**Files:**
- Modify: `shared/device-sdk/go/serialport/port.go`
- Create: `shared/device-sdk/go/serialport/port_test.go`

**Step 1: Write the failing regression test**

Create a fake `serial.Port` whose `Read` blocks until `Close` is called. Construct `serialport.Port` inside a same-package test, start `Read` in a goroutine, call `Close`, and assert both calls return within 200 ms.

Also assert:

- a second `Close` is harmless;
- a read after close returns `serial port <name> not open`;
- race-enabled execution has no data race.

**Step 2: Verify the test fails**

Run:

```powershell
cd shared/device-sdk/go
go test ./serialport -run TestPortCloseUnblocksRead -count=1 -timeout 2s
```

Expected before the fix: FAIL or timeout because `Read` holds `Port.mu` while blocked and `Close` cannot acquire it.

**Step 3: Implement the minimal lock-scope fix**

Under `Port.mu`, copy the current `serial.Port` reference to a local variable and release the lock before calling the underlying blocking `Read` or `Write`. In `Close`, atomically take ownership by setting `p.port=nil` under lock, then call the underlying `Close` outside the lock.

Do not add a second wrapper or new dependency.

**Step 4: Verify**

```powershell
cd shared/device-sdk/go
go test ./serialport -count=1
go test -race ./serialport -count=1
```

Expected: PASS; `Close` unblocks the fake read without deadlock.

**Acceptance criteria:**
- [ ] A blocking serial read is released by `Port.Close()`.
- [ ] Existing `DefaultConfig` remains `9600-8-N-1` with 1 s read timeout.
- [ ] Double close and post-close operations behave deterministically.

**Dependencies:** None.

**Estimated scope:** Small, 2 files.

---

### Task 2: Freeze and Implement the PACE1000 Response Parser

**Protocol gate:** The LabVIEW image confirms the response scan format `%s%f`: discard the first string field and parse the second floating-point field. A real RX sample is still required for final terminator/HIL validation, but it no longer blocks parser implementation.

**Files:**
- Modify: `projects/windlabx4/docs/specs/spec-pace1000-integration.md`
- Create: `shared/device-sdk/go/protocol/pace1000.go`
- Create: `shared/device-sdk/go/protocol/pace1000_test.go`

**Step 1: Record the confirmed scan contract in the spec**

Document the confirmed `%s%f` semantics:

- first `%s` field is consumed and discarded;
- second `%f` field is the raw value;
- raw value is multiplied by 1000;
- response must contain exactly these two fields.

Leave response terminator as a HIL validation item.

**Step 2: Write failing table tests**

Tests must cover the confirmed valid frame plus malformed variants:

- valid normal value with a discarded string prefix;
- valid sign/scientific notation only if the real protocol permits it;
- empty response;
- missing string or float field;
- extra field;
- invalid token;
- `NaN` and `Inf`;
- value whose converted pressure is outside the confirmed acceptable range.

Core assertion:

```go
got, err := ParsePACE1000Pressure([]byte(confirmedFrame))
if err != nil { t.Fatal(err) }
if got != 101325 { t.Fatalf("got %v Pa", got) }
```

**Step 3: Verify tests fail**

```powershell
cd shared/device-sdk/go
go test ./protocol -run PACE1000 -count=1
```

Expected: FAIL because parser/constant does not exist.

**Step 4: Implement the strict parser**

Expose only:

```go
const PACE1000Query = ":sens?\r"

func ParsePACE1000Pressure(response []byte) (float64, error)
```

The parser must match the confirmed frame format, convert with `raw * 1000`, reject non-finite values, and avoid searching arbitrary text for the first number.

**Step 5: Verify**

```powershell
cd shared/device-sdk/go
go test ./protocol -run PACE1000 -count=1
```

Expected: PASS.

**Acceptance criteria:**
- [x] Parser behavior follows the confirmed LabVIEW `%s%f` scan contract.
- [ ] Query constant contains CR and no LF.
- [ ] Invalid frames never produce 0 or stale values.

**Dependencies:** None for implementation; real RX sample remains required for final HIL validation.

**Estimated scope:** Medium, 3 files.

---

### Task 3: Implement the Shared PACE1000 Driver

**Files:**
- Modify: `shared/device-sdk/go/daq/core/types.go`
- Create: `shared/device-sdk/go/daq/hardware/pace1000.go`
- Create: `shared/device-sdk/go/daq/hardware/pace1000_test.go`
- Optionally create: `shared/device-sdk/go/daq/hardware/pace1000_real_test.go`

**Step 1: Write failing interface and lifecycle tests**

Use a local unexported `pace1000Port` interface and injected opener/clock in tests. Cover:

- compile-time implementation of `shared/daq/ports.Device`;
- Connect opens the configured serial name with 9600-8-N-1;
- Start writes exact query bytes, waits 500 ms through a fake clock, reads, parses, and emits one channel;
- Stop halts polling without closing the port and allows restart;
- Disconnect closes the port and waits for the loop to exit;
- three consecutive empty/timeout/parse failures trigger error callback and leave Acquiring;
- one success resets the consecutive-failure count;
- manual Stop/Disconnect do not emit an error callback.

**Step 2: Verify tests fail**

```powershell
cd shared/device-sdk/go
go test ./daq/hardware -run PACE1000 -count=1
```

Expected: FAIL because the driver and type do not exist.

**Step 3: Add the shared profile fields needed by a serial driver**

Add to shared `core.Profile`:

```go
SerialPort string `json:"serialPort,omitempty"`
BaudRate   int    `json:"baudRate,omitempty"`
```

Add `DevicePACE1000 Type = "PACE1000"`. Do not add PACE-specific configuration fields.

**Step 4: Implement the driver**

The production constructor should consume shared `core.Profile`; the test constructor may inject opener and wait function. Keep each function under 50 lines by separating connection, one query transaction, and acquisition-loop state transitions.

The emitted shared payload is:

```go
core.DataPayload{
	DeviceID:       profile.ID,
	Timestamp:      core.NowMs(),
	Channels:       []float64{pressurePa},
	ChannelIndices: []int{0},
}
```

**Step 5: Verify**

```powershell
cd shared/device-sdk/go
go test ./daq/hardware -run PACE1000 -count=1
go test -race ./daq/hardware -run PACE1000 -count=1
```

Expected: PASS without real 500 ms sleeps.

**Acceptance criteria:**
- [ ] Driver implements the existing shared device interface only.
- [ ] Every valid transaction emits exactly one Pa value at index 0.
- [ ] Stop and Disconnect are bounded and race-free.
- [ ] Three consecutive failures leave Acquiring and notify the adapter.

**Dependencies:** Tasks 1 and 2.

**Estimated scope:** Medium, 3-4 files.

---

## Checkpoint A: Shared SDK

Run:

```powershell
cd shared/device-sdk/go
go test ./serialport ./protocol ./daq/hardware -count=1
go test -race ./serialport ./daq/hardware -run PACE1000 -count=1
go vet ./...
go build ./...
```

Do not proceed if blocking-read cancellation, parser strictness, or race tests fail.

---

### Task 4: Add Backend Identity, Default Profile, and Calibration Exclusion

**Files:**
- Modify: `projects/windlabx4/services/api-go/internal/core/device/types.go`
- Modify: `projects/windlabx4/services/api-go/internal/core/device/types_test.go`
- Modify: `projects/windlabx4/services/api-go/internal/adapters/config/default_profiles.go`
- Modify: `projects/windlabx4/services/api-go/internal/adapters/config/default_profiles_test.go`
- Modify: `projects/windlabx4/services/api-go/internal/adapters/config/file_profile_store_test.go`

**Step 1: Write failing profile and calibration tests**

Assert that `NewDefaultProfile(id, DevicePACE1000)` returns:

- `Transport="serial"`;
- empty `SerialPort`;
- `BaudRate=9600`;
- `SamplingRate=2`;
- exactly one channel with index 0, `大气压力`, `Pa`, precision 1, range 30000..120000;
- `CalibrationEnabled=false`;
- `IsAtmosphericChannel(DevicePACE1000, 0)==true`.

Add a file-store round-trip test for `serialPort`, baud, type, and the one-channel contract.

**Step 2: Verify tests fail**

```powershell
cd projects/windlabx4/services/api-go
go test ./internal/core/device ./internal/adapters/config -run 'PACE1000|Atmospheric' -count=1
```

**Step 3: Implement the isolated profile branch**

Add `DevicePACE1000` and a `defaultPACE1000Channels()` helper. In the post-switch calibration-default loop, explicitly preserve `CalibrationEnabled=false` for PACE1000 instead of allowing `SupportsZeroCalibration("Pa")` to turn it on.

Extend `NormalizeProfile` so legacy/minimal PACE profiles receive serial transport, 9600 baud, 2 Hz, and the fixed channel without overwriting a user-selected `SerialPort`.

**Step 4: Run focused and complete config regression tests**

```powershell
cd projects/windlabx4/services/api-go
go test ./internal/core/device ./internal/adapters/config -count=1
go test ./internal/... ./api/... -count=1
```

Expected: all existing device defaults remain unchanged.

**Acceptance criteria:**
- [ ] PACE1000 profile defaults and normalization are deterministic.
- [ ] Existing default-profile callers and startup tests remain green.
- [ ] Backend calibration paths treat PACE1000 channel 0 as atmospheric/non-calibratable.

**Dependencies:** None; can run in parallel with Tasks 1-2.

**Estimated scope:** Medium, 5 files.

---

### Task 5: Add the WindLabX4 Adapter and Register All Factories

**Files:**
- Create: `projects/windlabx4/services/api-go/internal/adapters/hardware/pace1000_adapter.go`
- Create: `projects/windlabx4/services/api-go/internal/adapters/hardware/pace1000_adapter_test.go`
- Modify: `projects/windlabx4/services/api-go/internal/bootstrap/bootstrap.go`
- Modify: `projects/windlabx4/services/api-go/pkg/appcontext/context.go`
- Modify: `projects/windlabx4/services/api-go/pkg/apiserver/apiserver.go`
- Modify in a second focused patch: `projects/windlabx4/services/api-go/pkg/types/types.go`

**Step 1: Write failing adapter tests**

Assert compile-time implementation of `ports.Device` and `ports.ErrorNotifiable`, plus:

- shared profile mapping includes `SerialPort`, `BaudRate`, type, sampling rate, and one channel;
- shared payload maps to WindLabX4 payload with `DeviceType=PACE1000` and `DeviceName`;
- nil driver reports Disconnected;
- read-loop failure clears the driver and calls the registered error callback.

**Step 2: Verify tests fail**

```powershell
cd projects/windlabx4/services/api-go
go test ./internal/adapters/hardware -run PACE1000 -count=1
```

**Step 3: Implement the thin adapter**

Follow `T1602Adapter` lifecycle patterns but omit configuration interfaces. The adapter must not parse serial responses or apply pressure conversion; those belong in shared protocol/driver code.

**Step 4: Register all three factories**

Add explicit PACE1000 cases in:

- `internal/bootstrap.deviceFactory.Create`;
- `pkg/appcontext.deviceFactory.Create`;
- `pkg/apiserver.deviceFactory.Create`.

Never allow PACE1000 to fall through to `NewSimulatedDevice`.

**Step 5: Verify**

```powershell
cd projects/windlabx4/services/api-go
go test ./internal/adapters/hardware ./internal/bootstrap ./pkg/appcontext ./pkg/apiserver -run PACE1000 -count=1
go test ./internal/... ./api/... -count=1
```

**Acceptance criteria:**
- [ ] Adapter contains only lifecycle delegation and type translation.
- [ ] Every production factory creates the PACE1000 adapter.
- [ ] DeviceManager receives unexpected acquisition-loop errors.

**Dependencies:** Tasks 3 and 4.

**Estimated scope:** Split into two medium patches; no patch should exceed 5 files.

---

### Task 6: Add the Serial-Only Frontend Device Flow

**Files:**
- Modify: `projects/windlabx4/apps/desktop-wails/frontend/src/api/types.ts`
- Modify: `projects/windlabx4/apps/desktop-wails/frontend/src/components/device/DeviceManagementDrawer.vue`
- Modify: `projects/windlabx4/apps/desktop-wails/frontend/src/utils/deviceCalibration.ts`
- Modify: `projects/windlabx4/apps/desktop-wails/frontend/src/utils/__tests__/deviceCalibration.test.ts`
- Create if extraction is needed: `projects/windlabx4/apps/desktop-wails/frontend/src/utils/pace1000Profile.ts`
- Create if extraction is used: `projects/windlabx4/apps/desktop-wails/frontend/src/utils/__tests__/pace1000Profile.test.ts`

**Step 1: Write failing utility tests**

Prefer extracting the PACE default-profile fragment only if that makes it testable without mounting the 2100+ line drawer. Assert:

- type literal is accepted;
- default profile is `transport='serial'`, baud 9600, 2 Hz;
- exactly one fixed atmospheric-pressure channel;
- PACE1000 is not calibratable.

**Step 2: Implement minimal UI branching**

Update:

- `DeviceType` union and device type options;
- `createDefaultChannels` and `createBlankProfile`;
- transport helpers so PACE1000 is serial-only, not TCP and not switchable;
- validation so serial port is required and baud does not need user editing;
- communication section so PACE1000 shows COM input and fixed `9600` text/read-only value, with no IP/port/transport switch;
- channel editor so unit/name cannot drift from the fixed contract, or hide editable channel controls for PACE1000.

Do not add a new PACE settings panel.

**Step 3: Verify**

```powershell
cd projects/windlabx4/apps/desktop-wails/frontend
npm run test -- --run
npm run typecheck
npm run build
```

**Step 4: Manual responsive check**

At desktop and narrow widths, verify the COM input and fixed baud label do not overlap, and PACE1000 never shows IP/port fields.

**Acceptance criteria:**
- [ ] Operator can create/save a PACE1000 profile after entering a COM port.
- [ ] UI cannot change transport, baud, channel name, channel count, or unit.
- [ ] Calibration actions are hidden/disabled by the existing whitelist.

**Dependencies:** Task 4. Runtime connection verification also needs Task 5.

**Estimated scope:** Medium, 4-6 files depending on test extraction.

---

### Task 7: Verify Storage and Calibration Integration

**Files:**
- Modify: `projects/windlabx4/services/api-go/internal/adapters/storage/csv_sink_test.go`
- Modify: `projects/windlabx4/services/api-go/internal/usecase/device_manager_test.go`
- Modify only if a test exposes a gap: corresponding production file

**Step 1: Add failing integration assertions**

Add tests proving:

- PACE1000 selects generic long format, not DAQ-P-1604 wide format;
- recorded row carries channel index 0 and the Pa value;
- full-device calibration filters the PACE atmospheric channel;
- direct calibration request for channel 0 is rejected.

**Step 2: Run tests**

```powershell
cd projects/windlabx4/services/api-go
go test ./internal/adapters/storage ./internal/usecase -run PACE1000 -count=1
```

If tests pass without production changes, keep production unchanged. This is the expected result for storage because generic long format already applies to every type except `DAQ-P-1604`.

**Step 3: Implement only demonstrated gaps**

Do not add a PACE-specific CSV branch unless a failing test proves generic format cannot satisfy the spec.

**Acceptance criteria:**
- [ ] PACE1000 records through generic long format.
- [ ] No calibration offset can be created for PACE1000 channel 0.
- [ ] No unrelated storage or calibration behavior changes.

**Dependencies:** Task 4; may run in parallel with Task 6.

**Estimated scope:** Small, tests first; production changes likely unnecessary.

---

## Checkpoint B: WindLabX4 Integration

```powershell
cd projects/windlabx4/services/api-go
go test ./internal/... ./api/... -count=1
go vet ./...
go build -buildvcs=false ./...
gofmt -l .

cd ../../apps/desktop-wails/frontend
npm run test -- --run
npm run typecheck
npm run build
```

Expected: all commands pass and `gofmt -l .` prints nothing.

---

### Task 8: Full Verification and Hardware Acceptance

**Files:**
- Modify: `projects/windlabx4/docs/specs/spec-pace1000-integration.md` with captured evidence/results
- Optionally modify: hardware-gated real test created in Task 3

**Step 1: Run workspace structure gates**

```powershell
cd C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace
.\validate-structure.ps1
.\validate-frontend-structure.ps1 -CheckFileSize
```

**Step 2: Run full relevant modules**

```powershell
cd shared/device-sdk/go
go test ./...
go vet ./...

cd ../../../projects/windlabx4/services/api-go
go test ./...
go vet ./...
go build -buildvcs=false ./...

cd ../../apps/desktop-wails/frontend
npm run test -- --run
npm run typecheck
npm run build
```

**Step 3: Real hardware test**

With the PACE1000 connected:

1. Select the actual COM port and connect.
2. Capture TX and verify exact bytes `3A 73 65 6E 73 3F 0D`.
3. Capture RX and verify it matches the parser fixture.
4. Compare the displayed Pa value with the PACE1000 panel.
5. Acquire and record for 10 minutes.
6. Stop/start acquisition three times.
7. Disconnect/reconnect three times and verify the COM port is released each time.
8. Inject or cause three invalid/timeout responses and verify the UI leaves Acquiring with an actionable error.

**Step 4: Analyze the final diff**

Run `gitnexus_detect_changes(scope="all")`. Review affected processes and confirm changes are limited to serial I/O, PACE1000 device creation, profile/config startup, acquisition, storage tests, and frontend device management.

**Acceptance criteria:**
- [ ] All automated gates pass.
- [ ] Real hardware pressure agrees within display precision.
- [ ] No blocked goroutine or occupied COM port remains after disconnect.
- [ ] Spec records final response format and HIL evidence.

**Dependencies:** Tasks 1-7.

**Estimated scope:** Verification only.

---

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Unknown RX grammar | High | Block parser implementation until raw ASCII+hex sample is captured |
| `Port.Read` blocks `Close` through shared mutex | High | Task 1 regression test and lock-scope fix before driver work |
| `NewDefaultProfile` has CRITICAL blast radius | High | Isolated switch branch plus full config/startup/integration regression suite |
| 500 ms wait causes slow/flaky tests | Medium | Inject fake wait/clock; no real sleeps in unit tests |
| Drawer is already over 2000 lines | Medium | Keep branch minimal; extract only testable profile helper if needed; run frontend structure gate |
| Three separate factories drift | Medium | Register and test all three in one explicit task |
| Pa unit enables generic calibration by default | High | Explicit `IsAtmosphericChannel` rule and preserve `CalibrationEnabled=false` during normalization |
| Serial response arrives in chunks | Medium | Driver accumulates bytes according to confirmed terminator/format; parser receives one complete frame |

## HIL Follow-Up

Capture one raw PACE1000 response to `:sens?\r`, preferably as ASCII and hex, to record the real response terminator and first-field text. This no longer blocks implementation because `%s%f` already fixes the field structure.

## Commit Policy

Do not create commits, amend, push, or open a PR unless the user explicitly requests it. If commits are later requested, stage each verified task as a separate logical commit.
