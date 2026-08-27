package deviceconnect_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"cal1604/internal/application/deviceconnect"
	"cal1604/internal/device"
	"cal1604/internal/device/manager"
	"cal1604/internal/domain"
)

type publishRecord struct {
	Statuses []domain.DeviceStatus
	Payloads []domain.Device
}

func (r *publishRecord) Publish(dev domain.Device) {
	r.Statuses = append(r.Statuses, dev.Status)
	r.Payloads = append(r.Payloads, dev)
}

type fakeConnectionDriver struct {
	connectErrors []error
	connectCalls  int

	disconnectErrors []error
	disconnectCalls  int
}

func (d *fakeConnectionDriver) Connect(_ context.Context) error {
	idx := d.connectCalls
	d.connectCalls++
	if idx < len(d.connectErrors) {
		return d.connectErrors[idx]
	}
	return nil
}

func (d *fakeConnectionDriver) Disconnect(_ context.Context) error {
	idx := d.disconnectCalls
	d.disconnectCalls++
	if idx < len(d.disconnectErrors) {
		return d.disconnectErrors[idx]
	}
	return nil
}

type fakeDriverFactory struct {
	drivers map[string]device.ConnectionDriver
}

func (f *fakeDriverFactory) Create(dev domain.Device) (device.ConnectionDriver, error) {
	driver, ok := f.drivers[dev.ID]
	if !ok {
		return nil, errors.New("missing fake driver")
	}
	return driver, nil
}

func TestConnectRetriesWithBackoffUntilSuccess(t *testing.T) {
	deviceStore := manager.NewDeviceManager()
	deviceStore.Upsert(domain.Device{
		ID:     "m1",
		Type:   domain.DeviceTypeMeasure,
		Model:  "WTN1604",
		Host:   "127.0.0.1",
		Port:   9000,
		Unit:   "kPa",
		Status: domain.DeviceStatusDisconnected,
	})

	stubDriver := &fakeConnectionDriver{
		connectErrors: []error{
			errors.New("dial timeout #1"),
			errors.New("dial timeout #2"),
		},
	}

	publisher := &publishRecord{}
	var backoffSteps []time.Duration

	service := deviceconnect.NewService(
		deviceStore,
		&fakeDriverFactory{drivers: map[string]device.ConnectionDriver{"m1": stubDriver}},
		deviceconnect.Config{
			ConnectAttemptTimeout:    50 * time.Millisecond,
			ConnectMaxAttempts:       3,
			ConnectInitialBackoff:    20 * time.Millisecond,
			ConnectMaxBackoff:        40 * time.Millisecond,
			DisconnectAttemptTimeout: 50 * time.Millisecond,
			DisconnectMaxAttempts:    1,
		},
		publisher.Publish,
		deviceconnect.WithSleepFunc(func(_ context.Context, delay time.Duration) error {
			backoffSteps = append(backoffSteps, delay)
			return nil
		}),
	)

	updated, err := service.Connect(context.Background(), "m1")
	if err != nil {
		t.Fatalf("connect returned unexpected error: %v", err)
	}

	if updated.Status != domain.DeviceStatusConnected {
		t.Fatalf("expected connected status, got %s", updated.Status)
	}

	if stubDriver.connectCalls != 3 {
		t.Fatalf("expected 3 connect attempts, got %d", stubDriver.connectCalls)
	}

	if len(backoffSteps) != 2 {
		t.Fatalf("expected 2 backoff sleeps, got %d", len(backoffSteps))
	}

	if backoffSteps[0] != 20*time.Millisecond || backoffSteps[1] != 40*time.Millisecond {
		t.Fatalf("unexpected backoff sequence: %v", backoffSteps)
	}

	if len(publisher.Statuses) != 2 {
		t.Fatalf("expected two status publish events, got %d", len(publisher.Statuses))
	}

	if publisher.Statuses[0] != domain.DeviceStatusConnecting || publisher.Statuses[1] != domain.DeviceStatusConnected {
		t.Fatalf("unexpected status publish order: %v", publisher.Statuses)
	}
}

func TestConnectMarksDeviceErrorAndRecordsReason(t *testing.T) {
	deviceStore := manager.NewDeviceManager()
	deviceStore.Upsert(domain.Device{
		ID:     "p1",
		Type:   domain.DeviceTypePressure,
		Model:  "ConST 811A",
		Host:   "127.0.0.1",
		Port:   7000,
		Unit:   "kPa",
		Status: domain.DeviceStatusDisconnected,
	})

	stubDriver := &fakeConnectionDriver{
		connectErrors: []error{
			errors.New("network timeout"),
			errors.New("network timeout"),
			errors.New("network timeout"),
		},
	}

	publisher := &publishRecord{}
	fixedTime := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)

	service := deviceconnect.NewService(
		deviceStore,
		&fakeDriverFactory{drivers: map[string]device.ConnectionDriver{"p1": stubDriver}},
		deviceconnect.Config{
			ConnectAttemptTimeout:    50 * time.Millisecond,
			ConnectMaxAttempts:       3,
			ConnectInitialBackoff:    10 * time.Millisecond,
			ConnectMaxBackoff:        20 * time.Millisecond,
			DisconnectAttemptTimeout: 50 * time.Millisecond,
			DisconnectMaxAttempts:    1,
		},
		publisher.Publish,
		deviceconnect.WithNowFunc(func() time.Time {
			return fixedTime
		}),
		deviceconnect.WithSleepFunc(func(_ context.Context, _ time.Duration) error {
			return nil
		}),
	)

	updated, err := service.Connect(context.Background(), "p1")
	if err == nil {
		t.Fatal("expected connect error, got nil")
	}

	if updated.Status != domain.DeviceStatusError {
		t.Fatalf("expected error status, got %s", updated.Status)
	}

	if !strings.Contains(updated.LastErrorReason, "network timeout") {
		t.Fatalf("expected network timeout in reason, got %q", updated.LastErrorReason)
	}

	if updated.LastErrorAt == nil || !updated.LastErrorAt.Equal(fixedTime) {
		t.Fatalf("expected fixed error time %s, got %+v", fixedTime.Format(time.RFC3339), updated.LastErrorAt)
	}

	if len(publisher.Statuses) != 2 {
		t.Fatalf("expected two status events, got %d", len(publisher.Statuses))
	}

	if publisher.Statuses[1] != domain.DeviceStatusError {
		t.Fatalf("expected final status event to be error, got %s", publisher.Statuses[1])
	}

	lastPayload := publisher.Payloads[len(publisher.Payloads)-1]
	if lastPayload.LastErrorAt == nil || !lastPayload.LastErrorAt.Equal(fixedTime) {
		t.Fatalf("expected published error time %s, got %+v", fixedTime.Format(time.RFC3339), lastPayload.LastErrorAt)
	}
}
