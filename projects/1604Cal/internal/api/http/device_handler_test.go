package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cal1604/internal/api/dto"
	"cal1604/internal/application/deviceconnect"
	"cal1604/internal/events"
	"cal1604/internal/device"
	"cal1604/internal/device/manager"
	"cal1604/internal/domain"
)

type unitConsistencyResponse struct {
	Consistent bool     `json:"consistent"`
	Conflicts  []string `json:"conflicts"`
}

type setDeviceStatusPayload struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func TestGetDevicesReturnsEmptyListAtStart(t *testing.T) {
	router := NewRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp dto.Response[[]domain.Device]
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !resp.Success {
		t.Fatalf("expected success response, got %+v", resp)
	}

	if len(resp.Data) != 0 {
		t.Fatalf("expected empty list, got %d items", len(resp.Data))
	}
}

func TestUpdateDeviceStatus(t *testing.T) {
	router := NewRouter()

	addPayload := map[string]any{
		"id":    "p1",
		"name":  "pressure-1",
		"type":  "pressure",
		"model": "ConST 811A",
		"host":  "192.168.1.101",
		"port":  7000,
		"unit":  "kPa",
	}
	addBody, err := json.Marshal(addPayload)
	if err != nil {
		t.Fatalf("marshal add payload: %v", err)
	}

	addReq := httptest.NewRequest(http.MethodPost, "/api/v1/devices", bytes.NewReader(addBody))
	addReq.Header.Set("Content-Type", "application/json")
	addRec := httptest.NewRecorder()
	router.ServeHTTP(addRec, addReq)
	if addRec.Code != http.StatusOK {
		t.Fatalf("expected add status 200, got %d", addRec.Code)
	}

	statusReqBody, err := json.Marshal(setDeviceStatusPayload{ID: "p1", Status: "error"})
	if err != nil {
		t.Fatalf("marshal status payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/status", bytes.NewReader(statusReqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)

	var listResp dto.Response[[]domain.Device]
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}

	if got := listResp.Data[0].Status; got != "error" {
		t.Fatalf("expected status error, got %s", got)
	}
}

func TestPostDeviceThenList(t *testing.T) {
	router := NewRouter()

	payload := map[string]any{
		"id":    "m1",
		"name":  "measure-1",
		"type":  "measure",
		"model": "WTN1604",
		"host":  "192.168.1.100",
		"port":  9000,
		"unit":  "kPa",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/devices", bytes.NewReader(body))
	postReq.Header.Set("Content-Type", "application/json")
	postRec := httptest.NewRecorder()
	router.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusOK {
		t.Fatalf("expected post status 200, got %d", postRec.Code)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)

	var resp dto.Response[[]domain.Device]
	if err := json.NewDecoder(listRec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 device, got %d", len(resp.Data))
	}

	if resp.Data[0].ID != "m1" {
		t.Fatalf("expected id m1, got %s", resp.Data[0].ID)
	}
}

func TestUnitConsistencyCheck(t *testing.T) {
	// CheckUnitConsistency 只检查已连接设备，需要通过 UpdateStatus 设置为 connected
	store := manager.NewDeviceManager()
	store.Upsert(domain.Device{
		ID: "m1", Name: "m1", Type: domain.DeviceTypeMeasure, Model: "mock",
		Host: "192.168.1.200", Port: 9000, Unit: "kPa",
		Status: domain.DeviceStatusConnected,
	})
	store.Upsert(domain.Device{
		ID: "p1", Name: "p1", Type: domain.DeviceTypePressure, Model: "mock",
		Host: "192.168.1.201", Port: 9001, Unit: "psi",
		Status: domain.DeviceStatusConnected,
	})
	router := NewRouterWithDependencies(store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/checks/unit-consistency", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp dto.Response[unitConsistencyResponse]
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Data.Consistent {
		t.Fatal("expected unit consistency to be false")
	}

	if len(resp.Data.Conflicts) == 0 {
		t.Fatal("expected conflicts to be returned")
	}
}

func TestConnectAndDisconnectDeviceEndpoints(t *testing.T) {
	router := NewRouter()

	addPayload := map[string]any{
		"id":    "p2",
		"name":  "pressure-2",
		"type":  "pressure",
		"model": "ConST 820",
		"host":  "192.168.1.102",
		"port":  7002,
		"unit":  "kPa",
	}
	addBody, err := json.Marshal(addPayload)
	if err != nil {
		t.Fatalf("marshal add payload: %v", err)
	}
	addReq := httptest.NewRequest(http.MethodPost, "/api/v1/devices", bytes.NewReader(addBody))
	addReq.Header.Set("Content-Type", "application/json")
	addRec := httptest.NewRecorder()
	router.ServeHTTP(addRec, addReq)
	if addRec.Code != http.StatusOK {
		t.Fatalf("expected add status 200, got %d", addRec.Code)
	}

	connectReq := httptest.NewRequest(http.MethodPost, "/api/v1/devices/connect", bytes.NewReader([]byte(`{"id":"p2"}`)))
	connectReq.Header.Set("Content-Type", "application/json")
	connectRec := httptest.NewRecorder()
	router.ServeHTTP(connectRec, connectReq)

	if connectRec.Code != http.StatusOK {
		t.Fatalf("expected connect status 200, got %d", connectRec.Code)
	}

	disconnectReq := httptest.NewRequest(http.MethodPost, "/api/v1/devices/disconnect", bytes.NewReader([]byte(`{"id":"p2"}`)))
	disconnectReq.Header.Set("Content-Type", "application/json")
	disconnectRec := httptest.NewRecorder()
	router.ServeHTTP(disconnectRec, disconnectReq)

	if disconnectRec.Code != http.StatusOK {
		t.Fatalf("expected disconnect status 200, got %d", disconnectRec.Code)
	}
}

func TestConnectEndpointReturnsErrorReasonAndStreamsSSE(t *testing.T) {
	deviceStore := manager.NewDeviceManager()
	deviceStore.Upsert(domain.Device{
		ID:     "p3",
		Name:   "pressure-3",
		Type:   domain.DeviceTypePressure,
		Model:  "ConST 811A",
		Host:   "127.0.0.1",
		Port:   7003,
		Unit:   "kPa",
		Status: domain.DeviceStatusDisconnected,
	})

	publisherServer := &apiServer{}
	connector := deviceconnect.NewService(
		deviceStore,
		&handlerFakeDriverFactory{drivers: map[string]device.ConnectionDriver{
			"p3": &handlerFakeConnectionDriver{
				connectErrors: []error{
					errors.New("tcp timeout"),
					errors.New("tcp timeout"),
				},
			},
		}},
		deviceconnect.Config{
			ConnectAttemptTimeout:    50 * time.Millisecond,
			ConnectMaxAttempts:       2,
			ConnectInitialBackoff:    10 * time.Millisecond,
			ConnectMaxBackoff:        20 * time.Millisecond,
			DisconnectAttemptTimeout: 50 * time.Millisecond,
			DisconnectMaxAttempts:    1,
		},
		publisherServer.publishDeviceStatusChanged,
		deviceconnect.WithNowFunc(func() time.Time {
			return time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
		}),
		deviceconnect.WithSleepFunc(func(_ context.Context, _ time.Duration) error {
			return nil
		}),
	)

	router := NewRouterWithDependencies(deviceStore, connector)

	streamCtx, cancelStream := context.WithCancel(context.Background())
	defer cancelStream()

	streamReq := httptest.NewRequest(http.MethodGet, "/api/v1/events/stream", nil).WithContext(streamCtx)
	streamRec := httptest.NewRecorder()
	streamDone := make(chan struct{})
	go func() {
		router.ServeHTTP(streamRec, streamReq)
		close(streamDone)
	}()
	time.Sleep(20 * time.Millisecond)

	connectReq := httptest.NewRequest(http.MethodPost, "/api/v1/devices/connect", bytes.NewReader([]byte(`{"id":"p3"}`)))
	connectReq.Header.Set("Content-Type", "application/json")
	connectRec := httptest.NewRecorder()
	router.ServeHTTP(connectRec, connectReq)

	if connectRec.Code != http.StatusOK {
		t.Fatalf("expected connect status 200, got %d", connectRec.Code)
	}

	var resp dto.Response[domain.Device]
	if err := json.NewDecoder(connectRec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode connect response: %v", err)
	}

	if resp.Data.Status != domain.DeviceStatusError {
		t.Fatalf("expected device status error, got %s", resp.Data.Status)
	}

	if !strings.Contains(resp.Data.LastErrorReason, "tcp timeout") {
		t.Fatalf("expected error reason to include tcp timeout, got %q", resp.Data.LastErrorReason)
	}

	if resp.Data.LastErrorAt == nil {
		t.Fatal("expected lastErrorAt to be populated")
	}

	time.Sleep(40 * time.Millisecond)
	cancelStream()

	select {
	case <-streamDone:
	case <-time.After(time.Second):
		t.Fatal("sse stream did not exit after cancellation")
	}

	body := streamRec.Body.String()
	if !strings.Contains(body, events.EventDeviceStatusChanged) {
		t.Fatalf("expected status changed event in stream body, got %q", body)
	}

	if !strings.Contains(body, "errorReason") || !strings.Contains(body, "tcp timeout") {
		t.Fatalf("expected stream body to include error reason payload, got %q", body)
	}
}

func TestPostDeviceRejectsInvalidHostAndPort(t *testing.T) {
	router := NewRouter()

	invalidHostPayload := map[string]any{
		"id":    "m-ip",
		"name":  "bad-ip",
		"type":  "measure",
		"model": "WTN1604",
		"host":  "invalid-ip-text",
		"port":  9000,
		"unit":  "kPa",
	}
	hostBody, err := json.Marshal(invalidHostPayload)
	if err != nil {
		t.Fatalf("marshal invalid host payload: %v", err)
	}

	hostReq := httptest.NewRequest(http.MethodPost, "/api/v1/devices", bytes.NewReader(hostBody))
	hostReq.Header.Set("Content-Type", "application/json")
	hostRec := httptest.NewRecorder()
	router.ServeHTTP(hostRec, hostReq)

	if hostRec.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request for invalid host, got %d", hostRec.Code)
	}

	invalidPortPayload := map[string]any{
		"id":    "m-port",
		"name":  "bad-port",
		"type":  "measure",
		"model": "WTN1604",
		"host":  "192.168.1.120",
		"port":  70000,
		"unit":  "kPa",
	}
	portBody, err := json.Marshal(invalidPortPayload)
	if err != nil {
		t.Fatalf("marshal invalid port payload: %v", err)
	}

	portReq := httptest.NewRequest(http.MethodPost, "/api/v1/devices", bytes.NewReader(portBody))
	portReq.Header.Set("Content-Type", "application/json")
	portRec := httptest.NewRecorder()
	router.ServeHTTP(portRec, portReq)

	if portRec.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request for invalid port, got %d", portRec.Code)
	}
}

type handlerFakeConnectionDriver struct {
	connectErrors []error
	connectCalls  int

	disconnectErrors []error
	disconnectCalls  int
}

func (d *handlerFakeConnectionDriver) Connect(_ context.Context) error {
	idx := d.connectCalls
	d.connectCalls++
	if idx < len(d.connectErrors) {
		return d.connectErrors[idx]
	}
	return nil
}

func (d *handlerFakeConnectionDriver) Disconnect(_ context.Context) error {
	idx := d.disconnectCalls
	d.disconnectCalls++
	if idx < len(d.disconnectErrors) {
		return d.disconnectErrors[idx]
	}
	return nil
}

type handlerFakeDriverFactory struct {
	drivers map[string]device.ConnectionDriver
}

func (f *handlerFakeDriverFactory) Create(dev domain.Device) (device.ConnectionDriver, error) {
	drv, ok := f.drivers[dev.ID]
	if !ok {
		return nil, errors.New("fake handler driver not found")
	}
	return drv, nil
}

// TestMergeTareOffsets 验证编辑设备配置时不会把校零偏移清空。
func TestMergeTareOffsets(t *testing.T) {
	old := domain.Device{
		ID: "m1",
		Channels: []domain.ChannelConfig{
			{Index: 1, TareOffset: 1.5}, // 已校零
			{Index: 2, TareOffset: 0},   // 未校零
			{Index: 3, TareOffset: -2.0},
		},
	}
	// 模拟前端 DTO 保存：通道无 tareOffset（全 0）。
	newDev := domain.Device{
		ID: "m1",
		Channels: []domain.ChannelConfig{
			{Index: 1, TareOffset: 0},
			{Index: 2, TareOffset: 0},
			{Index: 3, TareOffset: 0},
		},
	}

	mergeTareOffsets(&newDev, old)

	if newDev.Channels[0].TareOffset != 1.5 {
		t.Fatalf("expected ch1 offset kept 1.5, got %v", newDev.Channels[0].TareOffset)
	}
	if newDev.Channels[1].TareOffset != 0 {
		t.Fatalf("expected ch2 offset 0, got %v", newDev.Channels[1].TareOffset)
	}
	if newDev.Channels[2].TareOffset != -2.0 {
		t.Fatalf("expected ch3 offset kept -2.0, got %v", newDev.Channels[2].TareOffset)
	}
}
