package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"shared.local/device-sdk/go/motion/adapters/hardware"
	"shared.local/device-sdk/go/motion/core"
	motionports "shared.local/device-sdk/go/motion/ports"
	motionmanager "shared.local/motion-control/go/manager"
	motionprofile "shared.local/motion-control/go/profile"

	"windlabx4/services/api-go/api"
	windaqconfig "windlabx4/services/api-go/internal/adapters/config"
	storageadapter "windlabx4/services/api-go/internal/adapters/storage"
	"windlabx4/services/api-go/internal/core/device"
	windaqports "windlabx4/services/api-go/internal/ports"
	"windlabx4/services/api-go/internal/usecase"
	"windlabx4/services/api-go/pkg/appcontext"
	"windlabx4/services/api-go/pkg/wiring"
)

type apiProfileStore struct {
	profiles []device.Profile
}

func (s *apiProfileStore) LoadProfiles() ([]device.Profile, error) {
	return append([]device.Profile(nil), s.profiles...), nil
}

func (s *apiProfileStore) SaveProfiles(profiles []device.Profile) error {
	s.profiles = append([]device.Profile(nil), profiles...)
	return nil
}

type apiDeviceFactory struct{}

func (apiDeviceFactory) Create(profile device.Profile) (windaqports.Device, error) {
	// Return a simulated device that implements the Device interface
	return &simulatedDevice{profile: profile}, nil
}

type calibrationDeviceFactory struct {
	device *simulatedDevice
}

func (f *calibrationDeviceFactory) Create(profile device.Profile) (windaqports.Device, error) {
	f.device = &simulatedDevice{profile: profile}
	return f.device, nil
}

type simulatedDevice struct {
	profile   device.Profile
	dataSink  device.DataSink
	connected bool
	acquiring bool
}

func (d *simulatedDevice) ID() string        { return d.profile.ID }
func (d *simulatedDevice) Connect() error    { d.connected = true; return nil }
func (d *simulatedDevice) Disconnect() error { d.connected = false; d.acquiring = false; return nil }
func (d *simulatedDevice) StartAcquisition() error {
	d.acquiring = true
	if d.dataSink != nil {
		d.dataSink(device.DataPayload{DeviceID: d.profile.ID, Timestamp: device.NowMs(), Channels: []float64{1}, ChannelIndices: []int{0}})
	}
	return nil
}
func (d *simulatedDevice) StopAcquisition() error { d.acquiring = false; return nil }
func (d *simulatedDevice) Status() device.Status {
	connection := device.ConnectionDisconnected
	if d.acquiring {
		connection = device.ConnectionAcquiring
	} else if d.connected {
		connection = device.ConnectionConnected
	}
	return device.Status{ID: d.profile.ID, Name: d.profile.Name, Type: d.profile.Type, Connection: connection, Acquiring: d.acquiring}
}
func (d *simulatedDevice) SetDataSink(sink device.DataSink) { d.dataSink = sink }

type apiPublisher struct{}

func (apiPublisher) Publish(string, any) {}

func newTestMotionManager(axisNames ...core.AxisName) (windaqports.MotionManager, *motionmanager.MotionManager) {
	profileStore := motionprofile.NewMemoryMotionProfileStore()
	axes := make([]core.AxisConfig, len(axisNames))
	for i, name := range axisNames {
		speed := 10.0
		axes[i] = core.AxisConfig{Name: name, Enabled: true, Kind: core.AxisKindLinear, MaxSpeed: &speed}
	}
	profiles := []core.MotionControllerProfile{
		{
			ID:      "test-motion",
			Name:    "Test Controller",
			Type:    core.ControllerTypeSimulated,
			Address: "127.0.0.1",
			Port:    9000,
			Axes:    axes,
		},
	}
	profileStore.SaveProfiles(profiles)
	rawMgr := motionmanager.NewMotionManager(profileStore, func(profile core.MotionControllerProfile) (motionports.MotionController, error) {
		return hardware.NewSimulatedMotionController(profile), nil
	})
	rawMgr.LoadProfiles()
	return wiring.WrapMotionManager(rawMgr), rawMgr
}

func TestDeviceAcquisitionHTTPFlow(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	manager, err := usecase.NewDeviceManager(&apiProfileStore{}, apiDeviceFactory{}, hub.OnData)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	router := api.NewRouter(api.Deps{DeviceManager: manager, AcquisitionHub: hub})

	profile := windaqconfig.NewDefaultProfile("sim-1", device.DeviceSimulated)
	profileBody, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	request(t, router, http.MethodPut, "/api/device/profiles", profileBody, http.StatusOK)
	request(t, router, http.MethodPost, "/api/device/sim-1/connect", nil, http.StatusOK)
	request(t, router, http.MethodPost, "/api/device/sim-1/startAcquisition", nil, http.StatusOK)
	defer request(t, router, http.MethodPost, "/api/device/sim-1/stopAcquisition", nil, http.StatusOK)

	deadline := time.After(700 * time.Millisecond)
	for {
		resp := request(t, router, http.MethodGet, "/api/daq/latest/sim-1", nil, http.StatusOK)
		var payload device.DataPayload
		if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode latest response: %v", err)
		}
		if len(payload.Channels) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for latest data")
		case <-time.After(20 * time.Millisecond):
		}
	}

	resp := request(t, router, http.MethodGet, "/api/device/sim-1/status", nil, http.StatusOK)
	var status device.Status
	if err := json.Unmarshal(resp.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if !status.Acquiring {
		t.Fatalf("expected acquiring status, got %#v", status)
	}
}

func TestDualTraversalRoutesUseIsolatedRealRegistryManagers(t *testing.T) {
	configStore := windaqconfig.NewFileAppConfigStore(t.TempDir())
	bundle, err := appcontext.NewTraversalRegistry(appcontext.TraversalRegistryDeps{
		ConfigStore:     configStore,
		CheckpointStore: storageadapter.NewFileCheckpointStore(),
		DataDir:         t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewTraversalRegistry: %v", err)
	}
	router := api.NewRouter(api.Deps{TraversalRegistry: bundle.Registry})

	probe1 := []byte(`{"probeType":"five-hole","motionAxes":[{"controllerId":"ctrl-a","axis":"X"}]}`)
	probe2 := []byte(`{"probeType":"seven-hole","motionAxes":[{"controllerId":"ctrl-b","axis":"Y"}]}`)
	request(t, router, http.MethodPost, "/api/traversal/probe1/config", probe1, http.StatusOK)
	request(t, router, http.MethodPost, "/api/traversal/probe2/config", probe2, http.StatusOK)

	got1 := request(t, router, http.MethodGet, "/api/traversal/probe1/config", nil, http.StatusOK)
	got2 := request(t, router, http.MethodGet, "/api/traversal/probe2/config", nil, http.StatusOK)
	if got1.Body.String() == got2.Body.String() {
		t.Fatal("probe-scoped routes must not share manager configuration")
	}
	if !strings.Contains(got1.Body.String(), "five-hole") || !strings.Contains(got2.Body.String(), "seven-hole") {
		t.Fatalf("unexpected isolated configs: probe1=%s probe2=%s", got1.Body.String(), got2.Body.String())
	}
}

func TestDeviceDisconnectAndDeleteProfileHTTPFlow(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	store := &apiProfileStore{}
	manager, err := usecase.NewDeviceManager(store, apiDeviceFactory{}, hub.OnData)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	router := api.NewRouter(api.Deps{DeviceManager: manager, AcquisitionHub: hub})

	profile := windaqconfig.NewDefaultProfile("sim-1", device.DeviceSimulated)
	profileBody, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	request(t, router, http.MethodPut, "/api/device/profiles", profileBody, http.StatusOK)
	request(t, router, http.MethodPost, "/api/device/sim-1/connect", nil, http.StatusOK)
	request(t, router, http.MethodPost, "/api/device/sim-1/disconnect", nil, http.StatusOK)
	request(t, router, http.MethodGet, "/api/device/sim-1/status", nil, http.StatusNotFound)

	request(t, router, http.MethodPost, "/api/device/sim-1/connect", nil, http.StatusOK)
	request(t, router, http.MethodDelete, "/api/device/profiles/sim-1", nil, http.StatusOK)
	if len(store.profiles) != 0 {
		t.Fatalf("expected profile store to be empty, got %d", len(store.profiles))
	}
	request(t, router, http.MethodGet, "/api/device/sim-1/status", nil, http.StatusNotFound)
}

// TestDaqLatestReturns404WhenDeviceNotConnected 验证：设备从 DeviceManager 删除后
// （异常退出或主动断开），/api/daq/latest/{id} 必须返回 404，让前端轮询能感知
// 断连并更新 UI 状态。
//
// 此前该接口在设备不存在时返回 200 + 空 payload，导致前端轮询静默吞掉，
// UI 永远显示"采集中"——拔网线后用户无法感知断连。
func TestDaqLatestReturns404WhenDeviceNotConnected(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	store := &apiProfileStore{}
	manager, err := usecase.NewDeviceManager(store, apiDeviceFactory{}, hub.OnData)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	router := api.NewRouter(api.Deps{DeviceManager: manager, AcquisitionHub: hub})

	// 未连接时：404
	request(t, router, http.MethodGet, "/api/daq/latest/sim-1", nil, http.StatusNotFound)

	// 连接后：200（可能空 payload，因为还没出第一帧）
	profileBody, err := json.Marshal(windaqconfig.NewDefaultProfile("sim-1", device.DeviceSimulated))
	if err != nil {
		t.Fatal(err)
	}
	request(t, router, http.MethodPut, "/api/device/profiles", profileBody, http.StatusOK)
	request(t, router, http.MethodPost, "/api/device/sim-1/connect", nil, http.StatusOK)
	request(t, router, http.MethodGet, "/api/daq/latest/sim-1", nil, http.StatusOK)

	// 断开后：404（核心断言）
	request(t, router, http.MethodPost, "/api/device/sim-1/disconnect", nil, http.StatusOK)
	request(t, router, http.MethodGet, "/api/daq/latest/sim-1", nil, http.StatusNotFound)
}

func TestDeviceScanHTTPFlow(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	manager, err := usecase.NewDeviceManager(&apiProfileStore{}, apiDeviceFactory{}, hub.OnData)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	manager.SetScanner(apiScanner{results: []device.ScanResult{
		{ID: "sim-1", Name: "Simulator 1", Type: device.DeviceSimulated, Available: true},
	}})
	router := api.NewRouter(api.Deps{DeviceManager: manager, AcquisitionHub: hub})

	resp := request(t, router, http.MethodGet, "/api/device/scan", nil, http.StatusOK)
	var results []device.ScanResult
	if err := json.Unmarshal(resp.Body.Bytes(), &results); err != nil {
		t.Fatalf("decode scan response: %v", err)
	}
	if len(results) != 1 || results[0].ID != "sim-1" {
		t.Fatalf("unexpected scan results: %+v", results)
	}
}

func TestDeviceConfigurationHTTPFlow(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	store := &apiProfileStore{}
	manager, err := usecase.NewDeviceManager(store, apiDeviceFactory{}, hub.OnData)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	router := api.NewRouter(api.Deps{DeviceManager: manager, AcquisitionHub: hub})

	profile := windaqconfig.NewDefaultProfile("temp-1", device.DeviceDaqT1603)
	profileBody, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	request(t, router, http.MethodPut, "/api/device/profiles", profileBody, http.StatusOK)

	request(t, router, http.MethodPut, "/api/device/temp-1/unit", []byte(`{"unit":"degC"}`), http.StatusOK)
	if store.profiles[0].Channels[0].Unit != "degC" {
		t.Fatalf("expected persisted unit degC, got %q", store.profiles[0].Channels[0].Unit)
	}

	request(t, router, http.MethodPut, "/api/device/temp-1/daqT1603Config", []byte(`{"thermocoupleTypes":"KKKKKKKKKKKKKKKK","channelMask":"FFFF","samplingRate":10,"averageCount":4}`), http.StatusOK)
	resp := request(t, router, http.MethodGet, "/api/device/temp-1/daqT1603Config", nil, http.StatusOK)
	var got device.DaqT1603HardwareConfig
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode daqT1603 config response: %v", err)
	}
	if got.ThermocoupleTypes != "KKKKKKKKKKKKKKKK" || got.SamplingRate != 10 || got.ChannelMask != "FFFF" {
		t.Fatalf("unexpected daqT1603 config: %+v", got)
	}

	request(t, router, http.MethodGet, "/api/v1/devices/temp-1/calibration?channelIndex=not-a-number", nil, http.StatusBadRequest)
	request(t, router, http.MethodPost, "/api/v1/devices/temp-1/clearCalibration?channelIndex=-1", nil, http.StatusBadRequest)
}

// TestDaqT1602ConfigHTTPFlow 验证 DAQ-T-1602 配置端点：
// PUT 应用并持久化 → GET 回读一致；非法类型码被拒绝且不污染 profile。
func TestDaqT1602ConfigHTTPFlow(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	store := &apiProfileStore{}
	manager, err := usecase.NewDeviceManager(store, apiDeviceFactory{}, hub.OnData)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	router := api.NewRouter(api.Deps{DeviceManager: manager, AcquisitionHub: hub})

	profile := windaqconfig.NewDefaultProfile("temp-t1602", device.DeviceDaqT1602)
	profileBody, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	request(t, router, http.MethodPut, "/api/device/profiles", profileBody, http.StatusOK)

	// 默认配置：16 通道全 T 型（type code 2）
	resp := request(t, router, http.MethodGet, "/api/device/temp-t1602/daqT1602Config", nil, http.StatusOK)
	var defaults device.DaqT1602HardwareConfig
	if err := json.Unmarshal(resp.Body.Bytes(), &defaults); err != nil {
		t.Fatalf("decode daqT1602 config response: %v", err)
	}
	for i, code := range defaults.TypeCodes {
		if code != 2 {
			t.Fatalf("expected default typeCode 2 (T) at channel %d, got %d", i, code)
		}
	}

	// 应用自定义配置（卡2 CH0 改为 E 型）
	custom := defaults
	custom.TypeCodes[8] = 3
	customBody, err := json.Marshal(custom)
	if err != nil {
		t.Fatal(err)
	}
	request(t, router, http.MethodPut, "/api/device/temp-t1602/daqT1602Config", customBody, http.StatusOK)

	resp = request(t, router, http.MethodGet, "/api/device/temp-t1602/daqT1602Config", nil, http.StatusOK)
	var got device.DaqT1602HardwareConfig
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode daqT1602 config response: %v", err)
	}
	if got != custom {
		t.Fatalf("unexpected daqT1602 config: %+v", got)
	}
	if store.profiles[0].DaqT1602Config != custom {
		t.Fatalf("expected persisted config %+v, got %+v", custom, store.profiles[0].DaqT1602Config)
	}

	// 非法类型码（>7）→ 400
	invalid := custom
	invalid.TypeCodes[0] = 9
	invalidBody, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	request(t, router, http.MethodPut, "/api/device/temp-t1602/daqT1602Config", invalidBody, http.StatusBadRequest)
}

func TestDeviceCalibrationRequiresAcquisition(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	store := &apiProfileStore{profiles: []device.Profile{windaqconfig.NewDefaultProfile("pressure-1", device.DeviceDAQP1604)}}
	manager, err := usecase.NewDeviceManagerWithNormalizer(store, apiDeviceFactory{}, nil, windaqconfig.NewProfileNormalizer())
	if err != nil {
		t.Fatal(err)
	}
	usecase.AssembleDataSinkWithCalibration(hub, nil, manager, 20*time.Millisecond)
	router := api.NewRouter(api.Deps{DeviceManager: manager, AcquisitionHub: hub})

	request(t, router, http.MethodPut, "/api/device/pressure-1/calibrate", nil, http.StatusBadRequest)
}

func TestDeviceCalibrationHTTPFlow(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	profile := windaqconfig.NewDefaultProfile("pressure-1", device.DeviceDAQP1604)
	profile.Channels = profile.Channels[:1]
	store := &apiProfileStore{profiles: []device.Profile{profile}}
	factory := &calibrationDeviceFactory{}
	manager, err := usecase.NewDeviceManagerWithNormalizer(store, factory, nil, windaqconfig.NewProfileNormalizer())
	if err != nil {
		t.Fatal(err)
	}
	usecase.AssembleDataSinkWithCalibration(hub, nil, manager, 30*time.Millisecond)
	router := api.NewRouter(api.Deps{DeviceManager: manager, AcquisitionHub: hub})

	request(t, router, http.MethodPost, "/api/device/pressure-1/connect", nil, http.StatusOK)
	request(t, router, http.MethodPost, "/api/device/pressure-1/startAcquisition", nil, http.StatusOK)
	stopFrames := make(chan struct{})
	defer close(stopFrames)
	go func() {
		ticker := time.NewTicker(2 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopFrames:
				return
			case <-ticker.C:
				factory.device.dataSink(device.DataPayload{DeviceID: "pressure-1", Timestamp: device.NowMs(), Channels: []float64{1}, ChannelIndices: []int{0}})
			}
		}
	}()

	resp := request(t, router, http.MethodPut, "/api/v1/devices/pressure-1/calibrate?channelIndex=0", nil, http.StatusOK)
	var calibrated struct {
		Success bool                       `json:"success"`
		Data    []device.CalibrationResult `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &calibrated); err != nil {
		t.Fatal(err)
	}
	if !calibrated.Success || len(calibrated.Data) != 1 || calibrated.Data[0].Offset != 1 {
		t.Fatalf("unexpected calibration response: %+v", calibrated)
	}

	recordResp := request(t, router, http.MethodGet, "/api/device/pressure-1/calibration?channelIndex=0", nil, http.StatusOK)
	var record device.CalibrationRecord
	if err := json.Unmarshal(recordResp.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record.Offset != 1 || record.Unit != "Pa" || record.At == 0 {
		t.Fatalf("unexpected calibration record: %+v", record)
	}

	request(t, router, http.MethodPost, "/api/device/pressure-1/clearCalibration?channelIndex=0", nil, http.StatusOK)
	recordResp = request(t, router, http.MethodGet, "/api/device/pressure-1/calibration?channelIndex=0", nil, http.StatusOK)
	if err := json.Unmarshal(recordResp.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record.Offset != 0 {
		t.Fatalf("expected cleared calibration, got %+v", record)
	}
}

func TestDaqStreamSendsServerSentEvents(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	manager, err := usecase.NewDeviceManager(&apiProfileStore{}, apiDeviceFactory{}, hub.OnData)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	router := api.NewRouter(api.Deps{DeviceManager: manager, AcquisitionHub: hub})

	// 使用真实 httptest.Server + HTTP 客户端读取 SSE 流：
	// httptest.ResponseRecorder 不是并发安全的，handler goroutine 写入与主 goroutine 读取会造成 race。
	server := httptest.NewServer(router)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/daq/stream/dev-1", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %q", ct)
	}

	hub.OnData(device.DataPayload{DeviceID: "dev-1", Timestamp: 123, Channels: []float64{42}, ChannelIndices: []int{0}})

	// 按 SSE 帧边界读取，直到看到 payload 事件
	buf := make([]byte, 0, 1024)
	tmp := make([]byte, 256)
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for SSE payload, buf=%q", string(buf))
		default:
		}
		n, err := resp.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			if strings.Contains(string(buf), "event: payload") && strings.Contains(string(buf), `"deviceId":"dev-1"`) {
				return
			}
		}
		if err != nil {
			t.Fatalf("read SSE body: %v (buf=%q)", err, string(buf))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestMotionHTTPFlow(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	motionMgr, rawMotionMgr := newTestMotionManager(core.AxisX, core.AxisY, core.AxisZ)
	router := api.NewRouter(api.Deps{
		DeviceManager:  newTestDeviceManager(t, hub),
		AcquisitionHub: hub,
		MotionManager:  motionMgr,
		MotionService:  rawMotionMgr,
	})

	request(t, router, http.MethodPost, "/api/motion/connect", []byte(`{"id":"test-motion"}`), http.StatusOK)
	resp := request(t, router, http.MethodGet, "/api/motion/status", nil, http.StatusOK)
	var statuses []struct {
		Connected bool              `json:"connected"`
		Axes      []json.RawMessage `json:"axes"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &statuses); err != nil {
		t.Fatalf("decode motion status: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 controller status, got %d", len(statuses))
	}
	if !statuses[0].Connected {
		t.Fatal("expected connected after connect")
	}
	if len(statuses[0].Axes) != 3 {
		t.Fatalf("expected 3 axes, got %d", len(statuses[0].Axes))
	}

	request(t, router, http.MethodPost, "/api/motion/moveTo", []byte(`{"id":"test-motion","axis":"X","position":50}`), http.StatusOK)
	request(t, router, http.MethodPost, "/api/motion/jog", []byte(`{"id":"test-motion","axis":"Y","velocity":2}`), http.StatusOK)
	resp2 := request(t, router, http.MethodGet, "/api/motion/status", nil, http.StatusOK)
	if strings.Contains(resp2.Body.String(), `"moving":true`) {
		request(t, router, http.MethodPost, "/api/motion/stop", []byte(`{"id":"test-motion","axis":"Y"}`), http.StatusOK)
	}
	request(t, router, http.MethodPost, "/api/motion/emergencyStop", []byte(`{"id":"test-motion"}`), http.StatusOK)
	request(t, router, http.MethodPost, "/api/motion/disconnect", []byte(`{"id":"test-motion"}`), http.StatusOK)
}

func TestMotionHTTPValidation(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	motionMgr, rawMotionMgr := newTestMotionManager(core.AxisX)
	router := api.NewRouter(api.Deps{
		DeviceManager:  newTestDeviceManager(t, hub),
		AcquisitionHub: hub,
		MotionManager:  motionMgr,
		MotionService:  rawMotionMgr,
	})

	request(t, router, http.MethodGet, "/api/motion/connect", nil, http.StatusMethodNotAllowed)
	request(t, router, http.MethodPost, "/api/motion/status", nil, http.StatusMethodNotAllowed)
	request(t, router, http.MethodPost, "/api/motion/moveTo", []byte(`{"axis":"","position":0}`), http.StatusBadRequest)
	request(t, router, http.MethodPost, "/api/motion/jog", []byte(`{"axis":"","velocity":0}`), http.StatusBadRequest)
}

func TestCalibrationHTTPFlow(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	motionMgr, rawMotionMgr := newTestMotionManager(core.AxisX, core.AxisY, core.AxisZ)
	calMgr := usecase.NewCalibrationManager(hub, motionMgr, nil, nil)
	router := api.NewRouter(api.Deps{
		DeviceManager:      newTestDeviceManager(t, hub),
		AcquisitionHub:     hub,
		MotionManager:      motionMgr,
		MotionService:      rawMotionMgr,
		CalibrationManager: calMgr,
	})

	request(t, router, http.MethodGet, "/api/calibration/status", nil, http.StatusOK)
	request(t, router, http.MethodPost, "/api/calibration/start", []byte(`{"taskId":"cal-1","deviceId":"sim-1","type":"five-hole","channels":[0,1],"pressurePoints":[0,50,100],"averageSamples":5}`), http.StatusOK)
	resp := request(t, router, http.MethodGet, "/api/calibration/status", nil, http.StatusOK)
	var st struct {
		State       string `json:"state"`
		TotalPoints int    `json:"totalPoints"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode calibration status: %v", err)
	}
	if st.State != "running" {
		t.Fatalf("expected running state, got %q", st.State)
	}
	if st.TotalPoints != 3 {
		t.Fatalf("expected 3 total points, got %d", st.TotalPoints)
	}

	request(t, router, http.MethodPost, "/api/calibration/pause", nil, http.StatusOK)
	resp2 := request(t, router, http.MethodGet, "/api/calibration/status", nil, http.StatusOK)
	if !strings.Contains(resp2.Body.String(), `"paused"`) {
		t.Fatalf("expected paused state, got %s", resp2.Body.String())
	}

	request(t, router, http.MethodPost, "/api/calibration/resume", nil, http.StatusOK)
	resp3 := request(t, router, http.MethodGet, "/api/calibration/status", nil, http.StatusOK)
	if !strings.Contains(resp3.Body.String(), `"running"`) {
		t.Fatalf("expected running after resume, got %s", resp3.Body.String())
	}

	request(t, router, http.MethodPost, "/api/calibration/collect", nil, http.StatusBadRequest)
	request(t, router, http.MethodPost, "/api/calibration/stop", nil, http.StatusOK)
}

func TestCalibrationHTTPValidation(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	calMgr := usecase.NewCalibrationManager(hub, nil, nil, nil)
	router := api.NewRouter(api.Deps{
		DeviceManager:      newTestDeviceManager(t, hub),
		AcquisitionHub:     hub,
		MotionManager:      wiring.NewMotionManager(nil, nil),
		CalibrationManager: calMgr,
	})

	request(t, router, http.MethodPost, "/api/calibration/start", []byte(`{}`), http.StatusBadRequest)
	request(t, router, http.MethodPost, "/api/calibration/start", []byte(`{"taskId":"cal-1","deviceId":"sim-1","channels":[0],"pressurePoints":[0],"averageSamples":1}`), http.StatusBadRequest)
	request(t, router, http.MethodGet, "/api/calibration/nonexistent", nil, http.StatusNotFound)
}

func TestTraversalHTTPFlow(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	motionMgr, rawMotionMgr := newTestMotionManager(core.AxisX, core.AxisY, core.AxisZ)
	travMgr := usecase.NewTraversalManager(hub, motionMgr, nil, nil, nil)
	router := api.NewRouter(api.Deps{
		DeviceManager:    newTestDeviceManager(t, hub),
		AcquisitionHub:   hub,
		MotionManager:    motionMgr,
		MotionService:    rawMotionMgr,
		TraversalManager: travMgr,
	})

	request(t, router, http.MethodGet, "/api/traversal/status", nil, http.StatusOK)
	gridResp := request(t, router, http.MethodPost, "/api/traversal/generateGrid", []byte(`{"xStart":0,"xEnd":10,"xStep":10,"yStart":0,"yEnd":0,"yStep":1,"zStart":5}`), http.StatusOK)
	var generatedPath []struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		Z float64 `json:"z"`
	}
	if err := json.Unmarshal(gridResp.Body.Bytes(), &generatedPath); err != nil {
		t.Fatalf("decode generated traversal path: %v", err)
	}
	if len(generatedPath) != 2 || generatedPath[0].Z != 5 {
		t.Fatalf("unexpected generated path: %+v", generatedPath)
	}

	pathPayload := []byte(`{"taskId":"trav-1","deviceId":"sim-1","channels":[0],"path":[{"x":0,"y":0,"z":0},{"x":50,"y":25,"z":10}]}`)
	request(t, router, http.MethodPost, "/api/traversal/start", pathPayload, http.StatusOK)
	resp := request(t, router, http.MethodGet, "/api/traversal/status", nil, http.StatusOK)
	var st struct {
		State       string `json:"state"`
		TotalPoints int    `json:"totalPoints"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode traversal status: %v", err)
	}
	if st.State != "running" {
		t.Fatalf("expected running state, got %q", st.State)
	}
	if st.TotalPoints != 2 {
		t.Fatalf("expected 2 total points, got %d", st.TotalPoints)
	}

	request(t, router, http.MethodPost, "/api/traversal/pause", nil, http.StatusOK)
	request(t, router, http.MethodPost, "/api/traversal/resume", nil, http.StatusOK)
	request(t, router, http.MethodPost, "/api/traversal/stop", nil, http.StatusOK)
}

func TestTraversalHTTPValidation(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	travMgr := usecase.NewTraversalManager(hub, nil, nil, nil, nil)
	router := api.NewRouter(api.Deps{
		DeviceManager:    newTestDeviceManager(t, hub),
		AcquisitionHub:   hub,
		MotionManager:    wiring.NewMotionManager(nil, nil),
		TraversalManager: travMgr,
	})

	request(t, router, http.MethodPost, "/api/traversal/start", []byte(`{}`), http.StatusBadRequest)
	request(t, router, http.MethodPost, "/api/traversal/start", []byte(`{"taskId":"trav-1","deviceId":"sim-1","channels":[],"path":[]}`), http.StatusBadRequest)
}

func TestStorageHTTPFlow(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	recorder := usecase.NewStorageRecorder(nil)
	router := api.NewRouter(api.Deps{
		DeviceManager:   newTestDeviceManager(t, hub),
		AcquisitionHub:  hub,
		StorageRecorder: recorder,
	})

	request(t, router, http.MethodGet, "/api/storage/status", nil, http.StatusOK)
	request(t, router, http.MethodPost, "/api/storage/start", []byte(`{"outputDir":"testdata","filePrefix":"run-1"}`), http.StatusBadRequest)
	request(t, router, http.MethodPost, "/api/storage/start", []byte(`{}`), http.StatusBadRequest)
	request(t, router, http.MethodPost, "/api/storage/stop", nil, http.StatusOK)
}

func TestPublishRateHTTPFlow(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	router := api.NewRouter(api.Deps{
		DeviceManager:   newTestDeviceManager(t, hub),
		AcquisitionHub:  hub,
		StorageRecorder: usecase.NewStorageRecorder(nil),
	})

	request(t, router, http.MethodGet, "/api/daq/publishRate", nil, http.StatusOK)
	request(t, router, http.MethodPut, "/api/daq/publishRate", []byte(`{"hz":10}`), http.StatusOK)
	request(t, router, http.MethodPut, "/api/daq/publishRate", []byte(`{"hz":0}`), http.StatusBadRequest)
}

func TestReportHTTPFlow(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	reportMgr := usecase.NewReportManager(nil)
	router := api.NewRouter(api.Deps{
		DeviceManager:  newTestDeviceManager(t, hub),
		AcquisitionHub: hub,
		ReportManager:  reportMgr,
	})

	request(t, router, http.MethodGet, "/api/report/status", nil, http.StatusOK)
	request(t, router, http.MethodPost, "/api/report/generate", []byte(`{}`), http.StatusBadRequest)
	request(t, router, http.MethodPost, "/api/report/generate", []byte(`{"outputDir":"/tmp","filePrefix":"test","deviceId":"sim-1"}`), http.StatusBadRequest)
}

func newTestDeviceManager(t *testing.T, hub *usecase.AcquisitionHub) *usecase.DeviceManager {
	t.Helper()
	mgr, err := usecase.NewDeviceManager(&apiProfileStore{}, apiDeviceFactory{}, hub.OnData)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	return mgr
}

type apiScanner struct {
	results []device.ScanResult
}

func (s apiScanner) Scan() ([]device.ScanResult, error) {
	return append([]device.ScanResult(nil), s.results...), nil
}

func request(t *testing.T, handler http.Handler, method string, path string, body []byte, wantStatus int) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != wantStatus {
		t.Fatalf("%s %s: expected status %d, got %d body=%s", method, path, wantStatus, resp.Code, resp.Body.String())
	}
	return resp
}
