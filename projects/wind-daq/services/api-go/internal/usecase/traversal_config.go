// Package usecase — traversal 配置层（从 traversal.go 拆分）
//
// 包含：traversalAPIConfig（前端 JSON 形态）、ParseAndStartTraversal（API 入口）、
// 持久化配置加载、isSubState 等子状态判定、角色 → 通道标签映射。
package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"time"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	seveninterp "ai-workspace/shared/algorithms/go/sevenhole/interpolation"
	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
)

// restoreInterpolatorTimeout 启动恢复时单个文件 I/O 的最大等待时间。
// 设备上位机典型 PRB / CSV 文件不超过几 MB，5 秒已足够，超时则放弃恢复
// 并在前端 CheckPreconditions 中暴露超时原因，避免阻塞应用启动。
const restoreInterpolatorTimeout = 5 * time.Second

func (m *TraversalManager) loadPersistedConfig() {
	if m.configStore == nil {
		return
	}
	data, err := m.configStore.LoadConfig(traversalConfigKey)
	if err != nil || data == nil {
		return
	}
	m.mu.Lock()
	m.configRaw = json.RawMessage(data)
	m.mu.Unlock()

	// 同步 probeType 到当前配置（与 SaveConfigRaw 同语义）：
	// 重启后 m.config 为零值，缺省按五孔判定会让已恢复的七孔插值器
	// 在前置检查中被误判为未加载（探针感知的 CheckPreconditions 依赖该字段）。
	var probeTypeProbe struct {
		ProbeType string `json:"probeType"`
	}
	if err := json.Unmarshal(data, &probeTypeProbe); err == nil && probeTypeProbe.ProbeType != "" {
		m.mu.Lock()
		m.config.ProbeType = probeTypeProbe.ProbeType
		m.mu.Unlock()
	}

	// 仅从配置中提取 savePath 并回填断点路径。
	// 插值器恢复改为显式的 RestoreInterpolatorFromPersistedConfig 调用，
	// 避免在装配阶段没有注入 InterpolatorLoader 时阻塞或失败。
	var probe struct {
		SavePath string `json:"savePath"`
	}
	if err := json.Unmarshal(data, &probe); err != nil || probe.SavePath == "" {
		return
	}
	if m.checkpointStore == nil {
		return
	}
	candidate := probe.SavePath + ".checkpoint.json"
	exists, err := m.checkpointStore.Stat(candidate)
	if err != nil || !exists {
		return
	}
	m.mu.Lock()
	m.lastCheckpointPath = candidate
	m.mu.Unlock()
}

// RestoreInterpolatorFromPersistedConfig 启动期异步恢复插值器。
//
// 装配阶段在 SetInterpolatorLoader 之后调用此方法，**在后台 goroutine 中**
// 按持久化配置加载 PRB / CSV / 多 PRB。任何错误都会写入 lastInterpolatorRestoreErr
// 由 CheckPreconditions 暴露给前端；超时 / 加载器未注入 / 配置缺失均视为软失败，
// 不影响主应用启动。
//
// 设计要点：
//   - 异步：磁盘 I/O 不阻塞 NewAppContext / BuildAPIServer 主路径。
//   - 超时：通过 context.WithTimeout(restoreInterpolatorTimeout) 限制阻塞上限。
//   - 单源真相：使用同一份 GetConfigRaw 快照，避免与运行期 SaveConfigRaw 竞争。
func (m *TraversalManager) RestoreInterpolatorFromPersistedConfig() {
	m.mu.RLock()
	data := append(json.RawMessage(nil), m.configRaw...)
	loader := m.interpLoader
	m.mu.RUnlock()

	if len(data) == 0 {
		return
	}
	if loader == nil {
		m.setInterpolatorRestoreErr("启动恢复失败：未注入插值器加载端口 (InterpolatorLoader)")
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), restoreInterpolatorTimeout)
		defer cancel()
		m.restoreInterpolatorFromConfig(ctx, data, loader)
	}()
}

// loadResult 用于把异步 loader 调用的结果（插值器 + 错误）通过 channel
// 投递给等待 ctx.Done() 的主 goroutine，确保即使 I/O 阻塞超过 ctx 截止时间，
// 主 goroutine 也能跳过 SetInterpolator，避免陈旧结果污染运行期状态。
type loadResult struct {
	interpolator coreinterp.Interpolator
	err          error
}

// sevenHoleLoadResult 是 loadResult 的七孔变体（返回类型为 seveninterp.Interpolator）。
type sevenHoleLoadResult struct {
	interpolator seveninterp.Interpolator
	err          error
}

// runSevenHoleLoaderWithTimeout 与 runLoaderWithTimeout 同语义，适配七孔加载签名。
func runSevenHoleLoaderWithTimeout(ctx context.Context, load func() (seveninterp.Interpolator, error)) (seveninterp.Interpolator, error, bool) {
	done := make(chan sevenHoleLoadResult, 1)
	go func() {
		interpolator, err := load()
		done <- sevenHoleLoadResult{interpolator: interpolator, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err(), true
	case res := <-done:
		return res.interpolator, res.err, false
	}
}

// runLoaderWithTimeout 在 goroutine 中执行 loader 调用，等待 ctx.Done 或结果到达。
//
// 返回值约定：
//   - 成功：(interp, nil, false)
//   - 加载失败：(nil, err, false)
//   - 超时：(nil, ctx.Err(), true) —— 标记 timedOut=true，调用方可据此决定不写 SetInterpolator
//
// 注意：底层 os.Open / Read 等系统调用无法被 ctx 真正中断，loader goroutine
// 可能在 ctx 超时后仍继续执行直到 syscall 返回；我们通过 channel 缓冲容量为 1
// 让那个 goroutine 写完就退出，避免泄漏。
func runLoaderWithTimeout(ctx context.Context, load func() (coreinterp.Interpolator, error)) (coreinterp.Interpolator, error, bool) {
	done := make(chan loadResult, 1)
	go func() {
		interpolator, err := load()
		done <- loadResult{interpolator: interpolator, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err(), true
	case res := <-done:
		return res.interpolator, res.err, false
	}
}

// restoreInterpolatorFromConfig 从已保存的配置 JSON 中提取插值文件路径并重新加载插值器。
//
// 优先级：新算法 CSV > 多 PRB > 单 PRB，与前端 TraversalPrbStep 的逻辑一致。
//
// 失败处理：任何分支加载失败都会通过 setInterpolatorRestoreErr 记录原因，
// 由 CheckPreconditions 在 PRB 检查项中暴露给前端，避免前端基于
// configRaw 静默推断为"已加载"而出现状态不一致。
//
// 该函数通过 ports.InterpolatorLoader 接口加载文件，**不直接依赖
// adapters/interpolation 实现包**，遵守工作区六边形分层约束。
func (m *TraversalManager) restoreInterpolatorFromConfig(ctx context.Context, data []byte, loader ports.InterpolatorLoader) {
	// 容错解析：prbFile 和 calibrationCsvFile 字段在前端均为对象结构
	var rawCfg struct {
		ProbeType             string `json:"probeType"`
		InterpolationAlgorithm string `json:"interpolationAlgorithm"`
		PrbFile               struct {
			FilePath string `json:"filePath"`
		} `json:"prbFile"`
		CalibrationCsvFile struct {
			FilePath string `json:"filePath"`
		} `json:"calibrationCsvFile"`
		UseMultiPrb  bool `json:"useMultiPrb"`
		MultiPrb     *struct {
			Files []struct {
				FilePath string `json:"filePath"`
			} `json:"files"`
			MachNumbers       []float64 `json:"machNumbers"`
			InterpolationMode string    `json:"interpolationMode"`
		} `json:"multiPrb"`
		SevenHolePrb *sevenHolePrbConfig `json:"sevenHolePrb"`
	}
	if err := json.Unmarshal(data, &rawCfg); err != nil {
		msg := fmt.Sprintf("启动恢复失败：解析配置 JSON 出错: %v", err)
		slog.Error("interpolator restore failed",
			"component", "traversal",
			"error", msg,
		)
		m.setInterpolatorRestoreErr(msg)
		return
	}

	// 进入加载流程前先清空上一次错误，避免在配置变更后仍保留陈旧消息
	m.setInterpolatorRestoreErr("")

	// 上下文取消（超时）时直接放弃，避免在加载完成后再写入陈旧结果。
	if err := ctx.Err(); err != nil {
		m.setInterpolatorRestoreErr(fmt.Sprintf("启动恢复超时：%v", err))
		return
	}

	// 探针类型边界校验（双变体语义）：未知非空类型报错；五孔字段与
	// sevenHolePrb 并存合法，仅按激活 probeType 恢复对应数据源（§5.2.1 第 6 条），
	// 未激活变体字段仅作持久化数据透传，不进入 usecase。
	if rawCfg.ProbeType != "" && rawCfg.ProbeType != traversal.ProbeTypeFiveHole && rawCfg.ProbeType != traversal.ProbeTypeSevenHole {
		m.setInterpolatorRestoreErr(fmt.Sprintf("启动恢复失败：未知探针类型 %q", rawCfg.ProbeType))
		return
	}

	// 根据插值算法优先级加载（通过 runLoaderWithTimeout 让 ctx 超时真正生效：
	// 超时后即便底层 I/O 仍在阻塞，主 goroutine 也不会调用 SetInterpolator）
	switch {
	case rawCfg.ProbeType == traversal.ProbeTypeSevenHole:
		// 七孔唯一路径：LoadSevenHolePRB（无 CSV/多 PRB 模式，spec §5.4）。
		// 五孔字段与 sevenHolePrb 并存时仅恢复七孔（激活方）。
		seven := rawCfg.SevenHolePrb
		if seven == nil || seven.InnerFile == nil || seven.InnerFile.FilePath == "" || len(seven.OuterFiles) != 6 {
			got := 0
			if seven != nil {
				got = len(seven.OuterFiles)
			}
			msg := fmt.Sprintf("启动恢复失败：七孔文件集不完整（PRB 或校准 CSV 均需 1 个内区文件 + 恰 6 个扇区文件，实际 %d 个）", got)
			slog.Error("interpolator restore failed",
				"component", "traversal",
				"error", msg,
			)
			m.setInterpolatorRestoreErr(msg)
			return
		}
		var outerPaths [6]string
		for i, f := range seven.OuterFiles {
			outerPaths[i] = f.FilePath
		}
		innerPath := seven.InnerFile.FilePath
		// 按 kind 选择数据源：校准 CSV（七孔校准导出）或 .prb 文件集（默认）。
		isCalibrationCsv := seven.Kind == "seven-hole-calibration-csv"
		interpolator, err, timedOut := runSevenHoleLoaderWithTimeout(ctx, func() (seveninterp.Interpolator, error) {
			if isCalibrationCsv {
				return loader.LoadSevenHoleCalibrationCSV(innerPath, outerPaths)
			}
			return loader.LoadSevenHolePRB(innerPath, outerPaths)
		})
		if timedOut {
			msg := fmt.Sprintf("启动恢复超时：加载七孔%s文件集超过 %s", map[bool]string{true: "校准CSV", false: "PRB"}[isCalibrationCsv], restoreInterpolatorTimeout)
			slog.Warn("interpolator restore timed out",
				"component", "traversal",
				"type", "seven_hole_prb",
				"error", msg,
			)
			m.setInterpolatorRestoreErr(msg)
			return
		}
		if err != nil {
			msg := fmt.Sprintf("启动恢复失败：加载七孔%s文件集出错: %v", map[bool]string{true: "校准CSV", false: "PRB"}[isCalibrationCsv], err)
			slog.Error("interpolator restore failed",
				"component", "traversal",
				"type", "seven_hole_prb",
				"error", msg,
			)
			m.setInterpolatorRestoreErr(msg)
			return
		}
		m.SetSevenHoleInterpolator(interpolator)
		slog.Info("interpolator restored from seven-hole file set",
			"component", "traversal",
			"kind", seven.Kind,
			"inner_path", innerPath,
		)

	case rawCfg.InterpolationAlgorithm == "new" && rawCfg.CalibrationCsvFile.FilePath != "":
		// 新算法：从 CSV 标定文件加载
		csvPath := rawCfg.CalibrationCsvFile.FilePath
		interpolator, err, timedOut := runLoaderWithTimeout(ctx, func() (coreinterp.Interpolator, error) {
			return loader.LoadFiveHoleCSV(csvPath)
		})
		if timedOut {
			msg := fmt.Sprintf("启动恢复超时：加载 CSV 文件 %s 超过 %s", csvPath, restoreInterpolatorTimeout)
			slog.Warn("interpolator restore timed out",
				"component", "traversal",
				"type", "csv",
				"path", csvPath,
				"error", msg,
			)
			m.setInterpolatorRestoreErr(msg)
			return
		}
		if err != nil {
			msg := fmt.Sprintf("启动恢复失败：加载 CSV 文件 %s 出错: %v", csvPath, err)
			slog.Error("interpolator restore failed",
				"component", "traversal",
				"type", "csv",
				"path", csvPath,
				"error", msg,
			)
			m.setInterpolatorRestoreErr(msg)
			return
		}
		m.SetInterpolator(interpolator)
		slog.Info("interpolator restored from CSV",
			"component", "traversal",
			"path", csvPath,
		)

	case rawCfg.UseMultiPrb && rawCfg.MultiPrb != nil && len(rawCfg.MultiPrb.Files) > 0:
		// 多 PRB 模式
		filePaths := make([]string, 0, len(rawCfg.MultiPrb.Files))
		for _, f := range rawCfg.MultiPrb.Files {
			if f.FilePath != "" {
				filePaths = append(filePaths, f.FilePath)
			}
		}
		if len(filePaths) == 0 {
			m.setInterpolatorRestoreErr("启动恢复失败：多 PRB 配置中没有有效文件路径")
			return
		}
		mode := coreinterp.MultiPrbInterpolationMode(rawCfg.MultiPrb.InterpolationMode)
		machNumbers := append([]float64(nil), rawCfg.MultiPrb.MachNumbers...)
		interpolator, err, timedOut := runLoaderWithTimeout(ctx, func() (coreinterp.Interpolator, error) {
			return loader.LoadMultiPRB(filePaths, machNumbers, mode)
		})
		if timedOut {
			msg := fmt.Sprintf("启动恢复超时：加载 %d 个多 PRB 文件超过 %s", len(filePaths), restoreInterpolatorTimeout)
			slog.Warn("interpolator restore timed out",
				"component", "traversal",
				"type", "multi_prb",
				"file_count", len(filePaths),
				"error", msg,
			)
			m.setInterpolatorRestoreErr(msg)
			return
		}
		if err != nil {
			msg := fmt.Sprintf("启动恢复失败：加载多 PRB 文件出错: %v", err)
			slog.Error("interpolator restore failed",
				"component", "traversal",
				"type", "multi_prb",
				"file_count", len(filePaths),
				"error", msg,
			)
			m.setInterpolatorRestoreErr(msg)
			return
		}
		m.SetInterpolator(interpolator)
		slog.Info("interpolator restored from multi PRB",
			"component", "traversal",
			"file_count", len(filePaths),
		)

	case rawCfg.PrbFile.FilePath != "":
		// 单 PRB 模式
		prbPath := rawCfg.PrbFile.FilePath
		interpolator, err, timedOut := runLoaderWithTimeout(ctx, func() (coreinterp.Interpolator, error) {
			return loader.LoadPRB(prbPath)
		})
		if timedOut {
			msg := fmt.Sprintf("启动恢复超时：加载 PRB 文件 %s 超过 %s", prbPath, restoreInterpolatorTimeout)
			slog.Warn("interpolator restore timed out",
				"component", "traversal",
				"type", "prb",
				"path", prbPath,
				"error", msg,
			)
			m.setInterpolatorRestoreErr(msg)
			return
		}
		if err != nil {
			msg := fmt.Sprintf("启动恢复失败：加载 PRB 文件 %s 出错: %v", prbPath, err)
			slog.Error("interpolator restore failed",
				"component", "traversal",
				"type", "prb",
				"path", prbPath,
				"error", msg,
			)
			m.setInterpolatorRestoreErr(msg)
			return
		}
		m.SetInterpolator(interpolator)
		slog.Info("interpolator restored from PRB",
			"component", "traversal",
			"path", prbPath,
		)
	}
}

// setInterpolatorRestoreErr 线程安全地写入启动恢复错误消息，
// 空字符串表示无错误。该字段仅由 CheckPreconditions 读取。
func (m *TraversalManager) setInterpolatorRestoreErr(msg string) {
	m.mu.Lock()
	m.lastInterpolatorRestoreErr = msg
	m.mu.Unlock()
}

// isSubState 判断是否为运行中的子状态（moving/stabilizing/acquiring/saving/preparing）。
// RunTraversalLoop 用此区分"运行中"和"已终止"（Idle/Error/Completed/Stopped）。
func isSubState(s traversal.State) bool {
	return s == traversal.StateMoving || s == traversal.StateStabilizing ||
		s == traversal.StateAcquiring || s == traversal.StateSaving || s == traversal.StatePreparing
}

type traversalAPIConfig struct {
	Name   string `json:"name"`
	Layout struct {
		Pattern    string `json:"pattern"`
		SnakeOrder bool   `json:"snakeOrder"`
		// PrimaryAxis 控制矩形/线型布局走线主轴（见 core/traversal.LayoutConfig 注释）：
		// 缺省或 "y" 走 legacy（先走 Y），显式 "x" 才走新逻辑。omitempty 保持空值不序列化。
		PrimaryAxis string `json:"primaryAxis,omitempty"`
		// Line 仅沿 X 轴布点，Y 恒为 0（详见 core/traversal.LineLayout 注释）。
		// 旧配置中残留的 startY/endY/yStepSegments 字段会被 JSON 反序列化时静默忽略。
		Line *struct {
			StartX        float64                 `json:"startX"`
			EndX          float64                 `json:"endX"`
			XStepSegments []traversal.StepSegment `json:"xStepSegments"`
		} `json:"line"`
		Rectangle *struct {
			XMin          float64                 `json:"xMin"`
			XMax          float64                 `json:"xMax"`
			XStepSegments []traversal.StepSegment `json:"xStepSegments"`
			YMin          float64                 `json:"yMin"`
			YMax          float64                 `json:"yMax"`
			YStepSegments []traversal.StepSegment `json:"yStepSegments"`
		} `json:"rectangle"`
		Sector *struct {
			CenterX             float64                 `json:"centerX"`
			CenterY             float64                 `json:"centerY"`
			RadiusMin           float64                 `json:"radiusMin"`
			RadiusMax           float64                 `json:"radiusMax"`
			RadialStepSegments  []traversal.StepSegment `json:"radialStepSegments"`
			AngleStart          float64                 `json:"angleStart"`
			AngleEnd            float64                 `json:"angleEnd"`
			AngularStepSegments []traversal.StepSegment `json:"angularStepSegments"`
		} `json:"sector"`
		Custom *struct {
			// Points 直接复用 traversal.Point，per-point 配置字段（DwellMs/Samples/Test）通过指针携带
			Points []traversal.Point `json:"points"`
		} `json:"custom"`
	} `json:"layout"`
	Channels struct {
		ProbeChannels []struct {
			Name    string `json:"name"`
			Role    string `json:"role"`
			Channel struct {
				DeviceID     string `json:"deviceId"`
				ChannelIndex int    `json:"channelIndex"`
			} `json:"channel"`
			Enabled bool `json:"enabled"`
		} `json:"probeChannels"`
		// MotionAxes 前端配置的运动轴列表，指定哪些轴参与遍历运动。
		// 后端据此过滤 availableAxisTargets，避免对未配置/未接硬件的轴强制归零。
		MotionAxes []struct {
			Name         string `json:"name"`
			ControllerID string `json:"controllerId"`
			Axis         string `json:"axis"`
		} `json:"motionAxes"`
	} `json:"channels"`
	DwellTimeMs       int                             `json:"dwellTimeMs"`
	SamplesPerPoint   int                             `json:"samplesPerPoint"`
	SavePath          string                          `json:"savePath"`
	SaveFileName      string                          `json:"saveFileName"`
	SaveOptions       *traversal.SaveOptions          `json:"saveOptions,omitempty"`
	Validation        *traversal.DataValidationConfig `json:"validation,omitempty"`
	Stabilization     *traversal.StabilizationConfig  `json:"stabilization,omitempty"`
	InterpolationMode string                          `json:"interpolationMode,omitempty"`
	// PProbePressureType 五孔探针 P1-P5 压力类型："gauge"（默认）/ "absolute"。
	// 空串在 ParseAndStartTraversal 中兜底为 "gauge"，保证旧配置兼容。
	PProbePressureType string `json:"pProbePressureType,omitempty"`
	// ProbeType 探针类型："five-hole"（默认，空串归一化）/ "seven-hole"。
	ProbeType string `json:"probeType,omitempty"`
	// SevenHolePrb 七孔 PRB 文件集配置（仅 probeType="seven-hole" 合法；
	// 五孔/空类型携带本字段或未知类型均在边界报错，见 normalizeAndValidateProbeType）。
	SevenHolePrb *sevenHolePrbConfig `json:"sevenHolePrb,omitempty"`
	// MotionSafety 运动安全配置。为空时下游使用 DefaultMotionSafety。
	MotionSafety *traversal.MotionSafetyConfig `json:"motionSafety,omitempty"`
}

// roleToLabel 将前端 ProbeChannelConfig.role 转为压力标签
// 例如 "fiveHole.p1" → "P1"，"fiveHole.pAtm" → "Patm"
func roleToLabel(role, name string) string {
	switch role {
	case "fiveHole.p1":
		return "P1"
	case "fiveHole.p2":
		return "P2"
	case "fiveHole.p3":
		return "P3"
	case "fiveHole.p4":
		return "P4"
	case "fiveHole.p5":
		return "P5"
	case "fiveHole.pAtm":
		return "Patm"
	case "fiveHole.tAtm":
		return "Tatm"
	// 七孔 9 角色（spec-seven-hole-traversal §2.3）：外围 6 孔 + 中心孔 + 大气压力/温度。
	// 与五孔共用同一标签空间（P1..P7/Patm/Tatm），由策略表按探针类型取用。
	case "sevenHole.p1":
		return "P1"
	case "sevenHole.p2":
		return "P2"
	case "sevenHole.p3":
		return "P3"
	case "sevenHole.p4":
		return "P4"
	case "sevenHole.p5":
		return "P5"
	case "sevenHole.p6":
		return "P6"
	case "sevenHole.p7":
		return "P7"
	case "sevenHole.pAtm":
		return "Patm"
	case "sevenHole.tAtm":
		return "Tatm"
	}
	// 回退使用 name 字段作为标签
	return name
}

// sevenHolePrbFileRef 七孔 PRB 文件引用（持久化配置形状，spec §5.4）。
type sevenHolePrbFileRef struct {
	FilePath string `json:"filePath"`
}

// sevenHolePrbConfig 七孔 PRB 文件集配置：1 个内区文件（7.prb）+ 6 个扇区
// 文件（1.prb~6.prb，按孔号顺序）。仅在 probeType="seven-hole" 时合法。
type sevenHolePrbConfig struct {
	// Kind 前端判别联合的 kind 字段；出现时必须为 "seven-hole-prb-set"。
	Kind       string               `json:"kind,omitempty"`
	InnerFile  *sevenHolePrbFileRef `json:"innerFile"`
	OuterFiles []sevenHolePrbFileRef `json:"outerFiles"`
}

// normalizeAndValidateProbeType 在配置边界规范化探针类型（双变体语义）：
//   - 五孔字段与 sevenHolePrb 在配置 JSON 中并存合法，probeType 仅标记激活方；
//   - 空 probeType 归一化为 five-hole（旧配置兼容）；
//   - 未知非空 probeType 报错，不静默降级；
//   - 激活七孔时 sevenHolePrb 必须齐全（kind 合法、1 内区 + 恰 6 扇区路径非空），
//     五孔字段作为未激活变体并存不拦截。
func normalizeAndValidateProbeType(probeType string, seven *sevenHolePrbConfig) (string, error) {
	switch probeType {
	case "":
		return traversal.ProbeTypeFiveHole, nil
	case traversal.ProbeTypeFiveHole:
		return traversal.ProbeTypeFiveHole, nil
	case traversal.ProbeTypeSevenHole:
		if seven == nil {
			return "", fmt.Errorf("七孔配置缺少 sevenHolePrb 文件集")
		}
		if seven.Kind != "" && seven.Kind != "seven-hole-prb-set" && seven.Kind != "seven-hole-calibration-csv" {
			return "", fmt.Errorf("七孔插值配置 kind 必须为 %q 或 %q，实际 %q", "seven-hole-prb-set", "seven-hole-calibration-csv", seven.Kind)
		}
		if seven.InnerFile == nil || seven.InnerFile.FilePath == "" {
			return "", fmt.Errorf("七孔配置缺少内区文件 (sevenHolePrb.innerFile)")
		}
		if len(seven.OuterFiles) != 6 {
			return "", fmt.Errorf("七孔配置扇区文件必须恰为 6 份，实际 %d 份", len(seven.OuterFiles))
		}
		for i, f := range seven.OuterFiles {
			if f.FilePath == "" {
				return "", fmt.Errorf("七孔配置扇区 %d 文件路径为空", i+1)
			}
		}
		return traversal.ProbeTypeSevenHole, nil
	default:
		return "", fmt.Errorf("未知探针类型: %q（仅支持 five-hole / seven-hole）", probeType)
	}
}

// ParseConfig 将前端遍历配置 JSON 解析为内部 traversal.Config。
// 与 ParseAndStartTraversal 共享同一套解析逻辑，但只返回配置不启动任务，
// 供 CheckPreconditions 在启动前基于最新配置校验 Patm/Tatm 等映射。
func (m *TraversalManager) ParseConfig(raw json.RawMessage) (traversal.Config, error) {
	var legacy struct {
		TaskID   string            `json:"taskId"`
		DeviceID string            `json:"deviceId"`
		Channels []int             `json:"channels"`
		Path     []traversal.Point `json:"path"`
	}
	if err := json.Unmarshal(raw, &legacy); err == nil && legacy.TaskID != "" {
		config := traversal.Config{
			TaskID: legacy.TaskID, DeviceID: legacy.DeviceID,
			Channels: legacy.Channels, Path: legacy.Path,
		}
		slog.Info("parsing legacy traversal config",
			"component", "traversal",
			"task_id", config.TaskID,
			"device_id", config.DeviceID,
			"total_points", len(config.Path),
		)
		return config, nil
	}

	var cfg traversalAPIConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		slog.Error("parse traversal config failed",
			"component", "traversal",
			"error", err,
		)
		return traversal.Config{}, fmt.Errorf("invalid request: %w", err)
	}

	layout := traversal.LayoutConfig{
		Pattern:     cfg.Layout.Pattern,
		SnakeOrder:  cfg.Layout.SnakeOrder,
		PrimaryAxis: cfg.Layout.PrimaryAxis,
		Line: func() *traversal.LineLayout {
			if cfg.Layout.Line == nil {
				return nil
			}
			return &traversal.LineLayout{
				StartX:        cfg.Layout.Line.StartX,
				EndX:          cfg.Layout.Line.EndX,
				XStepSegments: cfg.Layout.Line.XStepSegments,
			}
		}(),
		Rectangle: func() *traversal.RectangleLayout {
			if cfg.Layout.Rectangle == nil {
				return nil
			}
			return &traversal.RectangleLayout{
				XMin: cfg.Layout.Rectangle.XMin, XMax: cfg.Layout.Rectangle.XMax,
				XStepSegments: cfg.Layout.Rectangle.XStepSegments,
				YMin:          cfg.Layout.Rectangle.YMin, YMax: cfg.Layout.Rectangle.YMax,
				YStepSegments: cfg.Layout.Rectangle.YStepSegments,
			}
		}(),
		Sector: func() *traversal.SectorLayout {
			if cfg.Layout.Sector == nil {
				return nil
			}
			return &traversal.SectorLayout{
				CenterX: cfg.Layout.Sector.CenterX, CenterY: cfg.Layout.Sector.CenterY,
				RadiusMin: cfg.Layout.Sector.RadiusMin, RadiusMax: cfg.Layout.Sector.RadiusMax,
				RadialStepSegments:  cfg.Layout.Sector.RadialStepSegments,
				AngleStart:          cfg.Layout.Sector.AngleStart,
				AngleEnd:            cfg.Layout.Sector.AngleEnd,
				AngularStepSegments: cfg.Layout.Sector.AngularStepSegments,
			}
		}(),
		Custom: func() *traversal.CustomLayout {
			if cfg.Layout.Custom == nil {
				return nil
			}
			// cfg.Layout.Custom.Points 已是 []traversal.Point（含 per-point 配置指针），
			// 直接 slice copy 保留全部字段，无需逐字段拷贝
			points := make([]traversal.Point, len(cfg.Layout.Custom.Points))
			copy(points, cfg.Layout.Custom.Points)
			return &traversal.CustomLayout{Points: points}
		}(),
	}
	points := traversal.PointsFromLayout(layout)
	if len(points) == 0 {
		return traversal.Config{}, fmt.Errorf("invalid layout: no points generated for pattern %q", layout.Pattern)
	}

	// channels 收集：按「设备+硬件通道索引」检测重复绑定并报错，禁止静默去重。
	//
	// 重复绑定同一物理通道意味着多个 ProbeChannel 抢一路硬件数据，
	// 这会导致两个致命问题：
	//   1. valuesForChannels 用 map[内部键] 去重后长度校验失败（采集超时）
	//   2. BuildRawPressure 用 channelLabels[内部键] 反查压力标签，map 覆盖后
	//      丢失其中一个压力输入（如 P1 被 Patm 覆盖 → P1=0，插值结果静默错误）
	//
	// 注意：不同设备的硬件通道序号允许重复（各设备 profile 均从 0 编号），
	// 只有「同一设备同一通道」被多个探针绑定才报错。
	// 典型触发场景：用户把 P1 和 Patm 都绑到了同一设备的 channelIndex=0。
	// 处理方式：直接返回错误强制用户修正配置，不静默降级。
	channels := make([]int, 0, len(cfg.Channels.ProbeChannels))
	channelLabels := make(map[int]string)
	channelRefs := make(map[int]traversal.ChannelRef)
	// firstProbeName 记录每个物理通道（设备+硬件索引）首次占用者的 probe.Name，
	// 用于重复报错时给出双方 probe 名称，避免歧义的 "channel" 占位符。
	firstProbeName := make(map[string]string, len(cfg.Channels.ProbeChannels))
	deviceID := ""
	seenPhysical := make(map[string]bool, len(cfg.Channels.ProbeChannels))
	usedKeys := make(map[int]bool, len(cfg.Channels.ProbeChannels))
	for _, probe := range cfg.Channels.ProbeChannels {
		if !probe.Enabled || probe.Channel.ChannelIndex < 0 {
			continue
		}
		devID := probe.Channel.DeviceID
		if devID == "" {
			return traversal.Config{}, fmt.Errorf(
				"probe %q: deviceId is required for channel binding", probe.Name,
			)
		}
		if deviceID == "" {
			deviceID = devID
		}
		chIdx := probe.Channel.ChannelIndex
		physKey := devID + "\x00" + strconv.Itoa(chIdx)
		if seenPhysical[physKey] {
			return traversal.Config{}, fmt.Errorf(
				"duplicate channel %d on device %s: probes %q and %q are bound to the same hardware channel",
				chIdx, devID, firstProbeName[physKey], probe.Name,
			)
		}
		seenPhysical[physKey] = true
		firstProbeName[physKey] = probe.Name
		// 内部键分配：优先沿用硬件索引（单设备/无冲突时与历史行为一致），
		// 跨设备序号冲突时分配下一个空闲整数，保证 Channels/Values/ChannelLabels
		// 的 int 键全局唯一；ChannelRefs 记录每个内部键的真实物理通道。
		key := chIdx
		for usedKeys[key] {
			key++
		}
		usedKeys[key] = true
		channels = append(channels, key)
		channelRefs[key] = traversal.ChannelRef{DeviceID: devID, Index: chIdx}
		// 通过 role/name 显式建立 内部键→label 映射，避免依赖通道索引顺序
		if label := roleToLabel(probe.Role, probe.Name); label != "" {
			channelLabels[key] = label
		}
	}

	// 解析 motionAxes：保留逻辑目标名及 controllerId+axis 绑定，仅这些轴参与遍历运动。
	// 空列表（旧配置未传 motionAxes）保持原行为：对所有已连接轴生成目标。
	// 必须保留 controllerId：否则会对其它 autoConnect 的真实控制器也发 MoveTo/等待到位，
	// 导致模拟控制器已到位仍卡在「移动中」直至超时。
	motionAxes := make([]traversal.MotionAxisBinding, 0, len(cfg.Channels.MotionAxes))
	seen := make(map[string]bool, len(cfg.Channels.MotionAxes))
	for _, ma := range cfg.Channels.MotionAxes {
		if ma.Axis == "" {
			continue
		}
		// 同一控制器+轴去重；不同控制器可绑定同名轴
		key := ma.ControllerID + "\x00" + ma.Axis
		if seen[key] {
			continue
		}
		seen[key] = true
		motionAxes = append(motionAxes, traversal.MotionAxisBinding{
			Name:         ma.Name,
			ControllerID: ma.ControllerID,
			Axis:         ma.Axis,
		})
	}
	if deviceID == "" {
		err := fmt.Errorf("deviceId is required")
		slog.Error("parse traversal config failed", "component", "traversal", "error", err)
		return traversal.Config{}, err
	}
	if len(channels) == 0 {
		err := fmt.Errorf("channels are required")
		slog.Error("parse traversal config failed", "component", "traversal", "error", err)
		return traversal.Config{}, err
	}
	if len(points) == 0 {
		err := fmt.Errorf("path is required")
		slog.Error("parse traversal config failed", "component", "traversal", "error", err)
		return traversal.Config{}, err
	}

	dwell := time.Duration(cfg.DwellTimeMs) * time.Millisecond
	if dwell < 0 {
		dwell = 0
	}
	samplesPerPoint := cfg.SamplesPerPoint
	if samplesPerPoint <= 0 {
		samplesPerPoint = 1
	}
	// 校验运动安全配置——校验失败时直接返回错误，不获取工作流锁、不创建文件、不下发运动命令。
	// 必须在构造 config 前完成，避免污染状态机。
	if cfg.MotionSafety != nil {
		if err := validateMotionSafetyConfig(cfg.MotionSafety, motionAxes); err != nil {
			slog.Error("parse traversal config: invalid motionSafety",
				"component", "traversal",
				"error", err,
			)
			return traversal.Config{}, fmt.Errorf("invalid motionSafety: %w", err)
		}
	}
	// 探针类型规范化与互斥校验（spec §2.3）：未知非空类型、五孔携带
	// sevenHolePrb、七孔文件集不齐均在边界报错，不进入后续流程。
	probeType, err := normalizeAndValidateProbeType(cfg.ProbeType, cfg.SevenHolePrb)
	if err != nil {
		slog.Error("parse traversal config: invalid probeType",
			"component", "traversal",
			"error", err,
		)
		return traversal.Config{}, err
	}
	config := traversal.Config{
		TaskID:            fmt.Sprintf("trav-%d", time.Now().UnixMilli()),
		DeviceID:          deviceID,
		LayoutPattern:     layout.Pattern,
		Channels:          channels,
		Path:              points,
		DwellTimeMs:       cfg.DwellTimeMs,
		SamplesPerPoint:   samplesPerPoint,
		SavePath:          cfg.SavePath,
		SaveFileName:      cfg.SaveFileName,
		SaveOptions:       cfg.SaveOptions,
		ChannelLabels:     channelLabels,
		ChannelRefs:       channelRefs,
		InterpolationMode: cfg.InterpolationMode,
		MotionAxes:        motionAxes,
		MotionSafety:      cfg.MotionSafety,
		ProbeType:         probeType,
	}
	// 压力类型兜底：空串与缺失均落 "gauge"，与历史行为一致，避免归一化逻辑误判绝压。
	pressureType := cfg.PProbePressureType
	if pressureType != "gauge" && pressureType != "absolute" {
		pressureType = "gauge"
	}
	config.PProbePressureType = pressureType
	// 注入数据验证与稳定等待配置（前端可选传入）
	m.SetValidation(cfg.Validation)
	m.SetStabilization(cfg.Stabilization)

	slog.Info("parsing traversal config",
		"component", "traversal",
		"task_id", config.TaskID,
		"device_id", config.DeviceID,
		"total_points", len(points),
		"layout_pattern", cfg.Layout.Pattern,
		"samples_per_point", samplesPerPoint,
		"dwell_ms", cfg.DwellTimeMs,
		"channels", config.Channels,
		"probe_channels_input_count", len(cfg.Channels.ProbeChannels),
	)

	return config, nil
}

func (m *TraversalManager) ParseAndStartTraversal(raw json.RawMessage) (string, error) {
	config, err := m.ParseConfig(raw)
	if err != nil {
		return "", err
	}

	// 后端再次校验采集态，防止绕过前端禁用按钮或确认对话框直接调用启动 API。
	// 遍历测试不再隐式发送 StartAcquisition，设备采集生命周期由操作员管理。
	// 多设备通道绑定（如五孔在设备 A、大气压/温度在设备 B）时逐台校验，
	// 任一设备未采集都拒绝启动，避免运行到首个点才暴露采样超时。
	m.mu.RLock()
	acqController := m.acquisitionController
	m.mu.RUnlock()
	if acqController != nil {
		checked := make(map[string]bool)
		for _, ref := range config.ResolvedChannelRefs() {
			if checked[ref.DeviceID] {
				continue
			}
			checked[ref.DeviceID] = true
			if !acqController.IsAcquiring(ref.DeviceID) {
				return "", fmt.Errorf("device %s is not acquiring; start acquisition before traversal", ref.DeviceID)
			}
		}
	}
	if config.LayoutPattern == "sector" {
		if m.motion == nil {
			return "", fmt.Errorf("sector origin check requires a motion manager")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		statuses := m.motion.StatusAll(ctx)
		cancel()
		if err := validateSectorOrigin(statuses, config.MotionAxes, config.MotionSafety); err != nil {
			return "", err
		}
	}

	if err := m.Start(config); err != nil {
		return "", err
	}
	m.SaveConfigRaw(raw)
	go m.RunTraversalLoop()
	slog.Info("traversal loop launched from ParseAndStartTraversal",
		"component", "traversal",
		"task_id", config.TaskID,
		"dwell_ms", config.DwellTimeMs,
	)
	return config.TaskID, nil
}

// motionCompletePoll 运动到位轮询间隔（毫秒）。
// 在 traversal_acquisition.go 中定义为 const，这里仅供 validateMotionSafetyConfig 引用。
// 为避免循环，将值拷贝在此并加测试断言二者一致。
const motionCompletePollMsForValidation = 100

// validateMotionSafetyConfig 校验 MotionSafetyConfig 合法性。
//
// 校验规则：
//  1. 浮点字段必须有限（非 NaN、非 Inf）
//  2. ArrivalTolerance > 0、ProgressEpsilon > 0
//  3. CriticalDeviationLimit > ArrivalTolerance（避免阈值倒置导致永远走不到 Deviation 分支）
//  4. NoProgressTimeoutMs >= 2*motionCompletePollMsForValidation（避免看门狗比轮询还快误触发）
//  5. AxisOverrides 键必须是 motionAxes 中已绑定的轴名
//  6. AxisOverrides 项内不允许嵌套 AxisOverrides（递归无意义）
//  7. 每个绑定轴 Resolve(axis) 后的合并值必须满足 CriticalDeviationLimit > ArrivalTolerance
//     （防止"全局 + 轴覆盖"组合倒置绕过单对象校验）
//
// 校验失败时返回错误，调用方应在创建任何文件/状态机副作用前返回。
func validateMotionSafetyConfig(cfg *traversal.MotionSafetyConfig, motionAxes []traversal.MotionAxisBinding) error {
	if cfg == nil {
		return nil
	}
	if err := validateMotionSafetyFields(cfg, ""); err != nil {
		return err
	}

	// 校验 AxisOverrides
	boundAxes := make(map[string]bool, len(motionAxes))
	for _, b := range motionAxes {
		if b.Axis != "" {
			boundAxes[b.Axis] = true
		}
	}
	for axisName, override := range cfg.AxisOverrides {
		if !boundAxes[axisName] {
			return fmt.Errorf("axisOverrides[%q]: axis not bound in motionAxes", axisName)
		}
		if override == nil {
			continue
		}
		// 递归覆盖无意义且增加解析复杂度
		if len(override.AxisOverrides) > 0 {
			return fmt.Errorf("axisOverrides[%q]: nested axisOverrides not allowed", axisName)
		}
		if err := validateMotionSafetyFields(override, axisName); err != nil {
			return err
		}
	}

	// 跨字段合并校验：对每个绑定轴调用 Resolve(axis)，校验解析后的合并值满足跨字段约束。
	// 必要性：validateMotionSafetyFields 只校验"同一对象内"的 criticalDeviationLimit > arrivalTolerance，
	// 无法覆盖"全局 arrivalTolerance + 轴覆盖 criticalDeviationLimit"的组合——
	// 例如全局 arrivalTolerance=10、X 轴覆盖 criticalDeviationLimit=5 会通过单对象校验，
	// 但 Resolve(X) 后得到 arrivalTolerance=10, criticalDeviationLimit=5，阈值倒置，
	// 运行时偏差 5–10 会先被到位检查接受，不会触发急停。
	// Resolve 返回的配置所有字段都有值（默认兜底），此处只校验跨字段约束，
	// 单字段范围校验已由上面的 validateMotionSafetyFields 完成。
	for _, b := range motionAxes {
		if b.Axis == "" {
			continue
		}
		resolved := cfg.Resolve(b.Axis)
		if *resolved.CriticalDeviationLimit <= *resolved.ArrivalTolerance {
			return fmt.Errorf("axis %q: resolved criticalDeviationLimit (%v) must be > arrivalTolerance (%v) after merging global + override",
				b.Axis, *resolved.CriticalDeviationLimit, *resolved.ArrivalTolerance)
		}
	}
	return nil
}

// validateMotionSafetyFields 校验单个 MotionSafetyConfig 的字段合法性。
// prefix 用于错误信息定位（"" 表示全局，轴名表示按轴覆盖）。
func validateMotionSafetyFields(cfg *traversal.MotionSafetyConfig, prefix string) error {
	pfx := ""
	if prefix != "" {
		pfx = "axisOverrides[" + prefix + "]."
	}
	if cfg.ArrivalTolerance != nil {
		v := *cfg.ArrivalTolerance
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("%sarrivalTolerance: must be finite, got %v", pfx, v)
		}
		if v <= 0 {
			return fmt.Errorf("%sarrivalTolerance: must be > 0, got %v", pfx, v)
		}
	}
	if cfg.CriticalDeviationLimit != nil {
		v := *cfg.CriticalDeviationLimit
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("%scriticalDeviationLimit: must be finite, got %v", pfx, v)
		}
		if v <= 0 {
			return fmt.Errorf("%scriticalDeviationLimit: must be > 0, got %v", pfx, v)
		}
		// 阈值关系：CriticalDeviationLimit > ArrivalTolerance
		if cfg.ArrivalTolerance != nil && v <= *cfg.ArrivalTolerance {
			return fmt.Errorf("%scriticalDeviationLimit (%v) must be > arrivalTolerance (%v)", pfx, v, *cfg.ArrivalTolerance)
		}
	}
	if cfg.NoProgressTimeoutMs != nil {
		v := *cfg.NoProgressTimeoutMs
		if v < 2*motionCompletePollMsForValidation {
			return fmt.Errorf("%snoProgressTimeoutMs: must be >= %d (2x poll interval), got %d",
				pfx, 2*motionCompletePollMsForValidation, v)
		}
	}
	if cfg.ProgressEpsilon != nil {
		v := *cfg.ProgressEpsilon
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("%sprogressEpsilon: must be finite, got %v", pfx, v)
		}
		if v <= 0 {
			return fmt.Errorf("%sprogressEpsilon: must be > 0, got %v", pfx, v)
		}
	}
	return nil
}

// 任何退出路径都会调用 sink.FinalizeTraversal 关闭文件，保证落盘
