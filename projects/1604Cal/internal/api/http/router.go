package http

import (
	"context"
	"io/fs"
	"net/http"
	"time"

	"cal1604/internal/application/deviceconnect"
	"cal1604/internal/config"
	"cal1604/internal/device/manager"
	"cal1604/internal/domain"
)

// CalibrationRuntimeConfig 定义标定启动门禁的运行时配置。
type CalibrationRuntimeConfig struct {
	EnforceValveCalibrationGate bool
}

func defaultCalibrationRuntimeConfig() CalibrationRuntimeConfig {
	// 默认开启阀门门禁：阀门=校准模式是标定与计量启动的必要条件。
	return CalibrationRuntimeConfig{EnforceValveCalibrationGate: true}
}

// NewRouterWithDeviceManager 基于指定设备管理器创建路由。
func NewRouterWithDeviceManager(deviceManager deviceManager) http.Handler {
	return newRouter(deviceManager, nil, deviceconnect.DefaultConfig(), defaultCalibrationRuntimeConfig(), nil, "", "templates/reports")
}

// NewRouterWithDependencies 基于指定依赖创建路由。
// 该方法用于生产注入与集成测试注入同一套 HTTP 处理逻辑。
func NewRouterWithDependencies(deviceManager deviceManager, connector deviceConnector) http.Handler {
	return newRouter(deviceManager, connector, deviceconnect.DefaultConfig(), defaultCalibrationRuntimeConfig(), nil, "", "templates/reports")
}

// NewRouterWithConnectConfig 基于指定连接配置创建路由。
func NewRouterWithConnectConfig(deviceManager deviceManager, connectConfig deviceconnect.Config) http.Handler {
	return newRouter(deviceManager, nil, connectConfig, defaultCalibrationRuntimeConfig(), nil, "", "templates/reports")
}

// NewRouterWithRuntimeConfig 基于连接配置、标定门禁配置和应用配置创建路由。
func NewRouterWithRuntimeConfig(deviceManager deviceManager, connectConfig deviceconnect.Config, calibrationConfig CalibrationRuntimeConfig, configPath string, appCfg ...config.AppConfig) http.Handler {
	var cfg *config.AppConfig
	if len(appCfg) > 0 {
		cfg = &appCfg[0]
	}
	// 使用相对于可执行文件的templates/reports目录
	handler, _ := newRouterWithServer(deviceManager, nil, connectConfig, calibrationConfig, cfg, configPath, "templates/reports")
	return handler
}

// NewRouterWithShutdown 创建路由并返回清理函数，应用退出时调用以释放所有后台资源。
func NewRouterWithShutdown(deviceManager deviceManager, connectConfig deviceconnect.Config, calibrationConfig CalibrationRuntimeConfig, configPath string, appCfg ...config.AppConfig) (http.Handler, func(context.Context)) {
	return NewRouterWithShutdownAndEmbedFS(deviceManager, connectConfig, calibrationConfig, configPath, nil, appCfg...)
}

// NewRouterWithShutdownAndEmbedFS 创建路由并支持嵌入模板文件系统。
func NewRouterWithShutdownAndEmbedFS(deviceManager deviceManager, connectConfig deviceconnect.Config, calibrationConfig CalibrationRuntimeConfig, configPath string, templateEmbedFS fs.FS, appCfg ...config.AppConfig) (http.Handler, func(context.Context)) {
	var cfg *config.AppConfig
	if len(appCfg) > 0 {
		cfg = &appCfg[0]
	}
	handler, srv := newRouterWithServer(deviceManager, nil, connectConfig, calibrationConfig, cfg, configPath, "templates/reports", templateEmbedFS)

	cleanup := func(ctx context.Context) {
		// 用 Wails shutdown context 并加超时兜底
		cleanupCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()

		// 停止后台轮询（无 I/O）
		srv.multipressService.StopPolling()

		// 停止计量自动采集和采集循环
		srv.measurementService.StopAutoCollect()
		_ = srv.measurementService.Stop()

		// 结束标定流程（停止压力控制和 WTN1604 校准）
		_ = srv.calibrationService.EndCalibration(cleanupCtx)

		// 停止所有已注册打压设备
		_ = srv.multipressService.StopAll(cleanupCtx)

		// 断开所有已连接设备
		for _, dev := range deviceManager.List() {
			if dev.Status == domain.DeviceStatusConnected {
				_, _ = srv.deviceConnector.Disconnect(cleanupCtx, dev.ID)
			}
		}

		// 清理嵌入模板临时目录
		if srv.reportService != nil {
			_ = srv.reportService.CleanupEmbedTemplates()
		}
	}

	return handler, cleanup
}

func newRouterWithServer(
	deviceManager deviceManager,
	connector deviceConnector,
	connectConfig deviceconnect.Config,
	calibrationConfig CalibrationRuntimeConfig,
	appCfg *config.AppConfig,
	configPath string,
	templateDir string,
	templateEmbedFS ...fs.FS,
) (http.Handler, *apiServer) {
	deps := newDependencies(deviceManager, connector, connectConfig, calibrationConfig, appCfg, configPath, templateDir, templateEmbedFS...)

	server := &apiServer{
		deviceManager:      deps.DeviceManager,
		coordinator:        deps.WorkflowCoordinator,
		deviceConnector:    deps.DeviceConnector,
		connectConfig:      deps.ConnectConfig,
		calibrationConfig:  calibrationConfig,
		calibrationService: deps.CalibrationService,
		multipressService:  deps.MultipressService,
		sessionService:     deps.SessionService,
		measurementService: deps.MeasurementService,
		reportService:      deps.ReportService,
		batchService:       deps.BatchService,
		configPath:         deps.ConfigPath,
		appConfig:          deps.AppConfig,
	}

	mux := http.NewServeMux()

	// 健康检查与事件流
	mux.HandleFunc("/api/v1/health", healthHandler)
	mux.HandleFunc("/api/v1/events/stream", eventsStreamHandler)

	// 配置
	mux.HandleFunc("GET /api/v1/config/device-connect", server.deviceConnectConfigHandler)
	mux.HandleFunc("GET /api/v1/config/calibration", server.calibrationReadConfigHandler)
	mux.HandleFunc("POST /api/v1/config/calibration", server.calibrationSaveConfigHandler)
	mux.HandleFunc("GET /api/v1/config/measurement", server.measurementReadConfigHandler)
	mux.HandleFunc("POST /api/v1/config/measurement", server.measurementSaveConfigHandler)
	mux.HandleFunc("GET /api/v1/config/alarm", server.alarmReadConfigHandler)
	mux.HandleFunc("POST /api/v1/config/alarm", server.alarmSaveConfigHandler)
	// 启动门禁开关：让前端与后端配置同源，避免前端硬编码短路后端配置。
	mux.HandleFunc("GET /api/v1/config/gates", server.gatesConfigHandler)
	// 上次绑定的设备集合：前端页面加载时恢复勾选（多设备按勾选顺序）。
	mux.HandleFunc("GET /api/v1/config/last-devices", server.lastDevicesReadHandler)

	// 设备管理
	mux.HandleFunc("GET /api/v1/devices", server.listDevicesHandler)
	mux.HandleFunc("POST /api/v1/devices", server.handleUpsertDevice)
	mux.HandleFunc("DELETE /api/v1/devices/{id}", server.handleDeleteDevice)
	mux.HandleFunc("POST /api/v1/devices/status", server.deviceStatusHandler)
	mux.HandleFunc("POST /api/v1/devices/connect", server.deviceConnectHandler)
	mux.HandleFunc("POST /api/v1/devices/disconnect", server.deviceDisconnectHandler)
	mux.HandleFunc("GET /api/v1/checks/unit-consistency", server.unitConsistencyHandler)

	// 会话控制
	mux.HandleFunc("GET /api/v1/sessions/state", server.sessionStateHandler)
	mux.HandleFunc("POST /api/v1/sessions/start", server.sessionStartHandler)
	mux.HandleFunc("POST /api/v1/sessions/pause", server.sessionPauseHandler)
	mux.HandleFunc("POST /api/v1/sessions/resume", server.sessionResumeHandler)
	mux.HandleFunc("POST /api/v1/sessions/stop", server.sessionStopHandler)

	// 设备会话（共享：设备绑定、读压、读稳定性、读计量数据、阀门、单位）
	mux.HandleFunc("POST /api/v1/session/devices", server.sessionSetDevicesHandler)
	mux.HandleFunc("POST /api/v1/session/measure-device", server.sessionSetMeasureDeviceHandler)
	mux.HandleFunc("POST /api/v1/session/measure-device/unbind", server.sessionUnbindMeasureDevicesHandler)
	mux.HandleFunc("GET /api/v1/session/pressure", server.sessionReadPressureHandler)
	mux.HandleFunc("GET /api/v1/session/stability", server.sessionReadStabilityHandler)
	mux.HandleFunc("GET /api/v1/session/measure-data", server.sessionReadMeasureDataHandler)
	mux.HandleFunc("GET /api/v1/session/valve", server.sessionGetValveHandler)
	mux.HandleFunc("GET /api/v1/session/valve/all", server.sessionGetValveAllHandler)
	mux.HandleFunc("POST /api/v1/session/valve", server.sessionSetValveHandler)
	mux.HandleFunc("PUT /api/v1/session/valve", server.sessionSetValveHandler)
	mux.HandleFunc("POST /api/v1/session/calibrate-zero", server.sessionCalibrateZeroHandler)
	mux.HandleFunc("POST /api/v1/session/calibrate-full-scale", server.sessionCalibrateFullScaleHandler)
	mux.HandleFunc("GET /api/v1/session/measure-unit", server.sessionGetMeasureUnitHandler)
	mux.HandleFunc("GET /api/v1/session/measure-unit/all", server.sessionGetMeasureUnitAllHandler)
	mux.HandleFunc("POST /api/v1/session/measure-unit", server.sessionSetMeasureUnitHandler)
	mux.HandleFunc("POST /api/v1/session/measure-unit/all", server.sessionSetMeasureUnitAllHandler)
	mux.HandleFunc("GET /api/v1/session/unit-consistency", server.sessionUnitConsistencyHandler)
	mux.HandleFunc("GET /api/v1/session/device-info", server.sessionReadDeviceInfoHandler)
	mux.HandleFunc("POST /api/v1/session/reset", server.sessionResetDeviceHandler)

	// 计量模块
	mux.HandleFunc("GET /api/v1/measurement/state", server.measurementStateHandler)
	mux.HandleFunc("POST /api/v1/measurement/start", server.measurementStartHandler)
	mux.HandleFunc("POST /api/v1/measurement/pause", server.measurementPauseHandler)
	mux.HandleFunc("POST /api/v1/measurement/stop", server.measurementStopHandler)
	mux.HandleFunc("POST /api/v1/measurement/points/generate", server.measurementGeneratePointsHandler)
	mux.HandleFunc("GET /api/v1/measurement/points", server.measurementPointsHandler)
	mux.HandleFunc("GET /api/v1/measurement/data", server.measurementDataHandler)
	mux.HandleFunc("POST /api/v1/measurement/export", server.measurementExportHandler)
	mux.HandleFunc("GET /api/v1/config/measurement-alarm", server.measurementGetAlarmConfigHandler)
	mux.HandleFunc("POST /api/v1/config/measurement-alarm", server.measurementSetAlarmConfigHandler)
	mux.HandleFunc("POST /api/v1/measurement/alarm/resolve", server.measurementAlarmResolveHandler)
	mux.HandleFunc("POST /api/v1/measurement/skip-device", server.measurementSkipDeviceHandler)
	mux.HandleFunc("GET /api/v1/measurement/alarm/pending", server.measurementAlarmPendingHandler)
	mux.HandleFunc("POST /api/v1/measurement/auto-collect", server.measurementAutoCollectHandler)
	mux.HandleFunc("POST /api/v1/measurement/manual-start", server.measurementManualStartHandler)
	mux.HandleFunc("POST /api/v1/measurement/manual-pressurize", server.measurementManualPressurizeHandler)
	mux.HandleFunc("POST /api/v1/measurement/manual-collect", server.measurementManualCollectHandler)
	mux.HandleFunc("POST /api/v1/measurement/stability-timeout/resolve", server.measurementStabilityTimeoutResolveHandler)
	// 稳定超时挂起查询：后端阻塞等待决策期间，页面刷新/崩溃恢复后前端据此重新弹窗。
	mux.HandleFunc("GET /api/v1/measurement/stability-timeout/pending", server.measurementStabilityTimeoutPendingHandler)

	// 校准流程
	mux.HandleFunc("POST /api/v1/calibration/devices", server.calibrationSetDevicesHandler)
	mux.HandleFunc("POST /api/v1/calibration/config", server.calibrationSetConfigHandler)
	mux.HandleFunc("POST /api/v1/calibration/channels", server.calibrationSetChannelsHandler)
	mux.HandleFunc("GET /api/v1/calibration/channels/list", server.calibrationGetChannelsHandler)
	mux.HandleFunc("POST /api/v1/calibration/points/generate", server.calibrationGeneratePointsHandler)
	mux.HandleFunc("GET /api/v1/calibration/points", server.calibrationGetPointsHandler)
	mux.HandleFunc("PUT /api/v1/calibration/points/{index}/target-pressure", server.calibrationUpdateTargetPressureHandler)
	mux.HandleFunc("POST /api/v1/calibration/pressurize", server.calibrationPressurizeHandler)
	mux.HandleFunc("POST /api/v1/calibration/collect", server.calibrationCollectHandler)
	mux.HandleFunc("POST /api/v1/calibration/fit", server.calibrationFitHandler)
	mux.HandleFunc("POST /api/v1/calibration/resolve-alarm", server.calibrationResolveAlarmHandler)
	mux.HandleFunc("POST /api/v1/calibration/skip-device", server.calibrationSkipDeviceHandler)
	mux.HandleFunc("POST /api/v1/calibration/retry-point", server.calibrationRetryPointHandler)
	mux.HandleFunc("GET /api/v1/calibration/alarm-config", server.calibrationGetAlarmConfigHandler)
	mux.HandleFunc("POST /api/v1/calibration/alarm-config/set", server.calibrationSetAlarmConfigHandler)
	mux.HandleFunc("GET /api/v1/calibration/session", server.calibrationGetSessionHandler)
	mux.HandleFunc("POST /api/v1/calibration/manual-pressurize", server.calibrationManualPressurizeHandler)
	mux.HandleFunc("POST /api/v1/calibration/manual-collect", server.calibrationManualCollectHandler)

	// 多设备打压控制
	mux.HandleFunc("POST /api/v1/multipress/register", server.multipressRegisterHandler)
	mux.HandleFunc("POST /api/v1/multipress/unregister", server.multipressUnregisterHandler)
	mux.HandleFunc("POST /api/v1/multipress/set-pressure", server.multipressSetPressureHandler)
	mux.HandleFunc("POST /api/v1/multipress/stop", server.multipressStopHandler)
	mux.HandleFunc("POST /api/v1/multipress/exhaust", server.multipressExhaustHandler)
	mux.HandleFunc("GET /api/v1/multipress/pressure", server.multipressReadPressureHandler)
	mux.HandleFunc("GET /api/v1/multipress/stability", server.multipressReadStabilityHandler)
	mux.HandleFunc("GET /api/v1/multipress/unit", server.multipressGetUnitHandler)
	mux.HandleFunc("POST /api/v1/multipress/unit", server.multipressSetUnitHandler)
	mux.HandleFunc("GET /api/v1/multipress/devices", server.multipressDevicesHandler)
	mux.HandleFunc("POST /api/v1/multipress/stop-all", server.multipressStopAllHandler)

	// 报告
	mux.HandleFunc("GET /api/v1/reports/templates/select", server.reportTemplateSelectHandler)
	mux.HandleFunc("POST /api/v1/reports/export", server.exportReportHandler)
	mux.HandleFunc("GET /api/v1/reports/templates", server.listTemplatesHandler)

	// 分批计量（多量程压力传感器）
	mux.HandleFunc("POST /api/v1/batch/sessions", server.batchCreateSessionHandler)
	mux.HandleFunc("GET /api/v1/batch/sessions/{sessionId}", server.batchGetSessionHandler)
	mux.HandleFunc("DELETE /api/v1/batch/sessions/{sessionId}", server.batchDeleteSessionHandler)
	mux.HandleFunc("POST /api/v1/batch/sessions/{sessionId}/batches/{batchId}/verify", server.batchVerifyHandler)
	mux.HandleFunc("POST /api/v1/batch/sessions/{sessionId}/batches/{batchId}/start", server.batchStartHandler)
	mux.HandleFunc("POST /api/v1/batch/sessions/{sessionId}/batches/{batchId}/complete", server.batchCompleteHandler)
	mux.HandleFunc("POST /api/v1/batch/sessions/{sessionId}/batches/{batchId}/reset", server.batchResetHandler)
	mux.HandleFunc("POST /api/v1/batch/report", server.batchReportHandler)

	return corsMiddleware(mux), server
}

// newRouter 是兼容旧调用的封装，返回 HTTP handler，忽略 apiServer。
func newRouter(
	deviceManager deviceManager,
	connector deviceConnector,
	connectConfig deviceconnect.Config,
	calibrationConfig CalibrationRuntimeConfig,
	appCfg *config.AppConfig,
	configPath string,
	templateDir string,
) http.Handler {
	h, _ := newRouterWithServer(deviceManager, connector, connectConfig, calibrationConfig, appCfg, configPath, templateDir)
	return h
}

// corsMiddleware 为所有 API 响应添加 CORS 头，处理 OPTIONS 预检请求。
// 桌面模式下前端从 Vite 开发服务器加载（如 localhost:5179），
// 而 API 运行在不同端口，需要 CORS 支持才能正常通信。
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// NewRouter 返回 API 路由，用于注册系统对外提供的 HTTP 端点。
func NewRouter() http.Handler {
	return NewRouterWithDeviceManager(manager.NewDeviceManager())
}
