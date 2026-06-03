# Implementation Plan: DAQ-T-1603 配置项精简与通道独立热电偶类型

## Overview

Remove "平均次数" and "序号" from both daq-t1603 and wind-daq UIs. Add per-channel thermocouple type selection. Go struct field `ThermocoupleType` renamed to `ThermocoupleTypes` (16-char string). Shared SDK unchanged.

## Architecture Decisions

- **Per-channel TC type storage**: `T1603Config.ThermocoupleTypes` (16-char string, matches hardware protocol). `ChannelConfig.ThermocoupleType` (per-channel char) for UI convenience. Save aggregates all channel types into the 16-char string.
- **wind-daq approach**: `DaqT1603Config.vue` keeps only global HW params (removes avg/seq/global TC type). Per-channel TC type column added directly in `DeviceManagementDrawer.vue`'s channel table.
- **Backward compat**: Go struct fields `AverageCount`/`ShowSequence` remain in Go code for JSON compat; `thermocoupleType`(old name) → `thermocoupleTypes`(new name) is a breaking rename requiring `wails generate module`.

## Task List

### Phase 1: daq-t1603 Foundation

#### Task 1: Go struct rename + adapter update

**Description:** Rename `T1603Config.ThermocoupleType` → `T1603Config.ThermocoupleTypes`. Add `ThermocoupleType` to `ChannelConfig`. Update adapter to use the full 16-char string directly instead of `strings.Repeat`. Update `OnConfigSynced` callback to store all 16 chars.

**Acceptance criteria:**
- [ ] `core/types.go`: `T1603Config` has `ThermocoupleTypes string` (no `ThermocoupleType`)
- [ ] `core/types.go`: `ChannelConfig` has `ThermocoupleType string`
- [ ] `adapters/hardware/t1603_adapter.go`: `mapT1603SharedConfig` uses `cfg.ThermocoupleTypes` directly
- [ ] `adapters/hardware/t1603_adapter.go`: `OnConfigSynced` stores `cfg.ThermocoupleTypes` (all 16 chars)

**Verification:**
- [ ] `go vet ./...` passes
- [ ] `go test ./...` passes

**Dependencies:** None

**Files touched:**
- `projects/daq-t1603/apps/desktop-wails/core/types.go`
- `projects/daq-t1603/apps/desktop-wails/adapters/hardware/t1603_adapter.go`

**Scope:** Small (2 files)

---

#### Task 2: Regenerate Wails bindings

**Description:** Run `wails generate module` to regenerate frontend/go binding files reflecting the renamed Go struct fields.

**Acceptance criteria:**
- [ ] `frontend/wailsjs/go/models.ts` reflects `thermocoupleTypes` (not `thermocoupleType`) and `thermocoupleType` on channel config

**Verification:**
- [ ] `frontend/wailsjs/go/models.ts` updated with new field names
- [ ] `npm run typecheck` in frontend passes (or expected failures documented)

**Dependencies:** Task 1

**Files touched:**
- `frontend/wailsjs/go/models.ts` (auto-generated)

**Scope:** XS (1 command + verify output)

---

### Phase 2: daq-t1603 Frontend

#### Task 3: Frontend types + store

**Description:** Update `bridge/deviceBridge.ts` interfaces and `stores/deviceStore.ts` to reflect new field names and add `thermocoupleType` to `ChannelConfig`. Update default config to use 16-char string.

**Acceptance criteria:**
- [ ] `deviceBridge.ts`: `T1603Config` has `thermocoupleTypes: string`, `ChannelConfig` has `thermocoupleType: string`
- [ ] `deviceStore.ts`: `defaultT1603Config()` uses `thermocoupleTypes: 'KKKKKKKKKKKKKKKK'`
- [ ] `deviceStore.ts`: `t1603Defaults()` uses `thermocoupleTypes`
- [ ] `deviceStore.ts`: `updateT1603Config` aggregates per-channel types into 16-char string before saving (or passes through)

**Verification:**
- [ ] `npm run typecheck` passes (or expected failures from wailsjs mismatches documented)

**Dependencies:** Task 2

**Files touched:**
- `projects/daq-t1603/apps/desktop-wails/frontend/src/bridge/deviceBridge.ts`
- `projects/daq-t1603/apps/desktop-wails/frontend/src/stores/deviceStore.ts`

**Scope:** Small (2 files)

---

#### Task 4: daq-t1603 Vue config UI

**Description:** Rewrite `DaqT1603Config.vue`:
1. Remove "平均次数" select control (comment out)
2. Remove "序号" toggle control (comment out)
3. Replace single "热电偶类型" select with per-channel dropdown in each channel row
4. Add `channelTcTypes` reactive array (16 strings, default all 'K')

**Acceptance criteria:**
- [ ] Average count control absent from UI
- [ ] Sequence number toggle absent from UI
- [ ] Each channel row has a compact TC type select (K/J/T/E/N/S/R/B)
- [ ] `saveConfig()` reads per-channel types and composes 16-char string for `thermocoupleTypes`
- [ ] `syncFormFromProfile()` distributes `thermocoupleTypes` 16-char string to each channel

**Verification:**
- [ ] `npm run typecheck` passes
- [ ] Manual: open config panel, verify avg/seq gone
- [ ] Manual: verify each channel has TC type select with correct default

**Dependencies:** Task 3

**Files touched:**
- `projects/daq-t1603/apps/desktop-wails/frontend/src/components/device/DaqT1603Config.vue`

**Scope:** Medium (1 file, significant edits)

---

### Phase 3: wind-daq Frontend

#### Task 5: wind-daq types + DaqT1603Config.vue

**Description:**
1. `api/types.ts`: Add optional `thermocoupleType?: string` to `ChannelConfig`
2. `DaqT1603Config.vue`: Remove "平均次数", "序号", and global "热电偶类型" controls
3. Remove `tcTypeValue` computed, `tcTypes` array, and related emits
4. Keep: channelMask, samplingRate, binaryFormat, triggerMode/Edge/Count, showTimestamp, openCircuitCheck

**Acceptance criteria:**
- [ ] `ChannelConfig` has optional `thermocoupleType`
- [ ] wind-daq `DaqT1603Config.vue` has no avg/seq TC type controls
- [ ] Props/emits corresponding to removed fields are removed or kept for pass-through

**Verification:**
- [ ] `npm run typecheck` passes
- [ ] Manual: verify no compile errors in DeviceManagementDrawer using DaqT1603Config

**Dependencies:** None (can be done in parallel with Phase 2)

**Files touched:**
- `projects/wind-daq/apps/desktop-wails/frontend/src/api/types.ts`
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/device/DaqT1603Config.vue`

**Scope:** Medium (2 files)

---

#### Task 6: wind-daq DeviceManagementDrawer.vue

**Description:**
1. In the DAQ-T-1603 channel table section (`v-if="draft.type === 'DAQ-T-1603'"`), add a new table column for thermocouple type (after 通道名称)
2. Each row has a `<select>` with TC type options (K/J/T/E/N/S/R/B)
3. Default: all channels 'K'
4. Before save: aggregate per-channel types into `draft.daqT1603Config!.thermocoupleTypes` 16-char string
5. On load (openEdit/openCreate): distribute stored `thermocoupleTypes` string to each channel's `thermocoupleType` field
6. Remove `v-model:average-count` and `v-model:show-sequence` from `<DaqT1603Config>` usage

**Acceptance criteria:**
- [ ] DAQ-T-1603 channel table shows "#", "通道名称", "热电偶类型", "单位" columns
- [ ] Each channel has TC type select with correct default
- [ ] Save composes 16-char string from per-channel selections
- [ ] Load distributes 16-char string to per-channel fields

**Verification:**
- [ ] `npm run typecheck` passes
- [ ] Manual: open T1603 editor, verify channel table has TC type column

**Dependencies:** Task 5

**Files touched:**
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/device/DeviceManagementDrawer.vue`

**Scope:** Medium (1 file, significant edits)

---

### Checkpoint: Complete

#### Task 7: Verification

**Description:** Run all verification commands across both projects.

**Acceptance criteria:**
- [ ] `go vet ./...` and `go test ./...` in `projects/daq-t1603/apps/desktop-wails` pass
- [ ] `go vet ./...` and `go test ./...` in `projects/wind-daq/services/api-go` pass (if Go changes)
- [ ] `npm run typecheck` in `projects/daq-t1603/apps/desktop-wails/frontend` passes
- [ ] `npm run typecheck` in `projects/wind-daq/apps/desktop-wails/frontend` passes

**Dependencies:** Tasks 1-6

**Files touched:** None

**Scope:** XS (commands only)

## Dependency Graph

```
Task 1 (Go) ──→ Task 2 (Wails) ──→ Task 3 (TS/Store) ──→ Task 4 (Vue UI)
                                                             │
Task 5 (wind-daq TS/UI) ─────────────────────────────────────┘
                                                             │
                                                    Task 7 (Verify)
                                                    ↑
Task 6 (wind-daq drawer) ───────────────────────────┘
```

Tasks 5+6 can be parallelized with Tasks 2-4 (separate project).

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| `wails generate module` output differs from expected | Med | Inspect generated models.ts, manually adjust bridge types |
| Existing profiles with `thermocoupleType` (old field name) break | Med | JSON unmarshal will silently miss the field; add fallback in adapter or loading code |
| wind-daq `DaqT1603Config.vue` prop/emit refactoring cascades | Low | Verify with `npm run typecheck` after each edit |
