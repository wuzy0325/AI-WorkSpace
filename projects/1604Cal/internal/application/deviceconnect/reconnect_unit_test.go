package deviceconnect_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"cal1604/internal/application/deviceconnect"
	"cal1604/internal/device"
	"cal1604/internal/device/manager"
	"cal1604/internal/domain"
)

// flakyUnitDriver 模拟 WTN1604 重连后的真实行为：
// ReadUnit 失败（读超时/对端关闭）会把底层连接置为损坏，
// 必须重新建链后才能继续通信。
type flakyUnitDriver struct {
	fakeConnectionDriver

	// readUnitResults 依次返回每次 ReadUnit 的结果，耗尽后一直成功。
	readUnitResults []error
	readUnitCalls   int

	// readUnitFailuresClosingConn 为 true 时，读取失败会关闭底层连接
	// （与 tcp_base.sendWTN1604Command 的 closeConn 行为一致）。
	readUnitFailuresClosingConn bool

	connAlive bool
	unit      string
}

func (d *flakyUnitDriver) Connect(_ context.Context) error {
	idx := d.connectCalls
	d.connectCalls++
	if idx < len(d.connectErrors) {
		return d.connectErrors[idx]
	}
	d.connAlive = true
	return nil
}

func (d *flakyUnitDriver) Disconnect(_ context.Context) error {
	d.connAlive = false
	return nil
}

func (d *flakyUnitDriver) ReadUnit(_ context.Context) (string, error) {
	idx := d.readUnitCalls
	d.readUnitCalls++
	if idx < len(d.readUnitResults) {
		if err := d.readUnitResults[idx]; err != nil {
			if d.readUnitFailuresClosingConn {
				d.connAlive = false
			}
			return "", err
		}
	}
	return d.unit, nil
}

func newFlakyUnitTestDevice(t *testing.T) (*manager.DeviceManager, *flakyUnitDriver) {
	t.Helper()
	deviceStore := manager.NewDeviceManager()
	deviceStore.Upsert(domain.Device{
		ID:     "m1",
		Type:   domain.DeviceTypeMeasure,
		Model:  "WTN1604",
		Host:   "127.0.0.1",
		Port:   9000,
		Status: domain.DeviceStatusDisconnected,
	})
	return deviceStore, &flakyUnitDriver{unit: "kPa"}
}

func newFlakyUnitService(deviceStore *manager.DeviceManager, drv device.ConnectionDriver) *deviceconnect.Service {
	return deviceconnect.NewService(
		deviceStore,
		&fakeDriverFactory{drivers: map[string]device.ConnectionDriver{"m1": drv}},
		deviceconnect.Config{
			ConnectAttemptTimeout:    50 * time.Millisecond,
			ConnectMaxAttempts:       1,
			DisconnectAttemptTimeout: 50 * time.Millisecond,
			DisconnectMaxAttempts:    1,
		},
		func(domain.Device) {},
		deviceconnect.WithSleepFunc(func(context.Context, time.Duration) error { return nil }),
	)
}

// 场景：重连建链后首次读单位失败且失败伴随连接损坏（现场常见），
// 服务应重新建链并重试读取，最终设备已连接、单位同步、链路可用。
func TestConnectRecoversDeadConnAfterReadUnitFailure(t *testing.T) {
	deviceStore, drv := newFlakyUnitTestDevice(t)
	drv.readUnitResults = []error{errors.New("read length prefix: timeout")}
	drv.readUnitFailuresClosingConn = true

	service := newFlakyUnitService(deviceStore, drv)
	updated, err := service.Connect(context.Background(), "m1")
	if err != nil {
		t.Fatalf("connect returned unexpected error: %v", err)
	}

	if updated.Status != domain.DeviceStatusConnected {
		t.Fatalf("expected connected status, got %s", updated.Status)
	}
	if updated.Unit != "kPa" {
		t.Fatalf("expected unit synced from hardware, got %q", updated.Unit)
	}
	// 首次 Connect + 读失败后的重建 = 2 次；此时链路必须存活
	if drv.connectCalls != 2 {
		t.Fatalf("expected 2 connect calls (initial + recovery), got %d", drv.connectCalls)
	}
	if !drv.connAlive {
		t.Fatal("expected driver connection to be alive after recovery")
	}
}

// 场景：单位始终读取失败（设备持续不应答），设备仍应标记为已连接，
// 且服务必须做最后一次重建链路，保证后续命令不会全部报 not connected。
func TestConnectKeepsAliveConnWhenReadUnitAlwaysFails(t *testing.T) {
	deviceStore, drv := newFlakyUnitTestDevice(t)
	drv.readUnitResults = []error{
		errors.New("read timeout #1"),
		errors.New("read timeout #2"),
		errors.New("read timeout #3"),
	}
	drv.readUnitFailuresClosingConn = true

	service := newFlakyUnitService(deviceStore, drv)
	updated, err := service.Connect(context.Background(), "m1")
	if err != nil {
		t.Fatalf("connect returned unexpected error: %v", err)
	}

	if updated.Status != domain.DeviceStatusConnected {
		t.Fatalf("expected connected status despite unit read failure, got %s", updated.Status)
	}
	// 首次 + 2 次重试读取前重建 + 兜底重建 = 4 次
	if drv.connectCalls != 4 {
		t.Fatalf("expected 4 connect calls (initial + 2 retries + final keepalive), got %d", drv.connectCalls)
	}
	if !drv.connAlive {
		t.Fatal("expected final reconnect to leave a live connection")
	}
}

// 场景：重连后读取一次即成功（最常见路径），不应有多余的重连调用。
func TestConnectSingleReadSuccessDoesNotReconnect(t *testing.T) {
	deviceStore, drv := newFlakyUnitTestDevice(t)

	service := newFlakyUnitService(deviceStore, drv)
	updated, err := service.Connect(context.Background(), "m1")
	if err != nil {
		t.Fatalf("connect returned unexpected error: %v", err)
	}

	if updated.Unit != "kPa" {
		t.Fatalf("expected unit kPa, got %q", updated.Unit)
	}
	if drv.connectCalls != 1 {
		t.Fatalf("expected exactly 1 connect call, got %d", drv.connectCalls)
	}
}

// 场景：读取失败后的重连自身失败（如设备离线），不应把错误上抛为连接失败——
// TCP 建链本身是成功的，设备状态保持已连接由上层轮询决定后续处理。
// 兜底重建仍会再试一次，若成功则恢复链路。
func TestConnectSwallowsReconnectFailureAfterReadFailure(t *testing.T) {
	deviceStore, drv := newFlakyUnitTestDevice(t)
	drv.readUnitResults = []error{errors.New("read timeout")}
	drv.readUnitFailuresClosingConn = true
	// 连接调用序列：#1 首次成功；#2 读取失败后重建失败；#3 兜底重建成功
	drv.connectErrors = []error{nil, errors.New("dial timeout during recovery")}

	service := newFlakyUnitService(deviceStore, drv)
	updated, err := service.Connect(context.Background(), "m1")
	if err != nil {
		t.Fatalf("expected no error even when recovery connect fails, got %v", err)
	}
	if updated.Status != domain.DeviceStatusConnected {
		t.Fatalf("expected connected status, got %s", updated.Status)
	}
	if drv.connectCalls != 3 {
		t.Fatalf("expected 3 connect calls (initial + recovery + final), got %d", drv.connectCalls)
	}
	if !drv.connAlive {
		t.Fatal("expected final keepalive reconnect to restore the connection")
	}
}
