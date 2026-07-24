package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	motionhttp "shared.local/motion-control/go/httpapi"
	configadapter "wind-daq/services/api-go/internal/adapters/config"
	interpfiles "wind-daq/services/api-go/internal/adapters/interpolation"
	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/core/device"
	wind_report "wind-daq/services/api-go/internal/core/report"
	"wind-daq/services/api-go/internal/core/storage"
	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
	"wind-daq/services/api-go/internal/usecase"
	"wind-daq/services/api-go/pkg/logging"
)

type Deps struct {
	DeviceManager      *usecase.DeviceManager
	AcquisitionHub     *usecase.AcquisitionHub
	ReportManager      *usecase.ReportManager
	MotionManager      ports.MotionManager
	MotionService      motionhttp.MotionService
	CalibrationManager *usecase.CalibrationManager
	TraversalManager   *usecase.TraversalManager
	StorageRecorder    *usecase.StorageRecorder
	ConfigManager      *usecase.ConfigManager
	LogRing            *logging.RingBuffer
	LogManager         *logging.Manager // 用于日志分类开关 API

	// AppHandler 由 backend.App 实现，提供应用层 HTTP 端点（version/startup-mode/open-motion-window/resolve-path）。
	// 为 nil 时 /api/app/* 路由不注册，避免 nil 调用 panic。
	AppHandler AppHandler

	// OnAcquisitionStarted 在设备采集成功启动后异步调用。
	// backend.App 用它实现“采集启动后自动开始录制”的业务策略（读 storage-settings.autoStartOnAcquisition）。
	// 为 nil 时采集启动后无副作用，与原 Wails DeviceStartAcquisition 行为对齐。
	OnAcquisitionStarted func(deviceID string)
}

// AppHandler 由桌面壳层（backend.App）实现，注入到 api.Deps 暴露应用层 HTTP 端点。
// 接口隔离避免 api 包反向依赖 desktop-wails/backend。
type AppHandler interface {
	// Version 返回应用版本信息（名称 + 版本号）
	Version() AppVersionInfo
	// StartupMode 返回启动模式：“normal”（主窗口）或 “motion”（运动控制器独立窗口）
	StartupMode() string
	// OpenMotionWindow 启动运动控制器独立窗口子进程
	OpenMotionWindow() error
	// ResolvePath 将相对路径解析到用户可写目录
	ResolvePath(p string) (string, error)
}

// AppVersionInfo 是 GET /api/app/version 的响应体
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
	mux.HandleFunc("/api/daq/latest/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/daq/latest/")
		payload, ok := deps.AcquisitionHub.GetLatestData(id)
		if !ok {
			// 设备无数据时返回仅含 deviceId 的空 payload，保持与前端契约一致
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
			// 读取整个请求体后用 adapters/config 的解码器转换为 core 层的 calibration.Config。
			// 这里不直接 json.Decode 进 calibration.Config，因为前端发送的探针通道是嵌套
			// channel 格式，而 core 层禁止自带 UnmarshalJSON（零容忍约束），解码逻辑必须
			// 在 adapters/config 层完成。
			data, err := io.ReadAll(r.Body)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			body, err := configadapter.DecodeCalibrationConfig(data)
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
			handleFiveholeSnakePoints(w, r)
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
		action := strings.TrimPrefix(r.URL.Path, "/api/traversal/")
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
			interpolator, err := interpfiles.LoadPrbFile(body.FilePath)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			deps.TraversalManager.SetInterpolator(interpolator)
			writeJSON(w, http.StatusOK, prbFileInfo(body.FilePath, interpolator.GetValidRange()))
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
			interpolator, err := interpfiles.LoadFiveHoleNewFile(body.FilePath)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			deps.TraversalManager.SetInterpolator(interpolator)
			rangeInfo := interpolator.GetValidRange()
			writeJSON(w, http.StatusOK, map[string]any{
				"filePath":   body.FilePath,
				"fileName":   filepath.Base(body.FilePath),
				"loadedAt":   time.Now().UnixMilli(),
				"validRange": rangeInfo,
				"pointCount": interpolator.GetPointCount(),
			})
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
			interpolator, result, err := interpfiles.LoadMultiPrbFiles(body.FilePaths, body.MachNumbers)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if body.InterpolationMode != "" {
				interpolator.SetInterpolationMode(coreinterp.MultiPrbInterpolationMode(body.InterpolationMode))
			}
			deps.TraversalManager.SetInterpolator(interpolator)
			files := make([]map[string]any, 0, len(result.Files))
			for i, file := range result.Files {
				var machNumber any
				if i < len(result.MachNumbers) {
					machNumber = result.MachNumbers[i]
				}
				files = append(files, map[string]any{
					"filePath":   file.FilePath,
					"fileName":   file.FileName,
					"loadedAt":   time.Now().UnixMilli(),
					"validRange": file.ValidRange,
					"machNumber": machNumber,
				})
			}
			writeJSON(w, http.StatusOK, map[string]any{"files": files, "machNumbers": result.MachNumbers, "warnings": result.Warnings})
		case "calculateRealtime":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var body struct {
				Pressures coreinterp.InterpolationInput `json:"pressures"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			result, err := deps.TraversalManager.CalculateRealtime(body.Pressures)
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
			writeJSON(w, http.StatusOK, deps.TraversalManager.CheckPreconditions())
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
		case "loadCheckpoint":
			// 加载断点恢复信息（前端启动时调用，判断是否需要展示"恢复"横幅）
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			checkpoint, err := deps.TraversalManager.LoadCheckpoint()
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, checkpoint)
		case "resumeFromCheckpoint":
			// 从断点恢复测试（复用原 taskId，从已完成点数继续）
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var cp traversal.Checkpoint
			if err := json.NewDecoder(r.Body).Decode(&cp); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			taskID, err := deps.TraversalManager.ResumeFromCheckpoint(cp)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"taskId": taskID})
		case "clearCheckpoint":
			// 清除断点文件（用户主动放弃恢复时调用）
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			deps.TraversalManager.ClearCheckpoint()
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
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
	// ---- 应用层端点（version / startup-mode / open-motion-window / resolve-path） ----
	// 仅在注入 AppHandler 时注册，避免 nil 调用 panic。
	if deps.AppHandler != nil {
		mux.HandleFunc("/api/app/version", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			writeJSON(w, http.StatusOK, deps.AppHandler.Version())
		})
		mux.HandleFunc("/api/app/startup-mode", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"mode": deps.AppHandler.StartupMode()})
		})
		mux.HandleFunc("/api/app/open-motion-window", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if err := deps.AppHandler.OpenMotionWindow(); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		})
		mux.HandleFunc("/api/app/resolve-path", func(w http.ResponseWriter, r *http.Request) {
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
			writeJSON(w, http.StatusOK, map[string]string{"path": resolved})
		})
	}

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
		// 采集启动成功后异步触发自动录制检查（保留原 Wails DeviceStartAcquisition 行为）。
		if deps.OnAcquisitionStarted != nil {
			go deps.OnAcquisitionStarted(id)
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	case r.Method == http.MethodPost && action == "subscribe":
		// 标记订阅意图，前端通过 /api/daq/latest/{id} 轮询拉取数据。
		// 当前实现为空操作，保留以维持前端 subscribeStream 契约（参考原 Wails DeviceSubscribeStream）。
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
	case r.Method == http.MethodPut && action == "tare":
		var body struct {
			ChannelIndex int     `json:"channelIndex"`
			Offset       float64 `json:"offset"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := deps.DeviceManager.SetTare(id, body.ChannelIndex, body.Offset); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	case r.Method == http.MethodGet && action == "tare":
		channelIndex, err := parseChannelIndex(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		offset, err := deps.DeviceManager.GetTare(id, channelIndex)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]float64{"offset": offset})
	case r.Method == http.MethodPost && action == "clearTare":
		channelIndex, err := parseChannelIndex(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := deps.DeviceManager.ClearTare(id, channelIndex); err != nil {
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

func decodeBody(w http.ResponseWriter, r *http.Request, v any) (ok bool) {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return false
	}
	return true
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
	buf = append(buf, `,"channels":`...)
	if p.Channels == nil {
		buf = append(buf, `null`...)
	} else {
		buf = append(buf, '[')
		for i, v := range p.Channels {
			if i > 0 {
				buf = append(buf, ',')
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

func handleFiveholeSnakePoints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body calibration.FiveHolePointLayout
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	points, err := calibration.GenerateFiveHoleSnakePoints(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, points)
}

func prbFileInfo(filePath string, validRange coreinterp.PrbValidRange) map[string]any {
	return map[string]any{
		"filePath":   filePath,
		"fileName":   filepath.Base(filePath),
		"loadedAt":   time.Now().UnixMilli(),
		"validRange": validRange,
	}
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
//       body: {"category": "hardware-send", "enabled": false}
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
