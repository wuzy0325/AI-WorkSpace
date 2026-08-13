package api

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	motionhttp "shared.local/motion-control/go/httpapi"
	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/core/device"
	wind_report "wind-daq/services/api-go/internal/core/report"
	"wind-daq/services/api-go/internal/core/storage"
	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
	"wind-daq/services/api-go/internal/usecase"
	"wind-daq/services/api-go/pkg/logging"
	"wind-daq/services/api-go/pkg/types"
)

type Deps struct {
	DeviceManager      *usecase.DeviceManager
	AcquisitionHub     *usecase.AcquisitionHub
	ReportManager      *usecase.ReportManager
	MotionManager      ports.MotionManager
	MotionService      motionhttp.MotionService
	CalibrationManager *usecase.CalibrationManager
	TraversalManager   *usecase.TraversalManager
	// TraversalRegistry 双探针 registry（窄接口；nil 时 probe-scoped 路由返回 503）。
	// legacy 单段 /api/traversal/{action} 路径不使用本字段（spec FR4 兼容）。
	TraversalRegistry TraversalRegistry
	StorageRecorder   *usecase.StorageRecorder
	ConfigManager     *usecase.ConfigManager
	LogRing           *logging.RingBuffer
	LogManager        *logging.Manager // 用于日志分类开关 API

	// AppHandler 由桌面壳层（Electron 主进程的 backend.App）实现，注入到 api.Deps。
	// 经 HTTP 路由层暴露 /api/app/* 端点（version / startup-mode / resolve-path）。
	// Electron 分支必填，Wails 分支可空。
	AppHandler AppHandler
	// OnAcquisitionStarted 在 startAcquisition 成功后异步触发，
	// 实现"采集启动后自动开始录制"业务策略（读 storage-settings.autoStartOnAcquisition）。
	// 异步调用（go func），不阻塞 HTTP 响应；nil 时跳过。
	OnAcquisitionStarted func(deviceID string)
}

// AppHandler 由桌面壳层（Electron 主进程的 backend.App）实现，注入到 api.Deps。
// OpenMotionWindow 走 Electron IPC（不走 HTTP），但接口方法保留——编译期
// `var _ api.AppHandler = (*App)(nil)` 校验要求所有方法都被实现。
type AppHandler interface {
	// Version 返回应用版本信息（名称 + 版本号），供前端标题栏/关于对话框显示。
	Version() AppVersionInfo
	// StartupMode 返回 "normal"（主窗口）或 "motion"（运动控制器独立窗口子进程）。
	StartupMode() string
	// OpenMotionWindow 启动运动控制器独立窗口（Electron 走 IPC，不走 HTTP 路由）。
	OpenMotionWindow() error
	// ResolvePath 将相对路径解析到用户可写应用目录（%APPDATA%\wind-daq）。
	ResolvePath(path string) (string, error)
}

// AppVersionInfo 是 GET /api/app/version 的响应体。
type AppVersionInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func NewRouter(deps Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("/api/app/", func(w http.ResponseWriter, r *http.Request) {
		if deps.AppHandler == nil {
			writeError(w, http.StatusServiceUnavailable, "app handler not injected")
			return
		}
		action := strings.TrimPrefix(r.URL.Path, "/api/app/")
		switch action {
		case "version":
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			writeJSON(w, http.StatusOK, deps.AppHandler.Version())
		case "startup-mode":
			// 前端要求 text/plain 响应（不是 JSON），保持与 wails-adapter.ts 契约一致。
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, deps.AppHandler.StartupMode())
		case "resolve-path":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var body struct {
				Path string `json:"path"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			resolved, err := deps.AppHandler.ResolvePath(body.Path)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			// 同时返回 success 包裹 + path 字段，兼容前端 {path: string} 与字符串两种解析。
			writeJSON(w, http.StatusOK, map[string]string{"path": resolved})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	mux.HandleFunc("/api/device/profiles", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, deps.DeviceManager.GetProfiles())
		case http.MethodPut:
			var profile device.Profile
			if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if err := deps.DeviceManager.UpsertProfile(profile); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/device/scan", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		results, err := deps.DeviceManager.ScanDevices()
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, results)
	})
	mux.HandleFunc("/api/device/profiles/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/device/profiles/")
		if id == "" {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if err := deps.DeviceManager.DeleteProfile(id); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	})
	mux.HandleFunc("/api/device/", func(w http.ResponseWriter, r *http.Request) {
		handleDeviceByID(w, r, deps)
	})
	mux.HandleFunc("/api/v1/devices/", func(w http.ResponseWriter, r *http.Request) {
		cloned := r.Clone(r.Context())
		cloned.URL.Path = "/api/device/" + strings.TrimPrefix(r.URL.Path, "/api/v1/devices/")
		handleDeviceByID(w, cloned, deps)
	})
	// calibrationConfig 暴露校零采样时长等配置，供前端避免硬编码 5s。
	mux.HandleFunc("/api/v1/calibrationConfig", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"durationSec": device.CalibrationDurationSec,
		})
	})
	mux.HandleFunc("/api/daq/latest/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/daq/latest/")
		// 设备不存在（已断开/异常退出被 DeviceManager 从 map 删除）时返回 404，
		// 让前端轮询能感知断连并更新 UI 状态。
		// 此前返回 200 + 空 payload 导致前端轮询静默吞掉，UI 永远显示"采集中"。
		if _, ok := deps.DeviceManager.GetStatus(id); !ok {
			// 错误消息用 "device offline" 而非 "not connected"：覆盖更广
			// （含未连接、已断开、异常退出三态），避免语义误导。
			writeError(w, http.StatusNotFound, "device offline")
			return
		}
		payload, ok := deps.AcquisitionHub.GetLatestData(id)
		if !ok {
			// 设备已连接但尚未出第一帧：返回仅含 deviceId 的空 payload
			writeDataPayloadJSON(w, http.StatusOK, device.DataPayload{DeviceID: id})
			return
		}
		// 用手写编码绕过 reflect，规避 Go 1.26 structEncoder 偶发 panic
		writeDataPayloadJSON(w, http.StatusOK, payload)
	})
	mux.HandleFunc("/api/daq/stream/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/daq/stream/")
		if id == "" {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		handleDaqStream(w, r, deps.AcquisitionHub, id)
	})
	mux.HandleFunc("/api/daq/publishRate", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, map[string]float64{"hz": deps.AcquisitionHub.PublishRate()})
		case http.MethodPut:
			var body struct {
				Hz float64 `json:"hz"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if err := deps.AcquisitionHub.SetPublishRate(body.Hz); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/report/generate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var cfg struct {
			OutputDir  string `json:"outputDir"`
			FilePrefix string `json:"filePrefix"`
			DeviceID   string `json:"deviceId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		result, err := deps.ReportManager.Generate(wind_report.ReportConfig{OutputDir: cfg.OutputDir, FilePrefix: cfg.FilePrefix, DeviceID: cfg.DeviceID})
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("/api/report/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, deps.ReportManager.Status())
	})

	// ---- Motion API ----
	if deps.MotionService != nil {
		motionhttp.RegisterMotionRoutes(mux, deps.MotionService)
	}

	// ---- Calibration API ----
	mux.HandleFunc("/api/calibration/", func(w http.ResponseWriter, r *http.Request) {
		action := strings.TrimPrefix(r.URL.Path, "/api/calibration/")
		switch action {
		case "start":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			// 读取整个请求体后用 pkg/types 的解码器转换为 core 层的 calibration.Config。
			// 这里不直接 json.Decode 进 calibration.Config，因为前端发送的探针通道是嵌套
			// channel 格式，而 core 层禁止自带 UnmarshalJSON（零容忍约束），解码逻辑必须
			// 在 transport boundary（pkg/types）完成。Task 05 从 adapters/config 迁移过来。
			data, err := io.ReadAll(r.Body)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			body, err := types.DecodeCalibrationConfig(data)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			err = deps.CalibrationManager.Start(body)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "status":
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			writeJSON(w, http.StatusOK, deps.CalibrationManager.Status())
		case "collect":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if err := deps.CalibrationManager.CollectCurrentPoint(); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "result":
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			taskID := r.URL.Query().Get("taskId")
			if taskID == "" {
				writeError(w, http.StatusBadRequest, "taskId query parameter is required")
				return
			}
			result, ok := deps.CalibrationManager.GetResult(taskID)
			if !ok {
				writeError(w, http.StatusNotFound, "calibration result not found")
				return
			}
			writeJSON(w, http.StatusOK, result)
		case "saveCsv":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var body struct {
				TaskID   string `json:"taskId"`
				SavePath string `json:"savePath"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			path, err := deps.CalibrationManager.SaveCsv(body.TaskID, body.SavePath)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "filepath": path})
		case "precisionDefaults":
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			writeJSON(w, http.StatusOK, map[string]int{
				"probePrecision":    3,
				"machPrecision":     4,
				"velocityPrecision": 3,
			})
		case "fivehole":
			// spec Task 10：五孔点位预览委托 usecase 层 PreviewFiveHolePoints，
			// 不再在 API 层直接调 core.GenerateFiveHoleSnakePoints——
			// 与 sevenhole-preview 路由对称，HTTP/Wails 共用同一 usecase 入口。
			handleFiveHolePreview(w, r, deps.CalibrationManager)
		case "sevenhole-preview":
			// 七孔点位预览（spec Task 12）：前端"配置向导"调整 α/β/θ/φ 范围与步长时
			// 实时显示总点数与内/外区分布，让操作员在启动校准前确认点位规模。
			// sevenhole-start 不单独路由——走现有 start 路由，type 字段为 "seven-hole"，
			// usecase.Start 内部按 config.Type 分发到 createAlgorithm 七孔分支。
			handleSevenHolePreview(w, r, deps.CalibrationManager)
		case "pause":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if err := deps.CalibrationManager.Pause(); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "resume":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if err := deps.CalibrationManager.Resume(); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "stop":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if err := deps.CalibrationManager.Stop(); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	// ---- Traversal API ----
	mux.HandleFunc("/api/traversal/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/traversal/")
		// 两段 probe-scoped 路径（{probeId}/{action}）进入 dual dispatcher；
		// 单段路径继续走 legacy（禁止隐式转发到 probe1，spec FR4）。
		if strings.Contains(rest, "/") {
			handleDualTraversal(w, r, deps, rest)
			return
		}
		action := rest
		switch action {
		case "config":
			if deps.TraversalManager == nil {
				writeError(w, http.StatusBadRequest, "traversal manager is required")
				return
			}
			switch r.Method {
			case http.MethodGet:
				raw := deps.TraversalManager.GetConfigRaw()
				if len(raw) == 0 {
					writeJSON(w, http.StatusOK, nil)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(raw)
			case http.MethodPost:
				var raw json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
					writeError(w, http.StatusBadRequest, err.Error())
					return
				}
				deps.TraversalManager.SaveConfigRaw(raw)
				writeJSON(w, http.StatusOK, map[string]bool{"success": true})
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		case "importPrb":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var body struct {
				FilePath string `json:"filePath"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			// Task 08：委托 usecase.ImportPRB，API 不再 import interpolation adapter。
			res, err := deps.TraversalManager.ImportPRB(body.FilePath)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, res)
		case "importCalibrationCsv":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var body struct {
				FilePath string `json:"filePath"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			// Task 08：委托 usecase.ImportCalibrationCSV，pointCount 由 usecase 类型断言获取。
			res, err := deps.TraversalManager.ImportCalibrationCSV(body.FilePath)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, res)
		case "importMultiPrb":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var body struct {
				FilePaths         []string  `json:"filePaths"`
				MachNumbers       []float64 `json:"machNumbers"`
				InterpolationMode string    `json:"interpolationMode"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			// Task 08：委托 usecase.ImportMultiPRB，mode 透传由 usecase 处理，
			// API 不再 import coreinterp 来做类型转换、不再直接调用 SetInterpolationMode。
			res, err := deps.TraversalManager.ImportMultiPRB(body.FilePaths, body.MachNumbers, body.InterpolationMode)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, res)
		case "importSevenHolePrb":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			// 七孔 .prb 文件集导入（spec §5.6）：1 个内区文件 + 恰 6 个扇区文件，
			// 成功后仅设置七孔插值器（不影响五孔字段），返回逐文件信息。
			// Task 08：校验与响应组装下沉到 usecase.ImportSevenHolePRB。
			var body struct {
				InnerFilePath  string   `json:"innerFilePath"`
				OuterFilePaths []string `json:"outerFilePaths"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			res, err := deps.TraversalManager.ImportSevenHolePRB(body.InnerFilePath, body.OuterFilePaths)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, res)
		case "importSevenHoleCalibrationCsv":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			// 七孔校准 CSV 文件集导入（校准 CSV → 插值网格，spec §10 Q2 落地）：
			// 与 importSevenHolePrb 同 DTO（1 内区 + 6 扇区路径），
			// Task 08：校验与响应组装下沉到 usecase.ImportSevenHoleCalibrationCSV。
			var body struct {
				InnerFilePath  string   `json:"innerFilePath"`
				OuterFilePaths []string `json:"outerFilePaths"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			res, err := deps.TraversalManager.ImportSevenHoleCalibrationCSV(body.InnerFilePath, body.OuterFilePaths)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, res)
		case "clearInterpolator":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			// 显式清理指定探针类型的插值器（spec §5.2.1）：probeType 必填，
			// 不允许缺省猜测当前类型，供前端切换探针前失效陈旧校准。
			var body struct {
				ProbeType string `json:"probeType"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if body.ProbeType == "" {
				writeError(w, http.StatusBadRequest, "probeType 必填（five-hole / seven-hole）")
				return
			}
			if err := deps.TraversalManager.ClearProbeInterpolator(body.ProbeType); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"cleared": true})
		case "calculateRealtime":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			// 请求体为五孔超集（spec §5.6）：旧五孔 body 可省略 probeType；
			// 七孔必须显式传 "seven-hole" 且携带 P6/P7（*float64：nil=缺失，
			// 非 nil 含 0=已提供，禁止以零值猜测类型）。
			// Task 09：API 只解码 transport DTO，所有探针分发/P6P7 presence/
			// 类型一致性校验下沉到 usecase.CalculateRealtimeForAPI。
			var body struct {
				ProbeType string `json:"probeType"`
				Pressures struct {
					P1   float64  `json:"P1"`
					P2   float64  `json:"P2"`
					P3   float64  `json:"P3"`
					P4   float64  `json:"P4"`
					P5   float64  `json:"P5"`
					P6   *float64 `json:"P6"`
					P7   *float64 `json:"P7"`
					PAtm float64  `json:"Patm"`
					TAtm float64  `json:"Tatm"`
				} `json:"pressures"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			result, err := deps.TraversalManager.CalculateRealtimeForAPI(body.ProbeType, usecase.ProbePressureInput{
				P1:   body.Pressures.P1,
				P2:   body.Pressures.P2,
				P3:   body.Pressures.P3,
				P4:   body.Pressures.P4,
				P5:   body.Pressures.P5,
				P6:   body.Pressures.P6,
				P7:   body.Pressures.P7,
				PAtm: body.Pressures.PAtm,
				TAtm: body.Pressures.TAtm,
			})
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, result)
		case "checkPreconditions":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var body struct {
				Config json.RawMessage `json:"config"`
			}
			// 解析失败不阻断：无 config 时回退到 manager 当前配置
			_ = json.NewDecoder(r.Body).Decode(&body)
			var config *traversal.Config
			if len(body.Config) > 0 {
				cfg, err := deps.TraversalManager.ParseConfig(body.Config)
				if err != nil {
					writeError(w, http.StatusBadRequest, err.Error())
					return
				}
				config = &cfg
			}
			writeJSON(w, http.StatusOK, deps.TraversalManager.CheckPreconditions(config))
		case "generateGrid":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if deps.TraversalManager == nil {
				writeError(w, http.StatusBadRequest, "traversal manager is required")
				return
			}
			var grid traversal.GridConfig
			if err := json.NewDecoder(r.Body).Decode(&grid); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			path, err := deps.TraversalManager.GenerateGridPath(grid)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, path)
		case "start":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var raw json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			taskID, err := deps.TraversalManager.ParseAndStartTraversal(raw)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"taskId": taskID})
		case "status":
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			writeJSON(w, http.StatusOK, deps.TraversalManager.BuildStatusResponse())
		case "runPoint":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if err := deps.TraversalManager.RunCurrentPoint(); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "pause":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if err := deps.TraversalManager.Pause(); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "resume":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if err := deps.TraversalManager.Resume(); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "stop":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if err := deps.TraversalManager.Stop(); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "result":
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			taskID := r.URL.Query().Get("taskId")
			if taskID == "" {
				writeError(w, http.StatusBadRequest, "taskId query parameter is required")
				return
			}
			result, ok := deps.TraversalManager.GetResult(taskID)
			if !ok {
				writeError(w, http.StatusNotFound, "traversal result not found")
				return
			}
			writeJSON(w, http.StatusOK, result)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	mux.HandleFunc("/api/config/", func(w http.ResponseWriter, r *http.Request) {
		if deps.ConfigManager == nil {
			writeError(w, http.StatusServiceUnavailable, "config manager not initialized")
			return
		}
		key := strings.TrimPrefix(r.URL.Path, "/api/config/")
		if key == "" {
			writeError(w, http.StatusBadRequest, "config key is required")
			return
		}
		switch r.Method {
		case http.MethodGet:
			data, err := deps.ConfigManager.LoadConfig(key)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": data})
		case http.MethodPut:
			var raw json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if err := deps.ConfigManager.SaveConfig(key, raw); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// ---- Storage API ----
	mux.HandleFunc("/api/storage/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, deps.StorageRecorder.Status())
	})
	mux.HandleFunc("/api/storage/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		// 接收完整 RecordingConfig：业务级字段（StopConditions/FileRotation/Format）
		// 直接透传给 StorageRecorder.Start，由 sink 在 writerLoop 内评估。
		// sink 调优参数（queueCapacity/bufferSize/flush/sync）由装配层从 storage.json
		// 读取，不在此处覆盖，避免双轨配置冲突。
		var body storage.RecordingConfig
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		// 注入通道元数据：仅收集 DAQ-P-1603 profile 的 channels，供 sink 生成带单位后缀的 CSV 表头
		// （CH01_Pa/CH02_degC）。
		//
		// 限定 DAQ-P-1603 的原因：
		//   - ChannelConfig.UnmarshalJSON 将缺省 SensorType 兜底为 "pressure"
		//   - DAQ-T-1603 / WTN_PXI 等历史设备的默认 profile 未显式设置 SensorType，
		//     但 Unit 是 "degC"——若一视同仁注入，温度通道会被误标为 _Pa，与实际单位冲突
		//   - DAQ-P-1604 走 isWideFormat 固定表头分支，不需要 channelConfigs
		//
		// 录制中后连接的 DAQ-P-1603 若未在此映射中，sink 回退到通用 CH01..CHnn 表头（保持兼容）。
		// 即便前端 body 已携带 DeviceChannels（理论上不会），也以服务端实际 profile 为准覆盖。
		profiles := deps.DeviceManager.GetProfiles()
		body.DeviceChannels = make(map[string][]device.ChannelConfig, len(profiles))
		for _, profile := range profiles {
			if profile.Type == device.DeviceDAQP1603 && len(profile.Channels) > 0 {
				body.DeviceChannels[profile.ID] = profile.Channels
			}
		}
		if err := deps.StorageRecorder.Start(body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	})
	mux.HandleFunc("/api/storage/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if err := deps.StorageRecorder.Stop(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	})

	// ---- Log API ----
	// 即使 ring 为 nil，也注册日志端点（返回空数据），避免前端 SSE 连接反复 404
	mux.HandleFunc("/api/log/stream", func(w http.ResponseWriter, r *http.Request) {
		if deps.LogRing == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"success": false,
				"error":   "日志系统未初始化",
			})
			return
		}
		handleLogStream(w, r, deps.LogRing)
	})
	mux.HandleFunc("/api/log/recent", func(w http.ResponseWriter, r *http.Request) {
		if deps.LogRing == nil {
			writeJSON(w, http.StatusOK, map[string]any{"entries": []logging.RingEntry{}})
			return
		}
		handleLogRecent(w, r, deps.LogRing)
	})
	// 日志分类开关 API：控制各 category 是否写入 ring buffer
	mux.HandleFunc("/api/log/categories", func(w http.ResponseWriter, r *http.Request) {
		handleLogCategories(w, r, deps.LogManager)
	})

	// 中间件链：metrics（最外层，记录所有请求耗时）→ recover（拦截 panic）→ cors → mux
	// 顺序原因：metrics 需要能看到 recover 后的最终状态码；
	// cors 需要在 OPTIONS 短路前生效，所以放在 mux 之前最贴近。
	return metricsMiddleware(recoverMiddleware(corsMiddleware(mux)))
}

func handleDaqStream(w http.ResponseWriter, r *http.Request, hub *usecase.AcquisitionHub, deviceID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}
	subscription, unsubscribe := hub.Subscribe(deviceID, 16)
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case payload := <-subscription:
			// 用手写编码绕过 reflect，规避 Go 1.26 structEncoder 偶发 panic
			data := marshalDataPayload(payload)
			_, _ = fmt.Fprintf(w, "event: payload\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func handleDeviceByID(w http.ResponseWriter, r *http.Request, deps Deps) {
	path := strings.TrimPrefix(r.URL.Path, "/api/device/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	id, action := parts[0], parts[1]

	switch {
	case r.Method == http.MethodPost && action == "connect":
		if err := deps.DeviceManager.Connect(id); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	case r.Method == http.MethodPost && action == "disconnect":
		if err := deps.DeviceManager.Disconnect(id); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	case r.Method == http.MethodPost && action == "startAcquisition":
		if err := deps.DeviceManager.StartAcquisition(id); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		// 异步触发"采集启动后自动开始录制"业务策略。
		// 回调内会读 storage-settings.autoStartOnAcquisition，失败仅记录日志不阻塞采集。
		// 异步执行避免慢回调（如读配置 + 启动 sink）阻塞 HTTP 响应。
		if deps.OnAcquisitionStarted != nil {
			go deps.OnAcquisitionStarted(id)
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	case r.Method == http.MethodPost && action == "stopAcquisition":
		if err := deps.DeviceManager.StopAcquisition(id); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	case r.Method == http.MethodGet && action == "status":
		status, ok := deps.DeviceManager.GetStatus(id)
		if !ok {
			writeError(w, http.StatusNotFound, "device not connected")
			return
		}
		writeJSON(w, http.StatusOK, status)
	case r.Method == http.MethodPut && action == "unit":
		var body struct {
			Unit string `json:"unit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := deps.DeviceManager.SetUnit(id, body.Unit); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	case r.Method == http.MethodGet && action == "daqT1603Config":
		config, err := deps.DeviceManager.GetDaqT1603Config(id)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, config)
	case r.Method == http.MethodPut && action == "daqT1603Config":
		var config device.DaqT1603HardwareConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := deps.DeviceManager.ApplyDaqT1603Config(id, config); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	case r.Method == http.MethodGet && action == "daqT1602Config":
		config, err := deps.DeviceManager.GetDaqT1602Config(id)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, config)
	case r.Method == http.MethodPut && action == "daqT1602Config":
		var config device.DaqT1602HardwareConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := deps.DeviceManager.ApplyDaqT1602Config(id, config); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	case r.Method == http.MethodGet && action == "dsa3217ScanConfig":
		config, err := deps.DeviceManager.GetDsa3217ScanConfig(id)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": config})
	case r.Method == http.MethodPut && action == "dsa3217ScanConfig":
		var body struct {
			Avg    int `json:"avg"`
			Period int `json:"period"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		verify, err := deps.DeviceManager.ApplyDsa3217ScanConfig(id, body.Avg, body.Period)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": verify})
	case r.Method == http.MethodGet && action == "daqP1603Config":
		// DAQ-P-1603 配置回读：已连接设备从 driver 获取最新 profile，
		// 未连接设备返回持久化的 profile（前端用于回显）。
		config, err := deps.DeviceManager.GetDAQP1603Config(id)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": config})
	case r.Method == http.MethodPut && action == "daqP1603Config":
		// DAQ-P-1603 配置应用：已连接设备同步到硬件（ReleaseTask →
		// VerifyParam → InitTask），回读 profile 验证生效值。
		// 未连接设备返回错误（无法同步到未连接设备）。
		var config device.Profile
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		verify, err := deps.DeviceManager.ApplyDAQP1603Config(id, config)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": verify})
	// ---- v2 校零 API ----
	// Calibrate 同步阻塞 5 秒（hub 订阅均值采样），HTTP 客户端需设置 ≥6s 超时。
	// 使用 r.Context() 支持客户端取消（断开连接自动终止采样）。
	case r.Method == http.MethodPut && action == "calibrate":
		var targetChannel *int
		if raw := strings.TrimSpace(r.URL.Query().Get("channelIndex")); raw != "" {
			channelIndex, err := parseChannelIndex(r)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			targetChannel = &channelIndex
		}
		results, err := deps.DeviceManager.Calibrate(id, r.Context(), targetChannel)
		if err != nil {
			// 部分成功场景：Calibrate 返回 (results, err) 表示"已计算但落盘失败"，
			// 此时偏移值已计算但未持久化、未同步到 applier。
			// 返回 207 Multi-Status + warning 字段，让前端知道部分结果已可用，
			// 而非简单 400 丢弃 results。
			if len(results) > 0 {
				writeJSON(w, http.StatusMultiStatus, map[string]any{
					"success": false,
					"data":    results,
					"warning": err.Error(),
				})
				return
			}
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": results})
	case r.Method == http.MethodGet && action == "calibration":
		channelIndex, err := parseChannelIndex(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		record, err := deps.DeviceManager.GetCalibration(id, channelIndex)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, record)
	case r.Method == http.MethodGet && action == "calibrationProgress":
		writeJSON(w, http.StatusOK, deps.DeviceManager.GetCalibrationProgress(id))
	case r.Method == http.MethodPost && action == "clearCalibration":
		channelIndex, err := parseChannelIndex(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := deps.DeviceManager.ClearCalibration(id, channelIndex); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	case r.Method == http.MethodGet && action == "calibrationEnabled":
		channelIndex, err := parseChannelIndex(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		enabled, err := deps.DeviceManager.GetCalibrationEnabled(id, channelIndex)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"enabled": enabled})
	case r.Method == http.MethodPut && action == "calibrationEnabled":
		var body struct {
			ChannelIndex int  `json:"channelIndex"`
			Enabled      bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := deps.DeviceManager.SetCalibrationEnabled(id, body.ChannelIndex, body.Enabled); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func parseChannelIndex(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("channelIndex"))
	if raw == "" {
		return 0, nil
	}
	channelIndex, err := strconv.Atoi(raw)
	if err != nil || channelIndex < 0 {
		return 0, fmt.Errorf("channelIndex must be a non-negative integer")
	}
	return channelIndex, nil
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// marshalDataPayload 手写 device.DataPayload 的 JSON 编码，完全绕过 reflect。
//
// 背景：在 Go 1.26.1 + 高并发（多设备 500ms 轮询）场景下，标准库
// encoding/json 的 structEncoder 偶发出现 fields[i].encoder 与
// fields[i].index 指向的字段类型不匹配（intEncoder 被分派到 string 字段
// DeviceID），导致 "reflect: call of reflect.Value.Int on string Value"
// panic。该问题源于 fieldCache（sync.Map）缓存的 structFields.list 内部
// 不一致，-race 检测器无法捕获。此处通过手写 JSON 输出绕过 structEncoder
// 反射路径，保证高频轮询接口 /api/daq/latest/{id} 的稳定性。
//
// 输出格式与 encoding/json 默认行为对齐：
//   - deviceId: JSON 字符串（strconv.Quote 处理转义）
//   - timestamp: int64 十进制
//   - deviceTimestamp: int64 十进制，omitempty（零值省略）
//   - channels: []float64，nil 输出 null（与 encoding/json 一致）
//   - channelIndices: []int，nil 输出 null
func marshalDataPayload(p device.DataPayload) []byte {
	// 预估容量：deviceId(uuid+引号)≈42, timestamp≈16, channels(16×24)≈400,
	// channelIndices(16×8)≈130, 加上键名和分隔符，初始 512 足够多数场景。
	buf := make([]byte, 0, 512)

	buf = append(buf, '{')

	// deviceId（string）
	buf = append(buf, `"deviceId":`...)
	buf = append(buf, strconv.Quote(p.DeviceID)...)

	// timestamp（int64）
	buf = append(buf, `,"timestamp":`...)
	buf = strconv.AppendInt(buf, p.Timestamp, 10)

	// deviceTimestamp（int64, omitempty：零值省略，与原 json tag 行为一致）
	if p.DeviceTimestamp != 0 {
		buf = append(buf, `,"deviceTimestamp":`...)
		buf = strconv.AppendInt(buf, p.DeviceTimestamp, 10)
	}

	// channels（[]float64；nil 输出 null，非 nil 输出数组）
	// NaN/Inf 输出 null：Go encoding/json 默认会序列化为 "NaN" 字符串（非法 JSON），
	// 且 T1602 用 NaN 表示"通道未接入热电偶"，前端以 null 渲染为 "--"/波形空点。
	buf = append(buf, `,"channels":`...)
	if p.Channels == nil {
		buf = append(buf, `null`...)
	} else {
		buf = append(buf, '[')
		for i, v := range p.Channels {
			if i > 0 {
				buf = append(buf, ',')
			}
			if math.IsNaN(v) || math.IsInf(v, 0) {
				buf = append(buf, `null`...)
				continue
			}
			// 'g' 精度 -1 与 encoding/json floatEncoder(64) 一致
			buf = strconv.AppendFloat(buf, v, 'g', -1, 64)
		}
		buf = append(buf, ']')
	}

	// channelIndices（[]int；nil 输出 null）
	buf = append(buf, `,"channelIndices":`...)
	if p.ChannelIndices == nil {
		buf = append(buf, `null`...)
	} else {
		buf = append(buf, '[')
		for i, v := range p.ChannelIndices {
			if i > 0 {
				buf = append(buf, ',')
			}
			buf = strconv.AppendInt(buf, int64(v), 10)
		}
		buf = append(buf, ']')
	}

	buf = append(buf, '}')
	return buf
}

// writeDataPayloadJSON 用手写编码输出 DataPayload，规避 reflect 路径的 panic。
// 末尾追加换行符，与 json.Encoder.Encode 行为一致。
func writeDataPayloadJSON(w http.ResponseWriter, status int, p device.DataPayload) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	buf := marshalDataPayload(p)
	buf = append(buf, '\n')
	_, _ = w.Write(buf)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"success": false, "error": message})
}

// handleFiveHolePreview 五孔点位预览 handler（spec Task 10）
//
// 接收前端"配置向导"提交的 FiveHolePointLayout，调用 CalibrationManager.PreviewFiveHolePoints
// 生成蛇形/raster 点位列表，返回 bare array（与既有契约一致）。
//
// spec Task 10 修复：原 handleFiveholeSnakePoints 直接调 core.GenerateFiveHoleSnakePoints，
// 违反"API 不直接调用点位生成算法"边界。现统一委托 usecase 层，HTTP/Wails 共用入口。
//
// 与 handleSevenHolePreview 的区别：
//   - 五孔返回 bare array（[]FiveHoleSnakePoint）——历史契约，前端直接迭代
//   - 七孔返回包装对象（{points,totalCount,innerCount,outerCount}）——含聚合统计
//
// 错误处理：
//   - JSON 解码失败 → 400
//   - 配置非法（步长 ≤ 0）→ 400，透传 PreviewFiveHolePoints 错误
//   - 非 POST 方法 → 405
func handleFiveHolePreview(w http.ResponseWriter, r *http.Request, mgr *usecase.CalibrationManager) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body calibration.FiveHolePointLayout
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	points, err := mgr.PreviewFiveHolePoints(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, points)
}

// handleSevenHolePreview 七孔点位预览 handler（spec Task 12）
//
// 接收前端"配置向导"提交的 SevenHoleConfig，调用 CalibrationManager.PreviewSevenHolePoints
// 生成完整点位列表，并按 region 聚合统计内/外区点数，返回 SevenHolePreviewResult 包装。
//
// 与 fivehole 路由的区别（spec Task 10 后两者均委托 usecase）：
//   - fivehole 走 usecase 层 PreviewFiveHolePoints，返回 bare array []FiveHoleSnakePoint
//   - sevenhole-preview 走 usecase 层 PreviewSevenHolePoints，返回带聚合统计的
//     SevenHolePreviewResult（TotalCount/InnerCount/OuterCount）供前端状态栏直接显示
//
// NaN 哨兵清洗（spec §38）：CalPoint.Coordinates/MotionCoordinates 为 map[string]float64，
// Go 的 encoding/json 默认会把 NaN/Inf 序列化为 "NaN"/"+Inf" 字符串（非法 JSON）。
// GenerateSevenHolePoints 实际不会产生 NaN（所有边界检查后的算术运算），
// 但作为防御性兜底，序列化前清洗一遍：NaN/Inf 替换为 nil（JSON null），
// 前端读取 undefined 后可自行处理缺失字段（如内区点缺 θ/φ、外区点缺 α/β）。
//
// 错误处理：
//   - JSON 解码失败 → 400
//   - 配置非法（步长 ≤ 0、范围 min > max）→ 400，透传 GenerateSevenHolePoints 错误
func handleSevenHolePreview(w http.ResponseWriter, r *http.Request, mgr *usecase.CalibrationManager) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body calibration.SevenHoleConfig
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := mgr.PreviewSevenHolePoints(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sanitizeSevenHolePreview(result))
}

// sanitizedCalPoint 七孔点位清洗后结构（spec §38 NaN 哨兵清洗）
//
// Coordinates/MotionCoordinates 用 map[string]any 替代 map[string]float64，
// 让 NaN/Inf 值替换为 nil（JSON null），避免 Go encoding/json 序列化失败。
type sanitizedCalPoint struct {
	ID                int            `json:"id"`
	Coordinates       map[string]any `json:"coordinates"`
	MotionCoordinates map[string]any `json:"motionCoordinates,omitempty"`
	Region            string         `json:"region,omitempty"`
	Sector            int            `json:"sector,omitempty"`
}

// sanitizeSevenHolePreview 清洗 SevenHolePreviewResult 中的 NaN/Inf 值。
//
// 遍历每个 CalPoint 的 Coordinates 和 MotionCoordinates，将 NaN/Inf 替换为 nil，
// 返回新的 sanitizedCalPoint 切片（不影响原始数据，避免污染 usecase 缓存）。
func sanitizeSevenHolePreview(result calibration.SevenHolePreviewResult) map[string]any {
	points := make([]sanitizedCalPoint, 0, len(result.Points))
	for _, p := range result.Points {
		points = append(points, sanitizedCalPoint{
			ID:                p.ID,
			Coordinates:       sanitizeFloatMap(p.Coordinates),
			MotionCoordinates: sanitizeFloatMap(p.MotionCoordinates),
			Region:            p.Region,
			Sector:            p.Sector,
		})
	}
	return map[string]any{
		"points":     points,
		"totalCount": result.TotalCount,
		"innerCount": result.InnerCount,
		"outerCount": result.OuterCount,
	}
}

// sanitizeFloatMap 将 map[string]float64 中的 NaN/Inf 替换为 nil，返回 map[string]any。
//
// nil map 输入返回 nil（保持 omitempty 行为）；空 map 返回空 map（保持 JSON {} 而非 null）。
// 正常值原样保留为 float64，前端可按 typeof===number 判断。
func sanitizeFloatMap(m map[string]float64) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			out[k] = nil
		} else {
			out[k] = v
		}
	}
	return out
}

// handleLogStream 通过 SSE 实时推送后端日志到前端。
// 前端无需重连逻辑，连接断开后重新打开页面即可。
//
// 注意：必须先 Subscribe 再 Recent，否则两步之间产生的日志会丢失（先取快照
// 再订阅期间到达的事件无人接）。Recent 与 Subscribe 都按 entry.ID 单调，
// 这里用 lastID 去重，保证发到前端的条目不重复。
func handleLogStream(w http.ResponseWriter, r *http.Request, ring *logging.RingBuffer) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()

	// 先订阅，避免 Recent 与 Subscribe 之间的事件丢失。
	ch := ring.Subscribe(ctx)

	// 再发送已有历史（最近 200 条），按 ID 去重。
	var lastID uint64
	recent := ring.Recent(200)
	for _, entry := range recent {
		data, err := json.Marshal(entry)
		if err != nil {
			continue
		}
		if _, err := fmt.Fprintf(w, "event: log\ndata: %s\n\n", data); err != nil {
			return
		}
		if entry.ID > lastID {
			lastID = entry.ID
		}
	}
	flusher.Flush()

	// 订阅后续实时日志，写循环显式监听 ctx，避免慢客户端阻塞。
	for {
		select {
		case <-ctx.Done():
			return
		case entry, ok := <-ch:
			if !ok {
				return
			}
			// 跳过已通过 Recent 发出的历史条目
			if entry.ID <= lastID {
				continue
			}
			data, err := json.Marshal(entry)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "event: log\ndata: %s\n\n", data); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// handleLogRecent 返回最近的 N 条日志，供前端页面加载时回灌历史。
// limit 参数控制返回条数，默认 500，最大 2000。
func handleLogRecent(w http.ResponseWriter, r *http.Request, ring *logging.RingBuffer) {
	limit := 500
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 2000 {
			limit = parsed
		}
	}
	entries := ring.Recent(limit)
	if entries == nil {
		entries = []logging.RingEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

// handleLogCategories 处理日志分类开关的读取和设置。
// GET  /api/log/categories        → 返回所有已显式设置的 category 状态
// PUT  /api/log/categories        → 设置指定 category 的启用状态
//
//	body: {"category": "hardware-send", "enabled": false}
func handleLogCategories(w http.ResponseWriter, r *http.Request, mgr *logging.Manager) {
	switch r.Method {
	case http.MethodGet:
		states := map[string]bool{}
		if mgr != nil {
			states = mgr.GetCategoryStates()
		}
		writeJSON(w, http.StatusOK, map[string]any{"states": states})
	case http.MethodPut:
		if mgr == nil {
			writeError(w, http.StatusServiceUnavailable, "日志管理器未初始化，无法设置分类开关")
			return
		}
		var body struct {
			Category string `json:"category"`
			Enabled  bool   `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if body.Category == "" {
			writeError(w, http.StatusBadRequest, "category is required")
			return
		}
		mgr.SetCategoryEnabled(body.Category, body.Enabled)
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
