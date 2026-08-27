package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveConnectConfigUsesDefaultsWhenEnvMissing(t *testing.T) {
	cfg, err := resolveConnectConfig(func(string) string {
		return ""
	})
	if err != nil {
		t.Fatalf("resolve config returned unexpected error: %v", err)
	}

	// 默认值与 deviceconnect.DefaultConfig() 对齐：2 次重试用于短时网络抖动容错。
	// 详见 internal/application/deviceconnect/service.go DefaultConfig 注释。
	if cfg.ConnectMaxAttempts != 2 {
		t.Fatalf("expected default connect max attempts 2, got %d", cfg.ConnectMaxAttempts)
	}
}

func TestResolveConnectConfigLoadsFileWhenEnvPresent(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "runtime.json")

	content := `{
		"deviceConnect": {
			"connectAttemptTimeoutMs": 1800,
			"connectMaxAttempts": 6,
			"connectInitialBackoffMs": 90,
			"connectMaxBackoffMs": 500,
			"disconnectAttemptTimeoutMs": 1100,
			"disconnectMaxAttempts": 4,
			"disconnectInitialBackoffMs": 50,
			"disconnectMaxBackoffMs": 250
		}
	}`

	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write runtime config: %v", err)
	}

	cfg, err := resolveConnectConfig(func(key string) string {
		if key == configPathEnvName {
			return configPath
		}
		return ""
	})
	if err != nil {
		t.Fatalf("resolve config returned unexpected error: %v", err)
	}

	if cfg.ConnectAttemptTimeout != 1800*time.Millisecond {
		t.Fatalf("expected connect timeout 1800ms, got %s", cfg.ConnectAttemptTimeout)
	}
	if cfg.ConnectMaxAttempts != 6 {
		t.Fatalf("expected connect max attempts 6, got %d", cfg.ConnectMaxAttempts)
	}
	if cfg.DisconnectMaxAttempts != 4 {
		t.Fatalf("expected disconnect max attempts 4, got %d", cfg.DisconnectMaxAttempts)
	}
}

func TestResolveRuntimeConfigLoadsCalibrationValveGate(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "runtime.json")

	content := `{
		"calibration": {
			"enforceValveCalibrationGate": true
		}
	}`

	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write runtime config: %v", err)
	}

	runtimeCfg, err := resolveRuntimeConfig(func(key string) string {
		if key == configPathEnvName {
			return configPath
		}
		return ""
	})
	if err != nil {
		t.Fatalf("resolve runtime config returned unexpected error: %v", err)
	}

	if !runtimeCfg.Calibration.EnforceValveCalibrationGate {
		t.Fatalf("expected runtime calibration valve gate enabled")
	}
}
