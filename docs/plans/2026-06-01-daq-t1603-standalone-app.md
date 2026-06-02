# DAQ-T-1603 温度采集独立桌面应用 — 实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**目标:** 为 DAQ-T-1603 16通道温度采集器构建独立 Wails 桌面应用，提供设备列表管理、实时波形+数值显示、硬件配置、数据保存四大功能。

**架构:** Go 六边形架构后端 (core → ports → usecase → adapters) + Vue 3/TypeScript 前端通过 Wails 绑定。设备驱动复用 `shared/device-sdk/go` 中已有的 `DAQT1603` 驱动和 TCP 帧解析器。

**技术栈:** Wails v2 (Go + Vue 3)、TypeScript、Pinia、ECharts (vue-echarts)、CSS 自定义属性

**状态:** 后端 Tasks 1-7 已完成代码设计（见下方），前端 Tasks 8-15 已**实现并打磨完成**，代码位于 `projects/daq-t1603/apps/desktop-wails/frontend/src/` 下。

---

## 任务清单总览

| # | 任务 | 估计文件数 |
|---|------|-----------|
| 1 | 创建项目骨架 | 20+ |
| 2 | 核心领域类型 (core) | 2 |
| 3 | 端口接口定义 (ports) | 3 |
| 4 | 适配器实现 (adapters) | 3 |
| 5 | Usecase 编排层 | 2 |
| 6 | Wails 后端绑定层 | 1 |
| 7 | Wails 入口 + Go 模块配置 | 3 |
| 8 | 前端布局壳 | 4 |
| 9 | Pinia 状态管理 | 2 |
| 10 | 设备列表组件 | 1 |
| 11 | 波形图组件 | 1 |
| 12 | 通道数值网格组件 | 2 |
| 13 | 配置面板 | 1 |
| 14 | 主监控视图 | 1 |
| 15 | Wails 前端入口 | 3 |
| 16 | 注册到 workspace + 验证 | 2 |

---

### Task 1: 创建项目骨架

**文件:**
- 创建: `projects/daq-t1603/apps/desktop-wails/frontend/`
- 创建: `projects/daq-t1603/apps/desktop-wails/backend/`
- 创建: `projects/daq-t1603/services/api-go/cmd/server/`
- 创建: `projects/daq-t1603/services/api-go/internal/core/`
- 创建: `projects/daq-t1603/services/api-go/internal/ports/`
- 创建: `projects/daq-t1603/services/api-go/internal/usecase/`
- 创建: `projects/daq-t1603/services/api-go/internal/adapters/hardware/`
- 创建: `projects/daq-t1603/services/api-go/internal/adapters/config/`
- 创建: `projects/daq-t1603/services/api-go/internal/adapters/recording/`
- 创建: `projects/daq-t1603/tests/integration/`
- 创建: `projects/daq-t1603/AGENTS.md`
- 创建: `projects/daq-t1603/CLAUDE.md`

**Step 1: 运行 new-project.ps1 创建骨架目录**

```powershell
cd C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace
powershell -File .\scripts\new-project.ps1 -Name daq-t1603
```

期望输出: `Created project skeleton: projects/daq-t1603`

**Step 2: 删除不需要的目录**

本项目不需要 `adapters/db`、`adapters/mq`、`contracts/`、`deploy/`，保留需要的即可。

```powershell
Remove-Item -Recurse -Force projects/daq-t1603/contracts
Remove-Item -Recurse -Force projects/daq-t1603/deploy
Remove-Item -Recurse -Force projects/daq-t1603/services/api-go/internal/adapters/db
Remove-Item -Recurse -Force projects/daq-t1603/services/api-go/internal/adapters/mq
```

**Step 3: 创建适配器需要的目录**

```powershell
New-Item -ItemType Directory -Force -Path projects/daq-t1603/services/api-go/internal/adapters/config
New-Item -ItemType Directory -Force -Path projects/daq-t1603/services/api-go/internal/adapters/recording
```

**Step 4: 提交骨架**

```bash
cd C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace
git add projects/daq-t1603/
git commit -m "feat(daq-t1603): create project skeleton"
```

---

### Task 2: 核心领域类型 (core)

**文件:**
- 创建: `projects/daq-t1603/services/api-go/internal/core/types.go`
- 创建: `projects/daq-t1603/services/api-go/internal/core/recording.go`

**Step 1: 编写 types.go — 领域类型定义**

创建 `projects/daq-t1603/services/api-go/internal/core/types.go`:

```go
package core

import "time"

// DeviceStatus 设备连接状态
type DeviceStatus int

const (
	StatusDisconnected DeviceStatus = iota
	StatusConnected
	StatusAcquiring
	StatusError
)

func (s DeviceStatus) String() string {
	switch s {
	case StatusDisconnected:
		return "Disconnected"
	case StatusConnected:
		return "Connected"
	case StatusAcquiring:
		return "Acquiring"
	case StatusError:
		return "Error"
	default:
		return "Unknown"
	}
}

// T1603Config T1603 硬件配置参数
type T1603Config struct {
	ThermocoupleType string `json:"thermocoupleType"`
	ColdJunction     string `json:"coldJunction"`
	FilterHz         int    `json:"filterHz"`
}

// ChannelConfig 单个通道配置
type ChannelConfig struct {
	Index   int    `json:"index"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Unit    string `json:"unit"`
	Color   string `json:"color"`
}

// TemperatureProfile 温度采集设备配置
type TemperatureProfile struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Address  string          `json:"address"`
	Port     int             `json:"port"`
	Channels []ChannelConfig `json:"channels"`
	T1603Cfg T1603Config     `json:"t1603Config"`
}

// TemperatureSnapshot 一次采样快照
type TemperatureSnapshot struct {
	DeviceID  string    `json:"deviceId"`
	Timestamp int64     `json:"timestamp"`
	Values    []float64 `json:"values"`
	Unit      string    `json:"unit"`
}

// DeviceState 设备运行时状态（非持久化）
type DeviceState struct {
	Profile      TemperatureProfile
	Status       DeviceStatus
	Error        string
	ConnectedAt  int64
	AcquiringAt  int64
	SamplingRate float64
}

// RecordingStatus 录制状态
type RecordingStatus int

const (
	RecordingIdle RecordingStatus = iota
	RecordingActive
)

// RecordingSession 录制会话
type RecordingSession struct {
	ID            string
	OutputDir     string
	FilePrefix    string
	StartTime     time.Time
	SnapshotCount int
	Status        RecordingStatus
}
```

**Step 2: 验证编译**

```powershell
cd C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace
go vet ./projects/daq-t1603/services/api-go/internal/core/...
```

期望输出: 成功（无错误）

**Step 3: 提交**

```bash
git add projects/daq-t1603/services/api-go/internal/core/
git commit -m "feat(daq-t1603): add core domain types"
```

---

### Task 3: 端口接口定义 (ports)

**文件:**
- 创建: `projects/daq-t1603/services/api-go/internal/ports/device.go`
- 创建: `projects/daq-t1603/services/api-go/internal/ports/config.go`
- 创建: `projects/daq-t1603/services/api-go/internal/ports/recording.go`

**Step 1: 编写 device.go — 设备端口接口**

```go
package ports

import "daq-t1603/services/api-go/internal/core"

// DevicePort 设备操作接口
type DevicePort interface {
	Connect(profile core.TemperatureProfile) error
	Disconnect(id string) error
	StartAcquisition(id string) (<-chan core.TemperatureSnapshot, error)
	StopAcquisition(id string) error
	Status(id string) (core.DeviceState, bool)
	ApplyConfig(id string, cfg core.T1603Config) error
	SetDataSink(id string, sink func(core.TemperatureSnapshot))
}
```

**Step 2: 编写 config.go — 配置持久化接口**

```go
package ports

import "daq-t1603/services/api-go/internal/core"

// ConfigPort 设备配置持久化接口
type ConfigPort interface {
	LoadProfiles() ([]core.TemperatureProfile, error)
	SaveProfile(profile core.TemperatureProfile) error
	DeleteProfile(id string) error
}
```

**Step 3: 编写 recording.go — 录制保存接口**

```go
package ports

import "daq-t1603/services/api-go/internal/core"

// RecordingPort 数据录制保存接口
type RecordingPort interface {
	Start(outputDir string, prefix string) error
	Write(snapshot core.TemperatureSnapshot) error
	Stop() error
	Status() core.RecordingSession
}
```

**Step 4: 验证编译**

```powershell
cd C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace
go vet ./projects/daq-t1603/services/api-go/internal/ports/...
```

**Step 5: 提交**

```bash
git add projects/daq-t1603/services/api-go/internal/ports/
git commit -m "feat(daq-t1603): add port interfaces for device, config, recording"
```

---

### Task 4: 适配器实现 (adapters)

**文件:**
- 创建: `projects/daq-t1603/services/api-go/internal/adapters/hardware/t1603_adapter.go`
- 创建: `projects/daq-t1603/services/api-go/internal/adapters/config/json_config.go`
- 创建: `projects/daq-t1603/services/api-go/internal/adapters/recording/csv_recorder.go`

**Step 1: 编写 t1603_adapter.go — 设备适配器**

包装 `shared/device-sdk/go/daq/hardware.DAQT1603`，实现 `ports.DevicePort` 接口。

```go
package hardware

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	sharedhw "shared/device-sdk/go/daq/hardware"
	sharedcore "shared/device-sdk/go/daq/core"

	"daq-t1603/services/api-go/internal/core"
	"daq-t1603/services/api-go/internal/ports"
)

type T1603Adapter struct {
	mu      sync.RWMutex
	drivers map[string]*sharedhw.DAQT1603
	status  map[string]*core.DeviceState
	sinks   map[string]func(core.TemperatureSnapshot)
	seq     atomic.Int64
}

func NewT1603Adapter() *T1603Adapter {
	return &T1603Adapter{
		drivers: make(map[string]*sharedhw.DAQT1603),
		status:  make(map[string]*core.DeviceState),
		sinks:   make(map[string]func(core.TemperatureSnapshot)),
	}
}

func (a *T1603Adapter) Connect(profile core.TemperatureProfile) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.drivers[profile.ID]; exists {
		return fmt.Errorf("device %s already connected", profile.ID)
	}

	sharedProfile := sharedcore.Profile{
		ID:      profile.ID,
		Name:    profile.Name,
		Type:    "DAQ-T-1603",
		Address: profile.Address,
		Port:    profile.Port,
	}
	dev := sharedhw.NewDAQT1603(sharedProfile)
	if err := dev.Connect(); err != nil {
		return fmt.Errorf("connect device %s: %w", profile.ID, err)
	}

	a.drivers[profile.ID] = dev
	a.status[profile.ID] = &core.DeviceState{
		Profile:     profile,
		Status:      core.StatusConnected,
		ConnectedAt: core.NowMs(),
	}
	return nil
}

func (a *T1603Adapter) Disconnect(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	dev, ok := a.drivers[id]
	if !ok {
		return nil
	}
	delete(a.drivers, id)
	delete(a.sinks, id)
	if st, exists := a.status[id]; exists {
		st.Status = core.StatusDisconnected
	}
	return dev.Disconnect()
}

func (a *T1603Adapter) StartAcquisition(id string) (<-chan core.TemperatureSnapshot, error) {
	a.mu.Lock()
	dev, ok := a.drivers[id]
	if !ok {
		a.mu.Unlock()
		return nil, fmt.Errorf("device %s not connected", id)
	}

	ch := make(chan core.TemperatureSnapshot, 64)
	a.sinks[id] = func(snapshot core.TemperatureSnapshot) {
		select {
		case ch <- snapshot:
		default:
		}
	}
	if st, exists := a.status[id]; exists {
		st.Status = core.StatusAcquiring
		st.AcquiringAt = core.NowMs()
	}
	a.mu.Unlock()

	dev.SetDataSink(func(payload sharedcore.DataPayload) {
		values := make([]float64, len(payload.Channels))
		copy(values, payload.Channels)
		unit := "°C"
		a.mu.RLock()
		if st, ok := a.status[id]; ok {
			if len(st.Profile.Channels) > 0 {
				unit = st.Profile.Channels[0].Unit
			}
		}
		sink := a.sinks[id]
		a.mu.RUnlock()
		if sink != nil {
			sink(core.TemperatureSnapshot{
				DeviceID:  id,
				Timestamp: payload.Timestamp,
				Values:    values,
				Unit:      unit,
			})
		}
	})

	if err := dev.StartAcquisition(); err != nil {
		return nil, fmt.Errorf("start acquisition %s: %w", id, err)
	}
	return ch, nil
}

func (a *T1603Adapter) StopAcquisition(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	dev, ok := a.drivers[id]
	if !ok {
		return nil
	}
	delete(a.sinks, id)
	if st, exists := a.status[id]; exists {
		st.Status = core.StatusConnected
	}
	return dev.StopAcquisition()
}

func (a *T1603Adapter) Status(id string) (core.DeviceState, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	st, ok := a.status[id]
	if !ok {
		return core.DeviceState{}, false
	}
	// 从底层驱动读最新状态
	dev, hasDev := a.drivers[id]
	if hasDev {
		ds := dev.Status()
		if ds.Connection == sharedcore.ConnectionDisconnected {
			st.Status = core.StatusDisconnected
		} else if ds.Acquiring {
			st.Status = core.StatusAcquiring
		} else {
			st.Status = core.StatusConnected
		}
	}
	return *st, true
}

func (a *T1603Adapter) ApplyConfig(id string, cfg core.T1603Config) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	dev, ok := a.drivers[id]
	if !ok {
		return fmt.Errorf("device %s not connected", id)
	}
	if st, exists := a.status[id]; exists {
		st.Profile.T1603Cfg = cfg
	}
	return dev.ApplyDaqT1603Config(sharedcore.DaqT1603HardwareConfig{
		ThermocoupleType: cfg.ThermocoupleType,
		ColdJunction:     cfg.ColdJunction,
		FilterHz:         cfg.FilterHz,
	})
}

func (a *T1603Adapter) SetDataSink(id string, sink func(core.TemperatureSnapshot)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sinks[id] = sink
}
```

**Step 2: 编写 json_config.go — JSON 配置持久化**

```go
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"daq-t1603/services/api-go/internal/core"
)

type JSONConfigStore struct {
	mu       sync.RWMutex
	filePath string
}

func NewJSONConfigStore(filePath string) *JSONConfigStore {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		panic(err)
	}
	return &JSONConfigStore{filePath: filePath}
}

func (s *JSONConfigStore) LoadProfiles() ([]core.TemperatureProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var profiles []core.TemperatureProfile
	if err := json.Unmarshal(data, &profiles); err != nil {
		return nil, err
	}
	return profiles, nil
}

func (s *JSONConfigStore) SaveProfile(profile core.TemperatureProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	profiles, err := s.loadUnsafe()
	if err != nil {
		return err
	}
	replaced := false
	for i, p := range profiles {
		if p.ID == profile.ID {
			profiles[i] = profile
			replaced = true
			break
		}
	}
	if !replaced {
		profiles = append(profiles, profile)
	}
	return s.saveUnsafe(profiles)
}

func (s *JSONConfigStore) DeleteProfile(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	profiles, err := s.loadUnsafe()
	if err != nil {
		return err
	}
	filtered := profiles[:0]
	for _, p := range profiles {
		if p.ID != id {
			filtered = append(filtered, p)
		}
	}
	return s.saveUnsafe(filtered)
}

func (s *JSONConfigStore) loadUnsafe() ([]core.TemperatureProfile, error) {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var profiles []core.TemperatureProfile
	if err := json.Unmarshal(data, &profiles); err != nil {
		return nil, err
	}
	return profiles, nil
}

func (s *JSONConfigStore) saveUnsafe(profiles []core.TemperatureProfile) error {
	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
}
```

**Step 3: 编写 csv_recorder.go — CSV 录制保存**

```go
package recording

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"daq-t1603/services/api-go/internal/core"
)

type CSVRecorder struct {
	mu      sync.RWMutex
	file    *os.File
	session core.RecordingSession
	writer  *csvWriter
}

type csvWriter struct {
	file *os.File
}

func (w *csvWriter) write(record []string) error {
	line := ""
	for i, field := range record {
		if i > 0 {
			line += ","
		}
		line += field
	}
	line += "\n"
	_, err := w.file.WriteString(line)
	return err
}

func NewCSVRecorder() *CSVRecorder {
	return &CSVRecorder{}
}

func (r *CSVRecorder) Start(outputDir string, prefix string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.session.Status == core.RecordingActive {
		return fmt.Errorf("recording already in progress")
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	filename := fmt.Sprintf("%s_%s.csv", prefix, time.Now().Format("20060102-150405"))
	filePath := filepath.Join(outputDir, filename)
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}

	// 写入 CSV 表头
	header := "Timestamp"
	for i := 0; i < 16; i++ {
		header += fmt.Sprintf(",CH%02d", i+1)
	}
	header += "\n"
	if _, err := f.WriteString(header); err != nil {
		f.Close()
		return fmt.Errorf("write header: %w", err)
	}

	r.file = f
	r.writer = &csvWriter{file: f}
	r.session = core.RecordingSession{
		ID:         fmt.Sprintf("rec_%d", time.Now().UnixNano()),
		OutputDir:  outputDir,
		FilePrefix: prefix,
		StartTime:  time.Now(),
		Status:     core.RecordingActive,
	}
	return nil
}

func (r *CSVRecorder) Write(snapshot core.TemperatureSnapshot) error {
	r.mu.RLock()
	status := r.session.Status
	writer := r.writer
	r.mu.RUnlock()

	if status != core.RecordingActive || writer == nil {
		return nil
	}

	t := time.UnixMilli(snapshot.Timestamp)
	record := make([]string, 0, 17)
	record = append(record, t.Format("2006-01-02 15:04:05.000"))
	for _, v := range snapshot.Values {
		record = append(record, fmt.Sprintf("%.3f", v))
	}
	if err := writer.write(record); err != nil {
		return err
	}

	r.mu.Lock()
	r.session.SnapshotCount++
	r.mu.Unlock()
	return nil
}

func (r *CSVRecorder) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.file != nil {
		r.file.Close()
		r.file = nil
		r.writer = nil
	}
	r.session.Status = core.RecordingIdle
	return nil
}

func (r *CSVRecorder) Status() core.RecordingSession {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.session
}
```

**Step 4: 验证编译**

```powershell
cd C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace
go vet ./projects/daq-t1603/services/api-go/internal/adapters/...
```

**Step 5: 提交**

```bash
git add projects/daq-t1603/services/api-go/internal/adapters/
git commit -m "feat(daq-t1603): implement adapters for T1603 hardware, JSON config, CSV recording"
```

---

### Task 5: Usecase 编排层

**文件:**
- 创建: `projects/daq-t1603/services/api-go/internal/usecase/device_usecase.go`
- 创建: `projects/daq-t1603/services/api-go/internal/usecase/recording_usecase.go`

**Step 1: 编写 device_usecase.go**

```go
package usecase

import (
	"fmt"
	"log/slog"

	"daq-t1603/services/api-go/internal/core"
	"daq-t1603/services/api-go/internal/ports"
)

type DeviceUsecase struct {
	device ports.DevicePort
	config ports.ConfigPort
}

func NewDeviceUsecase(device ports.DevicePort, config ports.ConfigPort) *DeviceUsecase {
	return &DeviceUsecase{device: device, config: config}
}

func (uc *DeviceUsecase) GetProfiles() []core.TemperatureProfile {
	profiles, err := uc.config.LoadProfiles()
	if err != nil {
		slog.Warn("load profiles", "error", err)
		return nil
	}
	if profiles == nil {
		return []core.TemperatureProfile{}
	}
	return profiles
}

func (uc *DeviceUsecase) UpsertProfile(profile core.TemperatureProfile) error {
	return uc.config.SaveProfile(profile)
}

func (uc *DeviceUsecase) DeleteProfile(id string) error {
	_ = uc.device.Disconnect(id)
	return uc.config.DeleteProfile(id)
}

func (uc *DeviceUsecase) Connect(id string) error {
	profiles, err := uc.config.LoadProfiles()
	if err != nil {
		return fmt.Errorf("load profiles: %w", err)
	}
	var profile *core.TemperatureProfile
	for i := range profiles {
		if profiles[i].ID == id {
			profile = &profiles[i]
			break
		}
	}
	if profile == nil {
		return fmt.Errorf("profile %s not found", id)
	}
	return uc.device.Connect(*profile)
}

func (uc *DeviceUsecase) Disconnect(id string) error {
	return uc.device.Disconnect(id)
}

func (uc *DeviceUsecase) StartAcquisition(id string) (<-chan core.TemperatureSnapshot, error) {
	return uc.device.StartAcquisition(id)
}

func (uc *DeviceUsecase) StopAcquisition(id string) error {
	return uc.device.StopAcquisition(id)
}

func (uc *DeviceUsecase) GetStatus(id string) (core.DeviceState, bool) {
	return uc.device.Status(id)
}

func (uc *DeviceUsecase) ApplyConfig(id string, cfg core.T1603Config) error {
	return uc.device.ApplyConfig(id, cfg)
}
```

**Step 2: 编写 recording_usecase.go**

```go
package usecase

import (
	"daq-t1603/services/api-go/internal/core"
	"daq-t1603/services/api-go/internal/ports"
)

type RecordingUsecase struct {
	recorder ports.RecordingPort
}

func NewRecordingUsecase(recorder ports.RecordingPort) *RecordingUsecase {
	return &RecordingUsecase{recorder: recorder}
}

func (uc *RecordingUsecase) Start(outputDir string, prefix string) error {
	return uc.recorder.Start(outputDir, prefix)
}

func (uc *RecordingUsecase) Write(snapshot core.TemperatureSnapshot) error {
	return uc.recorder.Write(snapshot)
}

func (uc *RecordingUsecase) Stop() error {
	return uc.recorder.Stop()
}

func (uc *RecordingUsecase) Status() core.RecordingSession {
	return uc.recorder.Status()
}
```

**Step 3: 验证编译**

```powershell
cd C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace
go vet ./projects/daq-t1603/services/api-go/...
```

**Step 4: 提交**

```bash
git add projects/daq-t1603/services/api-go/internal/usecase/
git commit -m "feat(daq-t1603): add usecase orchestration layer"
```

---

### Task 6: Wails 后端绑定层

**文件:**
- 创建: `projects/daq-t1603/apps/desktop-wails/backend/app.go`
- 创建: `projects/daq-t1603/apps/desktop-wails/backend/app_test.go`

**Step 1: 编写 app.go — Wails 绑定（薄层，无业务逻辑）**

```go
package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"daq-t1603/services/api-go/internal/core"
	"daq-t1603/services/api-go/internal/usecase"
)

type App struct {
	ctx       context.Context
	cancel    context.CancelFunc
	deviceUC  *usecase.DeviceUsecase
	recordUC  *usecase.RecordingUsecase

	streamMu      sync.Mutex
	streamCancel  context.CancelFunc
}

type GenericResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

func NewApp(deviceUC *usecase.DeviceUsecase, recordUC *usecase.RecordingUsecase) *App {
	return &App{
		deviceUC: deviceUC,
		recordUC: recordUC,
	}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx, a.cancel = context.WithCancel(ctx)
	log.Println("DAQ-T-1603 应用已启动")
}

func (a *App) Shutdown(ctx context.Context) {
	if a.cancel != nil {
		a.cancel()
	}
	log.Println("DAQ-T-1603 应用已关闭")
}

// ==================== 设备管理 API ====================

func (a *App) GetProfiles() string {
	profiles := a.deviceUC.GetProfiles()
	data, _ := json.Marshal(profiles)
	return string(data)
}

func (a *App) UpsertProfile(profileJSON string) GenericResponse {
	var profile core.TemperatureProfile
	if err := json.Unmarshal([]byte(profileJSON), &profile); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	if err := a.deviceUC.UpsertProfile(profile); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

func (a *App) DeleteProfile(id string) GenericResponse {
	if err := a.deviceUC.DeleteProfile(id); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

func (a *App) Connect(id string) GenericResponse {
	if err := a.deviceUC.Connect(id); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

func (a *App) Disconnect(id string) GenericResponse {
	if err := a.deviceUC.Disconnect(id); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

func (a *App) StartAcquisition(id string) GenericResponse {
	ch, err := a.deviceUC.StartAcquisition(id)
	if err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	go a.relayStream(id, ch)
	return GenericResponse{Success: true}
}

func (a *App) StopAcquisition(id string) GenericResponse {
	if err := a.deviceUC.StopAcquisition(id); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

func (a *App) GetStatus(id string) string {
	st, ok := a.deviceUC.GetStatus(id)
	if !ok {
		return "{}"
	}
	data, _ := json.Marshal(st)
	return string(data)
}

func (a *App) ApplyConfig(id string, configJSON string) GenericResponse {
	var cfg core.T1603Config
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	if err := a.deviceUC.ApplyConfig(id, cfg); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

// relayStream 将采集数据通过 Wails Events 推送到前端
func (a *App) relayStream(deviceID string, ch <-chan core.TemperatureSnapshot) {
	for snapshot := range ch {
		data, _ := json.Marshal(snapshot)
		runtime.EventsEmit(a.ctx, "daq:payload", string(data))
		// 如果正在录制，写入 CSV
		_ = a.recordUC.Write(snapshot)
	}
}

// ==================== 录制 API ====================

func (a *App) StartRecording(outputDir string, filePrefix string) GenericResponse {
	if err := a.recordUC.Start(outputDir, filePrefix); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

func (a *App) StopRecording() GenericResponse {
	if err := a.recordUC.Stop(); err != nil {
		return GenericResponse{Success: false, Error: err.Error()}
	}
	return GenericResponse{Success: true}
}

func (a *App) GetRecordingStatus() string {
	session := a.recordUC.Status()
	data, _ := json.Marshal(session)
	return string(data)
}

// ==================== 对话框 API ====================

func (a *App) PickDirectory() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:                "选择保存目录",
		CanCreateDirectories: true,
	})
}
```

**Step 2: 验证编译**

```powershell
cd C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace
go vet ./projects/daq-t1603/apps/desktop-wails/backend/...
```

**Step 3: 提交**

```bash
git add projects/daq-t1603/apps/desktop-wails/backend/
git commit -m "feat(daq-t1603): add Wails backend bindings"
```

---

### Task 7: Wails 入口 + Go 模块配置

**文件:**
- 创建: `projects/daq-t1603/apps/desktop-wails/main.go`
- 创建: `projects/daq-t1603/apps/desktop-wails/go.mod`
- 创建: `projects/daq-t1603/apps/desktop-wails/wails.json`

**Step 1: 编写 wails.json**

```json
{
  "$schema": "https://wails.io/schemas/config.v2.json",
  "name": "daq-t1603",
  "outputfilename": "DAQ-T-1603",
  "frontend:dir": "frontend",
  "frontend:install": "npm install --no-audit --no-fund",
  "frontend:build": "npm run build",
  "frontend:dev:watcher": "npm run dev",
  "frontend:dev:serverUrl": "http://localhost:5173",
  "author": {
    "name": "DAQ-T-1603 Team",
    "email": ""
  }
}
```

**Step 2: 编写 main.go — 依赖注入 + 启动**

```go
package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"daq-t1603/apps/desktop-wails/backend"
	"daq-t1603/services/api-go/internal/adapters/config"
	"daq-t1603/services/api-go/internal/adapters/hardware"
	"daq-t1603/services/api-go/internal/adapters/recording"
	"daq-t1603/services/api-go/internal/usecase"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// 依赖注入
	cfgStore := config.NewJSONConfigStore("data/device-profiles.json")
	devAdapter := hardware.NewT1603Adapter()
	recorder := recording.NewCSVRecorder()

	deviceUC := usecase.NewDeviceUsecase(devAdapter, cfgStore)
	recordUC := usecase.NewRecordingUsecase(recorder)

	app := backend.NewApp(deviceUC, recordUC)

	err := wails.Run(&options.App{
		Title:  "DAQ-T-1603 温度采集",
		Width:  1280,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.Startup,
		OnShutdown: app.Shutdown,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
```

**Step 3: 初始化 Go 模块**

```powershell
cd C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\projects\daq-t1603\apps\desktop-wails
go mod init daq-t1603/apps/desktop-wails
```

**Step 4: 更新 go.mod —— 添加 replace 和依赖**

需要添加的依赖：
- `github.com/wailsapp/wails/v2`
- replace `shared/device-sdk/go` → `../../../../shared/device-sdk/go`

参考 motion-controller 的 go.mod 写法：

```
module daq-t1603/apps/desktop-wails

go 1.25.0

require (
    github.com/wailsapp/wails/v2 v2.12.0
    shared/device-sdk/go v0.0.0
)

require (
    git.sr.ht/~jackmordaunt/go-toast/v2 v2.0.3 // indirect
    github.com/bep/debounce v1.2.1 // indirect
    github.com/go-ole/go-ole v1.3.0 // indirect
    github.com/godbus/dbus/v5 v5.1.0 // indirect
    github.com/google/uuid v1.6.0 // indirect
    github.com/gorilla/websocket v1.5.3 // indirect
    github.com/jchv/go-winloader v0.0.0-20210711035445-715c2860da7e // indirect
    github.com/labstack/echo/v4 v4.13.3 // indirect
    github.com/labstack/gommon v0.4.2 // indirect
    github.com/leaanthony/go-ansi-parser v1.6.1 // indirect
    github.com/leaanthony/gosod v1.0.4 // indirect
    github.com/leaanthony/slicer v1.6.0 // indirect
    github.com/leaanthony/u v1.1.1 // indirect
    github.com/mattn/go-colorable v0.1.13 // indirect
    github.com/mattn/go-isatty v0.0.20 // indirect
    github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
    github.com/pkg/errors v0.9.1 // indirect
    github.com/rivo/uniseg v0.4.7 // indirect
    github.com/samber/lo v1.49.1 // indirect
    github.com/tkrajina/go-reflector v0.5.8 // indirect
    github.com/valyala/bytebufferpool v1.0.0 // indirect
    github.com/valyala/fasttemplate v1.2.2 // indirect
    github.com/wailsapp/go-webview2 v1.0.22 // indirect
    github.com/wailsapp/mimetype v1.4.1 // indirect
    golang.org/x/crypto v0.33.0 // indirect
    golang.org/x/net v0.35.0 // indirect
    golang.org/x/sys v0.30.0 // indirect
    golang.org/x/text v0.22.0 // indirect
)

replace (
    daq-t1603/services/api-go => ../../services/api-go
    shared/device-sdk/go => ../../../../shared/device-sdk/go
)
```

**Step 5: 初始化 services/api-go 的 go.mod**

```powershell
cd C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\projects\daq-t1603\services\api-go
go mod init daq-t1603/services/api-go
```

在 go.mod 中添加：
```
require shared/device-sdk/go v0.0.0

replace shared/device-sdk/go => ../../../../shared/device-sdk/go
```

**Step 6: 安装前端依赖 + 验证 wails 编译**

```powershell
cd C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\projects\daq-t1603\apps\desktop-wails
go mod tidy -e
```

**Step 7: 提交**

```bash
git add projects/daq-t1603/apps/desktop-wails/main.go
git add projects/daq-t1603/apps/desktop-wails/go.mod
git add projects/daq-t1603/apps/desktop-wails/go.sum
git add projects/daq-t1603/apps/desktop-wails/wails.json
git add projects/daq-t1603/services/api-go/go.mod
git add projects/daq-t1603/services/api-go/go.sum
git add projects/daq-t1603/apps/desktop-wails/backend/
git commit -m "feat(daq-t1603): add Wails entry point and module config"
```

---

### Task 8: 前端布局壳

**文件:**
- 创建: `projects/daq-t1603/apps/desktop-wails/frontend/src/components/layout/AppShell.vue`
- 创建: `projects/daq-t1603/apps/desktop-wails/frontend/src/styles.css`
- 创建: `projects/daq-t1603/apps/desktop-wails/frontend/src/env.d.ts`
- 创建: `projects/daq-t1603/apps/desktop-wails/frontend/index.html`

**Step 1: 创建 AppShell.vue — 主布局**

```
+-----------------------------------------+
|  顶部工具栏 (设备名 + 连接/采集/配置/保存) |
+-----------------------------------------+
| 设备列表 |  主内容区                      |
| (左侧)   |  (波形图 + 数值网格)            |
|          |                               |
|          |                               |
+-----------------------------------------+
|  底部状态栏                              |
+-----------------------------------------+
```

```vue
<script setup lang="ts">
import DeviceList from '@components/device/DeviceList.vue'
import MonitorView from '@views/MonitorView.vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useRecordingStore } from '@stores/recordingStore'
import { ref, computed } from 'vue'
import { Activity, Play, Square, Settings, Download } from '@lucide/vue'

const deviceStore = useDeviceStore()
const recordingStore = useRecordingStore()
const showConfig = ref(false)

const selectedDeviceId = computed(() => deviceStore.selectedId)
const selectedStatus = computed(() =>
  selectedDeviceId.value ? deviceStore.statusFor(selectedDeviceId.value) : ''
)
const isAcquiring = computed(() =>
  selectedDeviceId.value ? deviceStore.acquiringFor(selectedDeviceId.value) : false
)

function handleConnect() {
  if (!selectedDeviceId.value) return
  if (selectedStatus.value === 'Connected' || selectedStatus.value === 'Acquiring') {
    deviceStore.disconnect(selectedDeviceId.value)
  } else {
    deviceStore.connect(selectedDeviceId.value)
  }
}

function handleAcquisition() {
  if (!selectedDeviceId.value) return
  if (isAcquiring.value) {
    deviceStore.stopAcquisition(selectedDeviceId.value)
  } else {
    deviceStore.startAcquisition(selectedDeviceId.value)
  }
}

async function handleSave() {
  const dir = await window.runtime.Call('PickDirectory')
  if (!dir) return
  recordingStore.startRecording(dir, 'DAQ-T1603')
}

function handleStopSave() {
  recordingStore.stopRecording()
}
</script>

<template>
  <div class="app-shell">
    <header class="app-shell__header">
      <div class="app-shell__brand">
        <Activity class="w-5 h-5 text-emerald-500" />
        <span class="app-shell__title">DAQ-T-1603 温度采集</span>
      </div>
      <div class="app-shell__toolbar">
        <button
          class="app-shell__btn"
          :class="{ 'app-shell__btn--active': selectedStatus === 'Connected' || selectedStatus === 'Acquiring' }"
          :disabled="!selectedDeviceId"
          @click="handleConnect"
        >
          {{ selectedStatus === 'Connected' || selectedStatus === 'Acquiring' ? '断开' : '连接' }}
        </button>
        <button
          class="app-shell__btn"
          :class="{ 'app-shell__btn--acq': !isAcquiring, 'app-shell__btn--stop': isAcquiring }"
          :disabled="selectedStatus !== 'Connected' && !isAcquiring"
          @click="handleAcquisition"
        >
          {{ isAcquiring ? '停止采集' : '开始采集' }}
        </button>
        <button
          class="app-shell__btn app-shell__btn--icon"
          :disabled="!selectedDeviceId"
          @click="showConfig = true"
        >
          <Settings class="w-4 h-4" />
        </button>
        <button
          v-if="!recordingStore.isRecording"
          class="app-shell__btn app-shell__btn--save"
          :disabled="!isAcquiring"
          @click="handleSave"
        >
          <Download class="w-4 h-4" />
          开始保存
        </button>
        <button
          v-else
          class="app-shell__btn app-shell__btn--stop"
          @click="handleStopSave"
        >
          <Square class="w-4 h-4" />
          停止保存
        </button>
      </div>
    </header>

    <div class="app-shell__body">
      <aside class="app-shell__sidebar">
        <DeviceList />
      </aside>
      <main class="app-shell__main">
        <MonitorView />
      </main>
    </div>

    <footer class="app-shell__footer">
      <span v-if="selectedDeviceId" class="app-shell__status">
        设备状态: {{ selectedStatus }}
      </span>
      <span v-if="isAcquiring" class="app-shell__status app-shell__status--acq">
        采集中
      </span>
      <span v-if="recordingStore.isRecording" class="app-shell__status app-shell__status--rec">
        录制中 · {{ recordingStore.snapshotCount }} 条
      </span>
    </footer>

    <!-- 配置弹窗 -->
    <Teleport to="body">
      <div v-if="showConfig" class="modal-overlay" @click.self="showConfig = false">
        <div class="modal-panel">
          <DaqT1603Config
            v-if="selectedDeviceId"
            :deviceId="selectedDeviceId"
            @close="showConfig = false"
          />
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.app-shell {
  display: flex;
  flex-direction: column;
  height: 100vh;
  background: var(--bg-primary, #0f172a);
  color: var(--text-primary, #e2e8f0);
}

.app-shell__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 1rem;
  height: 56px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(15, 23, 42, 0.95);
  backdrop-filter: blur(8px);
  flex-shrink: 0;
}

.app-shell__brand {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.app-shell__title {
  font-size: 1.1rem;
  font-weight: 800;
  letter-spacing: -0.02em;
}

.app-shell__toolbar {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.app-shell__btn {
  padding: 0.4rem 0.875rem;
  border-radius: 0.5rem;
  font-size: 0.8rem;
  font-weight: 700;
  background: rgba(255, 255, 255, 0.05);
  color: #94a3b8;
  border: 1px solid rgba(255, 255, 255, 0.1);
  cursor: pointer;
  transition: all 0.2s ease;
  display: flex;
  align-items: center;
  gap: 0.375rem;
}

.app-shell__btn:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.1);
  color: #e2e8f0;
}

.app-shell__btn:disabled {
  opacity: 0.3;
  cursor: not-allowed;
}

.app-shell__btn--active {
  background: rgba(16, 185, 129, 0.1);
  color: #10b981;
  border-color: rgba(16, 185, 129, 0.3);
}

.app-shell__btn--acq {
  background: #10b981;
  color: white;
  border-color: #10b981;
}

.app-shell__btn--stop {
  background: rgba(244, 63, 94, 0.1);
  color: #f43f5e;
  border-color: rgba(244, 63, 94, 0.3);
}

.app-shell__btn--save {
  background: rgba(59, 130, 246, 0.1);
  color: #3b82f6;
  border-color: rgba(59, 130, 246, 0.3);
}

.app-shell__btn--icon {
  padding: 0.4rem 0.5rem;
}

.app-shell__body {
  flex: 1;
  display: flex;
  min-height: 0;
}

.app-shell__sidebar {
  width: 220px;
  border-right: 1px solid rgba(255, 255, 255, 0.08);
  flex-shrink: 0;
  overflow-y: auto;
}

.app-shell__main {
  flex: 1;
  padding: 1rem;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.app-shell__footer {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 0 1rem;
  height: 32px;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
  font-size: 0.75rem;
  color: #64748b;
  flex-shrink: 0;
}

.app-shell__status--acq {
  color: #10b981;
}

.app-shell__status--rec {
  color: #f43f5e;
}

.modal-overlay {
  position: fixed;
  inset: 0;
  z-index: 50;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(4px);
}

.modal-panel {
  width: 100%;
  max-width: 32rem;
}
</style>
```

**Step 2: 创建前端入口文件**

创建 `index.html`:
```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>DAQ-T-1603 温度采集</title>
</head>
<body>
  <div id="app"></div>
  <script type="module" src="/src/main.ts"></script>
</body>
</html>
```

创建 `env.d.ts`:
```typescript
/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<{}, {}, any>
  export default component
}
```

**Step 3: 提交**

```bash
git add projects/daq-t1603/apps/desktop-wails/frontend/src/components/layout/
git add projects/daq-t1603/apps/desktop-wails/frontend/src/styles.css
git add projects/daq-t1603/apps/desktop-wails/frontend/src/env.d.ts
git add projects/daq-t1603/apps/desktop-wails/frontend/index.html
git commit -m "feat(daq-t1603): add frontend layout shell"
```

---

### Task 9: Pinia 状态管理

**文件:**
- 创建: `projects/daq-t1603/apps/desktop-wails/frontend/src/stores/deviceStore.ts`
- 创建: `projects/daq-t1603/apps/desktop-wails/frontend/src/stores/recordingStore.ts`

**Step 1: 编写 deviceStore.ts**

```typescript
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export interface ChannelConfig {
  index: number
  name: string
  enabled: boolean
  unit: string
  color: string
}

export interface T1603Config {
  thermocoupleType: string
  coldJunction: string
  filterHz: number
}

export interface TemperatureProfile {
  id: string
  name: string
  address: string
  port: number
  channels: ChannelConfig[]
  t1603Config: T1603Config
}

export interface TemperatureSnapshot {
  deviceId: string
  timestamp: number
  values: number[]
  unit: string
}

export interface DeviceState {
  profile: TemperatureProfile
  status: number
  error: string
}

const MAX_HISTORY = 200

const CHANNEL_COLORS = [
  '#3b82f6', '#10b981', '#f59e0b', '#a855f7',
  '#f43f5e', '#06b6d4', '#f97316', '#6366f1',
  '#84cc16', '#14b8a6', '#d946ef', '#0ea5e9',
  '#eab308', '#22c55e', '#ef4444', '#8b5cf6',
]

function defaultChannels(): ChannelConfig[] {
  return Array.from({ length: 16 }, (_, i) => ({
    index: i,
    name: `通道 ${i + 1}`,
    enabled: true,
    unit: '°C',
    color: CHANNEL_COLORS[i % CHANNEL_COLORS.length],
  }))
}

function defaultT1603Config(): T1603Config {
  return { thermocoupleType: 'K', coldJunction: 'internal', filterHz: 50 }
}

export const useDeviceStore = defineStore('device', () => {
  const profiles = ref<TemperatureProfile[]>([])
  const selectedId = ref<string | null>(null)
  const statusMap = ref<Record<string, string>>({})
  const historyMap = ref<Record<string, TemperatureSnapshot[]>>({})
  const snapshotMap = ref<Record<string, TemperatureSnapshot>>({})
  const chartSelections = ref<Record<string, Set<number>>>({})

  const selectedProfile = computed(() =>
    profiles.value.find((p) => p.id === selectedId.value) ?? null
  )
  const selectedSnapshot = computed(() =>
    selectedId.value ? snapshotMap.value[selectedId.value] ?? null : null
  )

  function statusFor(id: string): string {
    return statusMap.value[id] ?? 'Disconnected'
  }

  function acquiringFor(id: string): boolean {
    return statusMap.value[id] === 'Acquiring'
  }

  function historyFor(id: string): TemperatureSnapshot[] {
    return historyMap.value[id] ?? []
  }

  function isChartSelected(id: string, channelIndex: number): boolean {
    return chartSelections.value[id]?.has(channelIndex) ?? false
  }

  function toggleChartSelection(id: string, channelIndex: number): void {
    if (!chartSelections.value[id]) {
      chartSelections.value[id] = new Set()
    }
    const set = chartSelections.value[id]!
    if (set.has(channelIndex)) {
      set.delete(channelIndex)
    } else {
      set.add(channelIndex)
    }
  }

  function setProfiles(list: TemperatureProfile[]): void {
    profiles.value = list
  }

  function upsertProfile(profile: TemperatureProfile): void {
    const idx = profiles.value.findIndex((p) => p.id === profile.id)
    if (idx >= 0) {
      profiles.value[idx] = profile
    } else {
      profiles.value.push(profile)
    }
  }

  function deleteProfile(id: string): void {
    profiles.value = profiles.value.filter((p) => p.id !== id)
    if (selectedId.value === id) {
      selectedId.value = null
    }
  }

  function updateStatus(id: string, status: string): void {
    statusMap.value[id] = status
  }

  function pushSnapshot(snapshot: TemperatureSnapshot): void {
    snapshotMap.value[snapshot.deviceId] = snapshot
    if (!historyMap.value[snapshot.deviceId]) {
      historyMap.value[snapshot.deviceId] = []
    }
    const history = historyMap.value[snapshot.deviceId]!
    history.push(snapshot)
    if (history.length > MAX_HISTORY) {
      history.splice(0, history.length - MAX_HISTORY)
    }
  }

  async function loadProfiles(): Promise<void> {
    const raw = await window.runtime.Call('GetProfiles')
    try {
      const list = JSON.parse(raw as string)
      if (Array.isArray(list)) {
        setProfiles(list)
      }
    } catch { /* ignore */ }
  }

  async function connect(id: string): Promise<void> {
    const resp: { success: boolean; error?: string } = await window.runtime.Call('Connect', id)
    if (resp.success) updateStatus(id, 'Connected')
  }

  async function disconnect(id: string): Promise<void> {
    await window.runtime.Call('Disconnect', id)
    updateStatus(id, 'Disconnected')
  }

  async function startAcquisition(id: string): Promise<void> {
    const resp: { success: boolean; error?: string } = await window.runtime.Call('StartAcquisition', id)
    if (resp.success) updateStatus(id, 'Acquiring')
  }

  async function stopAcquisition(id: string): Promise<void> {
    await window.runtime.Call('StopAcquisition', id)
    updateStatus(id, 'Connected')
  }

  async function applyConfig(id: string, cfg: T1603Config): Promise<void> {
    await window.runtime.Call('ApplyConfig', id, JSON.stringify(cfg))
  }

  async function addProfile(name: string, address: string, port: number): Promise<void> {
    const id = `t1603_${Date.now()}`
    const profile: TemperatureProfile = {
      id,
      name,
      address,
      port,
      channels: defaultChannels(),
      t1603Config: defaultT1603Config(),
    }
    upsertProfile(profile)
    await window.runtime.Call('UpsertProfile', JSON.stringify(profile))
  }

  async function removeProfile(id: string): Promise<void> {
    await window.runtime.Call('DeleteProfile', id)
    deleteProfile(id)
  }

  return {
    profiles, selectedId, statusMap, historyMap, snapshotMap, chartSelections,
    selectedProfile, selectedSnapshot,
    statusFor, acquiringFor, historyFor, isChartSelected, toggleChartSelection,
    setProfiles, upsertProfile, deleteProfile, updateStatus, pushSnapshot,
    loadProfiles, connect, disconnect, startAcquisition, stopAcquisition,
    applyConfig, addProfile, removeProfile,
  }
})
```

**Step 2: 编写 recordingStore.ts**

```typescript
import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useRecordingStore = defineStore('recording', () => {
  const isRecording = ref(false)
  const outputDir = ref('')
  const filePrefix = ref('')
  const snapshotCount = ref(0)

  async function startRecording(dir: string, prefix: string): Promise<void> {
    const resp: { success: boolean; error?: string } =
      await window.runtime.Call('StartRecording', dir, prefix)
    if (resp.success) {
      isRecording.value = true
      outputDir.value = dir
      filePrefix.value = prefix
      snapshotCount.value = 0
    }
  }

  async function stopRecording(): Promise<void> {
    await window.runtime.Call('StopRecording')
    isRecording.value = false
  }

  async function refreshStatus(): Promise<void> {
    const raw = await window.runtime.Call('GetRecordingStatus')
    try {
      const session = JSON.parse(raw as string)
      isRecording.value = session.status === 1
      snapshotCount.value = session.snapshotCount ?? 0
    } catch { /* ignore */ }
  }

  return { isRecording, outputDir, filePrefix, snapshotCount, startRecording, stopRecording, refreshStatus }
})
```

**Step 3: 提交**

```bash
git add projects/daq-t1603/apps/desktop-wails/frontend/src/stores/
git commit -m "feat(daq-t1603): add Pinia stores for device and recording"
```

---

### Task 10: 设备列表组件

**文件:**
- 创建: `projects/daq-t1603/apps/desktop-wails/frontend/src/components/device/DeviceList.vue`

**Step 1: 编写 DeviceList.vue**

```vue
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { Plus, Trash2, Wifi, WifiOff } from '@lucide/vue'

const deviceStore = useDeviceStore()
const showAddDialog = ref(false)
const newName = ref('')
const newAddress = ref('192.168.3.101')
const newPort = ref(9000)

onMounted(() => {
  deviceStore.loadProfiles()
})

function select(id: string) {
  deviceStore.selectedId = id
}

async function addDevice() {
  if (!newName.value.trim()) return
  await deviceStore.addProfile(newName.value.trim(), newAddress.value.trim(), newPort.value)
  showAddDialog.value = false
  newName.value = ''
}

async function removeDevice(id: string, event: MouseEvent) {
  event.stopPropagation()
  await deviceStore.removeProfile(id)
}
</script>

<template>
  <div class="device-list">
    <div class="device-list__header">
      <span class="device-list__title">设备列表</span>
      <button class="device-list__add" @click="showAddDialog = true">
        <Plus class="w-4 h-4" />
      </button>
    </div>
    <div class="device-list__items">
      <div
        v-for="profile in deviceStore.profiles"
        :key="profile.id"
        class="device-list__item"
        :class="{ 'device-list__item--active': deviceStore.selectedId === profile.id }"
        @click="select(profile.id)"
      >
        <div class="device-list__item-icon">
          <Wifi v-if="deviceStore.statusFor(profile.id) !== 'Disconnected'" class="w-4 h-4 text-emerald-500" />
          <WifiOff v-else class="w-4 h-4 text-slate-500" />
        </div>
        <div class="device-list__item-info">
          <span class="device-list__item-name">{{ profile.name }}</span>
          <span class="device-list__item-addr">{{ profile.address }}:{{ profile.port }}</span>
        </div>
        <button class="device-list__item-remove" @click="removeDevice(profile.id, $event)">
          <Trash2 class="w-3.5 h-3.5" />
        </button>
      </div>
      <div v-if="deviceStore.profiles.length === 0" class="device-list__empty">
        暂无设备，点击 + 添加
      </div>
    </div>

    <Teleport to="body">
      <div v-if="showAddDialog" class="add-dialog-overlay" @click.self="showAddDialog = false">
        <div class="add-dialog">
          <h3 class="add-dialog__title">添加设备</h3>
          <div class="add-dialog__field">
            <label>设备名称</label>
            <input v-model="newName" placeholder="例如: 温度采集器1" />
          </div>
          <div class="add-dialog__field">
            <label>IP 地址</label>
            <input v-model="newAddress" placeholder="192.168.3.101" />
          </div>
          <div class="add-dialog__field">
            <label>端口</label>
            <input v-model.number="newPort" type="number" placeholder="9000" />
          </div>
          <div class="add-dialog__actions">
            <button class="add-dialog__btn add-dialog__btn--secondary" @click="showAddDialog = false">取消</button>
            <button class="add-dialog__btn add-dialog__btn--primary" @click="addDevice">添加</button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.device-list {
  padding: 0.75rem;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.device-list__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.device-list__title {
  font-size: 0.75rem;
  font-weight: 700;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.device-list__add {
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.375rem;
  background: rgba(16, 185, 129, 0.1);
  color: #10b981;
  border: 1px solid rgba(16, 185, 129, 0.2);
  cursor: pointer;
  transition: all 0.2s ease;
}

.device-list__add:hover {
  background: rgba(16, 185, 129, 0.2);
}

.device-list__items {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.device-list__item {
  display: flex;
  align-items: center;
  gap: 0.625rem;
  padding: 0.5rem 0.625rem;
  border-radius: 0.5rem;
  cursor: pointer;
  transition: all 0.2s ease;
  position: relative;
}

.device-list__item:hover {
  background: rgba(255, 255, 255, 0.05);
}

.device-list__item--active {
  background: rgba(16, 185, 129, 0.1);
  border: 1px solid rgba(16, 185, 129, 0.2);
}

.device-list__item-icon {
  flex-shrink: 0;
}

.device-list__item-info {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.device-list__item-name {
  font-size: 0.85rem;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.device-list__item-addr {
  font-size: 0.7rem;
  color: #64748b;
}

.device-list__item-remove {
  opacity: 0;
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.25rem;
  color: #f43f5e;
  background: rgba(244, 63, 94, 0.1);
  transition: all 0.2s ease;
}

.device-list__item:hover .device-list__item-remove {
  opacity: 1;
}

.device-list__empty {
  padding: 1rem;
  text-align: center;
  color: #64748b;
  font-size: 0.8rem;
}

.add-dialog-overlay {
  position: fixed;
  inset: 0;
  z-index: 50;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(4px);
}

.add-dialog {
  width: 100%;
  max-width: 24rem;
  background: rgba(30, 41, 59, 0.95);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 1rem;
  padding: 1.5rem;
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.add-dialog__title {
  font-size: 1.1rem;
  font-weight: 700;
  margin: 0;
}

.add-dialog__field {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
}

.add-dialog__field label {
  font