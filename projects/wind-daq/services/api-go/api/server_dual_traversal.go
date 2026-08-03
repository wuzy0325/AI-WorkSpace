package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
	"wind-daq/services/api-go/internal/usecase"
)

// 双探针 probe-scoped 路由解析与 registry façade dispatcher（Task 12）。
//
// 规格：docs/specs/dual-traversal-spec.md FR4；docs/specs/tasks-dual-traversal.md Task 12。
//
// 规则：
//   - /api/traversal/{probeId}/{action} 两段路径命中本 dispatcher；
//     单段 legacy 路径不进入本文件（server.go 分流），禁止隐式转发到 probe1；
//   - 生命周期 action（start/runPoint/pause/resume/stop/close）只调用 registry façade，handler 不直接调用
//     manager 生命周期方法（spec I2）；
//   - 只读 config/status/result 与导入/实时计算等非生命周期操作经
//     registry.GetOrCreate 选择 manager 后委托共享 action handler；

// TraversalRegistry api 层依赖的窄 registry 接口（装配根注入 *usecase.ManagerRegistry）。
type TraversalRegistry interface {
	GetOrCreate(probeID usecase.ProbeID) (usecase.ManagedTraversalManager, error)
	Start(ctx context.Context, probeID usecase.ProbeID, rawConfig json.RawMessage) (string, error)
	RunPoint(ctx context.Context, probeID usecase.ProbeID) error
	Pause(ctx context.Context, probeID usecase.ProbeID) error
	Resume(ctx context.Context, probeID usecase.ProbeID) error
	Stop(ctx context.Context, probeID usecase.ProbeID) error
	CloseProbe(ctx context.Context, probeID usecase.ProbeID) error
}

// traversalSharedManager 非生命周期共享操作所需的 manager 方法集。
// *usecase.TraversalManager 天然满足；registry 返回的 managed manager 经类型断言获取。
type traversalSharedManager interface {
	usecase.ManagedTraversalManager
	BuildStatusResponse() map[string]any
	CheckPreconditions(config *traversal.Config) map[string]any
	ImportPRB(filePath string) (*usecase.PrbImportResult, error)
	ImportCalibrationCSV(filePath string) (*usecase.CalibrationCsvImportResult, error)
	ImportMultiPRB(filePaths []string, machNumbers []float64, mode string) (*usecase.MultiPrbImportResult, error)
	ImportSevenHolePRB(innerPath string, outerPaths []string) (*usecase.SevenHoleImportResult, error)
	ImportSevenHoleCalibrationCSV(innerPath string, outerPaths []string) (*usecase.SevenHoleImportResult, error)
	ClearProbeInterpolator(probeType string) error
	CalculateRealtimeForAPI(probeType string, in usecase.ProbePressureInput) (any, error)
}

// handleDualTraversal 两段 probe-scoped 路径的 dispatcher 入口。
// rest 为 TrimPrefix("/api/traversal/") 之后的部分（含 "/"，形如 "probe1/start"）。
func handleDualTraversal(w http.ResponseWriter, r *http.Request, deps Deps, rest string) {
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		// 多余路径段或缺失 action：稳定 404。
		w.WriteHeader(http.StatusNotFound)
		return
	}
	probeID, err := usecase.ParseProbeID(parts[0])
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if deps.TraversalRegistry == nil {
		writeError(w, http.StatusServiceUnavailable, "traversal registry is required")
		return
	}
	dispatchDualAction(w, r, deps, probeID, parts[1])
}

// dispatchDualAction 按 action 分派：生命周期走 registry façade，其余走共享 handler。
func dispatchDualAction(w http.ResponseWriter, r *http.Request, deps Deps, probeID usecase.ProbeID, action string) {
	switch action {
	case "start":
		dualStart(w, r, deps, probeID)
	case "runPoint":
		dualManagerCall(w, r, deps, probeID, deps.TraversalRegistry.RunPoint)
	case "pause":
		dualManagerCall(w, r, deps, probeID, deps.TraversalRegistry.Pause)
	case "resume":
		dualManagerCall(w, r, deps, probeID, deps.TraversalRegistry.Resume)
	case "stop":
		dualManagerCall(w, r, deps, probeID, deps.TraversalRegistry.Stop)
	case "close":
		dualClose(w, r, deps, probeID)
	default:
		dispatchDualSharedAction(w, r, deps, probeID, action)
	}
}

// dispatchDualSharedAction 非生命周期 action（只读/导入/实时计算）分派。
func dispatchDualSharedAction(w http.ResponseWriter, r *http.Request, deps Deps, probeID usecase.ProbeID, action string) {
	switch action {
	case "config":
		dualConfig(w, r, deps, probeID)
	case "status":
		dualStatus(w, r, deps, probeID)
	case "result":
		dualResult(w, r, deps, probeID)
	case "importPrb":
		dualImport(w, r, deps, probeID, func(m traversalSharedManager, body importFileBody) (any, error) {
			return m.ImportPRB(body.FilePath)
		})
	case "importCalibrationCsv":
		dualImport(w, r, deps, probeID, func(m traversalSharedManager, body importFileBody) (any, error) {
			return m.ImportCalibrationCSV(body.FilePath)
		})
	case "importMultiPrb":
		dualImportMultiPrb(w, r, deps, probeID)
	case "importSevenHolePrb":
		dualImportSevenHole(w, r, deps, probeID, false)
	case "importSevenHoleCalibrationCsv":
		dualImportSevenHole(w, r, deps, probeID, true)
	case "clearInterpolator":
		dualClearInterpolator(w, r, deps, probeID)
	case "calculateRealtime":
		dualCalculateRealtime(w, r, deps, probeID)
	case "checkPreconditions":
		dualCheckPreconditions(w, r, deps, probeID)
	default:
		// 未知 action（含 generateGrid 的 probe 形式）：稳定 404。
		w.WriteHeader(http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// 生命周期 action（仅 registry façade）
// ---------------------------------------------------------------------------

func dualStart(w http.ResponseWriter, r *http.Request, deps Deps, probeID usecase.ProbeID) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	taskID, err := deps.TraversalRegistry.Start(r.Context(), probeID, raw)
	if err != nil {
		writeRegistryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"taskId": taskID})
}

// dualManagerCall 无请求体的单参数生命周期 action（runPoint/pause/resume/stop）。
func dualManagerCall(w http.ResponseWriter, r *http.Request, deps Deps, probeID usecase.ProbeID,
	call func(context.Context, usecase.ProbeID) error) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := call(r.Context(), probeID); err != nil {
		writeRegistryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// dualClose 委托 registry 在 admission gate 内原子判定活动状态并关闭终态 manager。
func dualClose(w http.ResponseWriter, r *http.Request, deps Deps, probeID usecase.ProbeID) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := deps.TraversalRegistry.CloseProbe(r.Context(), probeID); err != nil {
		writeRegistryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// ---------------------------------------------------------------------------
// 错误码映射（spec FR4 / Task 12）
// ---------------------------------------------------------------------------

// writeRegistryError 把 registry façade 的 sentinel 错误映射为稳定 HTTP 状态码：
// 400 invalid_probe_id / 503 manager_creation_failed / 409 resource_conflict /
// 409 already_running / 503 registry_closing / 409 recoverable_task_exists /
// 409 checkpoint_version_mismatch / 503 registry_transitioning / 409 probe_closing。
func writeRegistryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalidProbeID):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, usecase.ErrResourceConflict),
		errors.Is(err, usecase.ErrAlreadyRunning),
		errors.Is(err, usecase.ErrProbeClosing),
		errors.Is(err, ports.ErrRecoverableTaskExists),
		errors.Is(err, ports.ErrCheckpointVersionMismatch):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, usecase.ErrRegistryClosing), errors.Is(err, usecase.ErrRegistryTransitioning):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	default:
		// GetOrCreate factory 失败等装配错误：503 manager_creation_failed。
		writeError(w, http.StatusServiceUnavailable, "manager_creation_failed: "+err.Error())
	}
}
