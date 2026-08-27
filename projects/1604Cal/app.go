package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	apihttp "cal1604/internal/api/http"
	"cal1604/internal/config"
	"cal1604/internal/device/manager"
	"cal1604/internal/domain"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const configPathEnvName = "CAL1604_CONFIG"
const shutdownTimeout = 5 * time.Second

// App 是 Wails 桌面应用的核心结构体，持有内嵌 HTTP 服务器生命周期状态。
type App struct {
	ctx           context.Context
	server        *http.Server
	port          int
	shutdownFuncs []func(context.Context)
}

// NewApp 创建 App 实例。
func NewApp() *App {
	return &App{}
}

// startup 在 Wails 窗口创建后被调用。
// 启动内嵌 HTTP 服务器，监听 127.0.0.1 上的随机可用端口。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	logPath, err := debugLogPath()
	if err != nil {
		log.Printf("resolve debug log path failed: %v", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		// Windows GUI 程序通常没有可靠 stderr，MultiWriter 遇到 stderr 错误会导致文件也写不进去。
		log.SetOutput(logFile)
	} else {
		log.Printf("open debug log %q failed: %v", logPath, err)
	}
	log.Printf("[app] startup cwd=%q log=%q at %s", mustGetwd(), logPath, time.Now().Format(time.RFC3339))

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
	log.Printf("[app] reset %d device statuses to disconnected", len(deviceManager.List()))

	router, shutdownSrv := apihttp.NewRouterWithShutdownAndEmbedFS(deviceManager, connectCfg, calibrationCfg, configPath, templateAssets, runtimeCfg)
	if shutdownSrv != nil {
		a.shutdownFuncs = append(a.shutdownFuncs, shutdownSrv)
	}

	// 为桌面环境添加 CORS 支持。
	// Wails webview 使用 wails:// 协议加载前端页面，
	// 向 http://127.0.0.1 发起的 API 请求需要 CORS 头。
	// corsMiddleware 已在 router 层应用，此处无需重复包装。

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("failed to allocate local port: %v", err)
	}

	a.port = listener.Addr().(*net.TCPAddr).Port
	a.server = &http.Server{Handler: router}

	go func() {
		if err := a.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("internal API server error: %v", err)
		}
	}()

	log.Printf("internal API server started on 127.0.0.1:%d", a.port)
}

// shutdown 在应用退出时被调用，优雅关闭内嵌 HTTP 服务器并释放所有后台资源。
func (a *App) shutdown(ctx context.Context) {
	log.Printf("[app] shutdown started")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// 先关闭 HTTP 活动连接，取消正在阻塞的连接请求和 SSE 长连接。
	if a.server != nil {
		if err := a.server.Close(); err != nil && err != http.ErrServerClosed {
			log.Printf("[app] force close HTTP server failed: %v", err)
		}
	}

	cleanupDone := make(chan struct{})
	go func() {
		defer close(cleanupDone)
		for _, fn := range a.shutdownFuncs {
			fn(shutdownCtx)
		}
	}()

	select {
	case <-cleanupDone:
		log.Printf("[app] shutdown cleanup completed")
	case <-shutdownCtx.Done():
		log.Printf("[app] shutdown cleanup timeout: %v", shutdownCtx.Err())
	case <-ctx.Done():
		log.Printf("[app] wails shutdown context canceled: %v", ctx.Err())
	}

	a.shutdownFuncs = nil

	// Close 已强制断开活动连接；Shutdown 作为兜底释放内部资源，不能无限等待。
	if a.server != nil {
		_ = a.server.Shutdown(shutdownCtx)
	}
	log.Printf("[app] shutdown finished")
}

// GetAPIPort 返回内嵌 HTTP 服务器端口，供前端通过 Wails 绑定调用。
func (a *App) GetAPIPort() int {
	return a.port
}

// confirmClose 在用户点击窗口右上角 X 关闭应用时弹出确认对话框。
// 返回 true 阻止关闭，返回 false 允许关闭。
func (a *App) confirmClose(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	result, err := wailsRuntime.MessageDialog(ctx, wailsRuntime.MessageDialogOptions{
		Type:          wailsRuntime.QuestionDialog,
		Title:         "退出确认",
		Message:       "确定要退出 Cal1604 校准系统吗？",
		DefaultButton: "No",
		CancelButton:  "No",
	})
	if err != nil {
		// 对话框弹出失败时允许关闭，避免窗口无法退出。
		log.Printf("[app] close confirm dialog failed, allow close: %v", err)
		return false
	}
	// Windows 上 MessageDialog 忽略 Buttons 参数，固定显示系统"是/否"按钮，
	// 返回值映射为 "Yes"/"No"，因此只能比较 "Yes" 判断确认退出。
	return result != "Yes"
}

func debugLogPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "cal1604_debug.log", err
	}
	logDir := filepath.Join(configDir, "cal1604", "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return "cal1604_debug.log", err
	}
	return filepath.Join(logDir, "cal1604_debug.log"), nil
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

// SaveFileContent 将内容写入指定文件（桌面模式导出用）。
func (a *App) SaveFileContent(path string, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

// AppendFrontendLog 将 WebView 诊断信息追加到安装版调试日志。
// 前端只发送限长后的单行文本，避免异常对象或大响应再次放大内存压力。
func (a *App) AppendFrontendLog(message string) {
	message = strings.ReplaceAll(strings.ReplaceAll(message, "\r", " "), "\n", " ")
	if len(message) > 2000 {
		message = message[:2000]
	}
	log.Printf("[frontend] %s", message)
}

// ShowSaveFilePath 弹出系统"另存为"对话框，返回用户选择的文件路径。
// 前置调用方需保证 defaultName 非空，filterPattern 示例："*.xlsx"。
// 用户取消对话框时返回空字符串（无错误）。
func (a *App) ShowSaveFilePath(defaultName, filterName, filterPattern string) string {
	if a.ctx == nil {
		return ""
	}
	path, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		Title:           "选择导出路径",
		DefaultFilename: defaultName,
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: filterName, Pattern: filterPattern},
			{DisplayName: "所有文件", Pattern: "*.*"},
		},
	})
	if err != nil {
		return ""
	}
	return path
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
