package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"cal1604/internal/application/deviceconnect"
	"cal1604/internal/config"
)

func TestLoadFromFileOverridesDeviceConnectConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "app.json")

	content := `{
		"deviceConnect": {
			"connectAttemptTimeoutMs": 1500,
			"connectMaxAttempts": 5,
			"connectInitialBackoffMs": 100,
			"connectMaxBackoffMs": 450,
			"disconnectAttemptTimeoutMs": 900,
			"disconnectMaxAttempts": 4,
			"disconnectInitialBackoffMs": 60,
			"disconnectMaxBackoffMs": 220
		}
	}`

	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	appCfg, err := config.LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	got := appCfg.ToDeviceConnectConfig()
	if got.ConnectAttemptTimeout != 1500*time.Millisecond {
		t.Fatalf("expected connect timeout 1500ms, got %s", got.ConnectAttemptTimeout)
	}
	if got.ConnectMaxAttempts != 5 {
		t.Fatalf("expected connect attempts 5, got %d", got.ConnectMaxAttempts)
	}
	if got.ConnectInitialBackoff != 100*time.Millisecond {
		t.Fatalf("expected connect initial backoff 100ms, got %s", got.ConnectInitialBackoff)
	}
	if got.ConnectMaxBackoff != 450*time.Millisecond {
		t.Fatalf("expected connect max backoff 450ms, got %s", got.ConnectMaxBackoff)
	}
	if got.DisconnectAttemptTimeout != 900*time.Millisecond {
		t.Fatalf("expected disconnect timeout 900ms, got %s", got.DisconnectAttemptTimeout)
	}
	if got.DisconnectMaxAttempts != 4 {
		t.Fatalf("expected disconnect attempts 4, got %d", got.DisconnectMaxAttempts)
	}
	if got.DisconnectInitialBackoff != 60*time.Millisecond {
		t.Fatalf("expected disconnect initial backoff 60ms, got %s", got.DisconnectInitialBackoff)
	}
	if got.DisconnectMaxBackoff != 220*time.Millisecond {
		t.Fatalf("expected disconnect max backoff 220ms, got %s", got.DisconnectMaxBackoff)
	}
}

func TestToDeviceConnectConfigFallsBackToDefaultsOnInvalidValues(t *testing.T) {
	appCfg := config.AppConfig{
		DeviceConnect: config.DeviceConnectFileConfig{
			ConnectAttemptTimeoutMs:    0,
			ConnectMaxAttempts:         -1,
			ConnectInitialBackoffMs:    -10,
			ConnectMaxBackoffMs:        -10,
			DisconnectAttemptTimeoutMs: 0,
			DisconnectMaxAttempts:      -3,
			DisconnectInitialBackoffMs: -20,
			DisconnectMaxBackoffMs:     -20,
		},
	}

	got := appCfg.ToDeviceConnectConfig()
	want := deviceconnect.DefaultConfig()

	if got != want {
		t.Fatalf("expected fallback to default config, got %+v", got)
	}
}

func TestDefaultCalibrationConfigEnablesValveGate(t *testing.T) {
	appCfg := config.Default()
	if !appCfg.Calibration.EnforceValveCalibrationGate {
		t.Fatalf("expected valve gate enabled by default (valve=calibration is required for both calibration and measurement startup)")
	}
}

func TestLoadFromFileOverridesCalibrationValveGate(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "app.json")

	content := `{
		"calibration": {
			"enforceValveCalibrationGate": false
		}
	}`

	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	appCfg, err := config.LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if appCfg.Calibration.EnforceValveCalibrationGate {
		t.Fatalf("expected valve gate disabled from config file")
	}
}

func TestDefaultMeasurementParamsIndependentFromCalibrationParams(t *testing.T) {
	appCfg := config.Default()

	if appCfg.CalibrationParams.PointCount == appCfg.MeasurementParams.PointCount {
		t.Fatalf("expected independent defaults, got both pointCount=%d", appCfg.CalibrationParams.PointCount)
	}
}

func TestLoadFromFileOverridesMeasurementParamsIndependently(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "app.json")

	content := `{
		"measurementParams": {
			"minPressure": 1,
			"maxPressure": 99,
			"pointCount": 12,
			"precision": 3,
			"averageCount": 4,
			"stableDurationMs": 8000,
			"precisionLevel": 0.2,
			"pressureMode": "roundTrip",
			"controlMode": "manual"
		}
	}`

	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	appCfg, err := config.LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if appCfg.MeasurementParams.PointCount != 12 {
		t.Fatalf("expected measurement pointCount 12, got %d", appCfg.MeasurementParams.PointCount)
	}

	if appCfg.CalibrationParams.PointCount != config.Default().CalibrationParams.PointCount {
		t.Fatalf("expected calibration params unchanged, got %+v", appCfg.CalibrationParams)
	}
}
