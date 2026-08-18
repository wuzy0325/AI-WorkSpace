package backend

import (
	"bytes"
	"errors"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"windlabx4/services/api-go/pkg/appcontext"
	"windlabx4/services/api-go/pkg/types"
)

func TestCallMgrDoesNotDuplicateUsecaseFailureLog(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	app := &App{appContext: &appcontext.AppContext{}}
	result := app.callMgr(struct{}{}, "设备管理器", func() error {
		return errors.New("connection failed")
	})

	if result.Success || result.Error != "connection failed" {
		t.Fatalf("unexpected response: %#v", result)
	}
	if logs.Len() != 0 {
		t.Fatalf("callMgr duplicated usecase failure log: %s", logs.String())
	}
}

func TestCallMgrLogsUninitializedManager(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	app := &App{appContext: &appcontext.AppContext{}}
	result := app.callMgr(nil, "设备管理器", func() error { return nil })

	if result.Success || result.Error == "" {
		t.Fatalf("expected uninitialized manager failure, got %#v", result)
	}
	if !bytes.Contains(logs.Bytes(), []byte("manager 未初始化")) {
		t.Fatalf("missing uninitialized manager log: %s", logs.String())
	}
}

func TestConfigLoadNilAppReturnsError(t *testing.T) {
	var app *App

	if _, err := app.ConfigLoad("storage-settings"); err == nil {
		t.Fatal("expected nil app ConfigLoad to return an error")
	}
}

func TestConfigSaveNilAppReturnsError(t *testing.T) {
	var app *App

	res := app.ConfigSave("storage-settings", `{}`)
	if res.Success || res.Error == "" {
		t.Fatalf("expected nil app ConfigSave to fail, got %#v", res)
	}
}

func TestConfigLoadReturnsDecodedConfig(t *testing.T) {
	ctx, err := appcontext.NewAppContext(t.TempDir())
	if err != nil {
		t.Fatalf("create app context: %v", err)
	}
	app := &App{appContext: ctx}

	res := app.ConfigSave("storage-settings", `{"baseDirectory":"data/recordings","filePrefix":"run"}`)
	if !res.Success {
		t.Fatalf("save config failed: %#v", res)
	}

	loaded, err := app.ConfigLoad("storage-settings")
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	data, ok := loaded["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected decoded config object, got %#v", loaded["data"])
	}
	if data["filePrefix"] != "run" {
		t.Fatalf("expected filePrefix run, got %#v", data["filePrefix"])
	}
}

// writableAppDataDir 在 Windows 下读 %APPDATA%，在 Linux/macOS 下读 $XDG_CONFIG_HOME 或 $HOME。
// 这两条测试只验证 Windows 路径解析行为；其他平台跳过，避免 CI 误报。
func TestResolvePathUsesWritableAppDataForRelativePath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("ResolvePath AppData 行为仅 Windows 适用")
	}
	appData := t.TempDir()
	t.Setenv("APPDATA", appData)

	app := &App{}
	got, err := app.ResolvePath(filepath.Join("data", "recordings"))
	if err != nil {
		t.Fatalf("resolve path failed: %v", err)
	}

	want := filepath.Join(appData, "windlabx4", "data", "recordings")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolvePathKeepsAbsolutePath(t *testing.T) {
	abs := filepath.Join(os.TempDir(), "WindLabX4-recordings")

	app := &App{}
	got, err := app.ResolvePath(abs)
	if err != nil {
		t.Fatalf("resolve path failed: %v", err)
	}
	if got != filepath.Clean(abs) {
		t.Fatalf("expected %q, got %q", filepath.Clean(abs), got)
	}
}

func TestStorageStartRecordingResolvesRelativeOutputDir(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("ResolvePath AppData 行为仅 Windows 适用")
	}
	appData := t.TempDir()
	t.Setenv("APPDATA", appData)

	ctx, err := appcontext.NewAppContext(t.TempDir())
	if err != nil {
		t.Fatalf("create app context: %v", err)
	}
	app := &App{appContext: ctx}

	res := app.StorageStartRecording(types.StorageRecordingConfig{
		OutputDir:  filepath.Join("data", "recordings"),
		FilePrefix: "run",
	})
	if !res.Success {
		t.Fatalf("start recording failed: %#v", res)
	}
	t.Cleanup(func() { _ = app.appContext.StorageRecorder.Stop() })

	want := filepath.Join(appData, "windlabx4", "data", "recordings")
	status := app.StorageGetStatus()
	if status.OutputDir != want {
		t.Fatalf("expected output dir %q, got %q", want, status.OutputDir)
	}
}

func TestStartLocalAPIServerRejectsOccupiedPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:8900")
	if err != nil {
		t.Fatalf("listen on local API port: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	app := &App{appContext: &appcontext.AppContext{}}
	if err := app.startLocalAPIServer(); err == nil {
		t.Fatal("expected local API startup to fail when its port is occupied")
	}
	if app.apiServer != nil {
		t.Fatal("local API server must not be retained after a port conflict")
	}
}
