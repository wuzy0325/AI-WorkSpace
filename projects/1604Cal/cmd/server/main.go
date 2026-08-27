package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	apihttp "cal1604/internal/api/http"
	"cal1604/internal/application/deviceconnect"
	"cal1604/internal/config"
	"cal1604/internal/device/manager"
	"cal1604/internal/domain"
)

const configPathEnvName = "CAL1604_CONFIG"

func main() {
	addr := ":18080"

	runtimeCfg, err := resolveRuntimeConfig(os.Getenv)
	if err != nil {
		log.Fatalf("load runtime config failed: %v", err)
	}
	configPath := strings.TrimSpace(os.Getenv(configPathEnvName))
	connectCfg := runtimeCfg.ToDeviceConnectConfig()
	calibrationCfg := apihttp.CalibrationRuntimeConfig{
		EnforceValveCalibrationGate: runtimeCfg.Calibration.EnforceValveCalibrationGate,
	}

	// 使用持久化设备管理器，设备配置会自动保存到本地文件
	deviceManager, err := manager.NewPersistentDeviceManager(manager.StorageConfig{})
	if err != nil {
		log.Fatalf("init persistent device manager failed: %v", err)
	}

	// 启动时重置所有设备为断开状态，避免上次会话的 "connected" 残留
	for _, dev := range deviceManager.List() {
		deviceManager.UpdateStatus(dev.ID, domain.DeviceStatusDisconnected)
	}
	log.Printf("[server] reset %d device statuses to disconnected", len(deviceManager.List()))

	router := apihttp.NewRouterWithRuntimeConfig(deviceManager, connectCfg, calibrationCfg, configPath, runtimeCfg)

	log.Printf("server listening on %s", addr)
	if err = http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("server exit: %v", err)
	}
}

// resolveRuntimeConfig 根据环境变量解析运行时配置。
// 当未配置文件路径时，返回内置默认值。
func resolveRuntimeConfig(getenv func(string) string) (config.AppConfig, error) {
	path := strings.TrimSpace(getenv(configPathEnvName))
	if path == "" {
		return config.Default(), nil
	}

	return config.LoadFromFile(path)
}

// resolveConnectConfig 根据环境变量解析连接可靠性配置。
// 当未配置文件路径时，返回内置默认值。
// 兼容已有单元测试与调用方。
func resolveConnectConfig(getenv func(string) string) (deviceconnect.Config, error) {
	runtimeCfg, err := resolveRuntimeConfig(getenv)
	if err != nil {
		return deviceconnect.Config{}, err
	}
	return runtimeCfg.ToDeviceConnectConfig(), nil
}
