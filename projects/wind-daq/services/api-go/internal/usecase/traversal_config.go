// Package usecase — traversal 配置层（从 traversal.go 拆分）
//
// 包含：traversalAPIConfig（前端 JSON 形态）、ParseAndStartTraversal（API 入口）、
// 持久化配置加载、isSubState 等子状态判定、角色 → 通道标签映射。
package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"shared.local/device-sdk/go/pkg/slog"
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

// 七孔 PRB 文件集 kind 字段值：前端按判别联合选择数据源，
// 后端在边界校验（normalizeAndValidateProbeType）与恢复路径（restoreSevenHoleFromConfig）
// 都需引用同一组常量，避免字面量散落导致拼写漂移。
const (
	// KindSevenHolePrbSet 七孔 .prb 文件集（内区 1 份 + 外区 6 份）。
	KindSevenHolePrbSet = "seven-hole-prb-set"
	// KindSevenHoleCalibrationCsv 七孔校准 CSV 文件集（与 .prb 集同结构，加载路径不同）。
	KindSevenHoleCalibrationCsv = "seven-hole-calibration-csv"
)

func (m *TraversalManager) loadPersistedConfig() {
	if m.configStore == nil {
		return
	}
	data, err := m.configStore.LoadConfig(m.currentConfigKey())
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
}

// RestoreInterpolatorFromPersistedConfig 启动期异步恢复插值器。
//
// 装配阶段在 SetInterpolatorLoader 之后调用此方法，**在后台 goroutine 中**
// 按持久化配置加载 PRB / CSV / 多 PRB。任何错误都会写入按探针类型分桶的
// lastFiveHoleRestoreErr / lastSevenHoleRestoreErr，由 CheckPreconditions
// 经 InterpolatorRestoreErrFor(probeType) 暴露给前端；超时 / 加载器未注入 /
// 配置缺失均视为软失败，不影响主应用启动。
//
// 设计要点：
//   - 异步：磁盘 I/O 不阻塞 NewAppContext / BuildAPIServer 主路径。
//   - 超时：通过 context.WithTimeout(restoreInterpolatorTimeout) 限制阻塞上限。
//   - 单源真相：使用同一份 GetConfigRaw 快照，避免与运行期 SaveConfigRaw 竞争。
//   - 双变体并行：五孔与七孔字段独立恢复，避免"切到未恢复侧即报未加载"的状态不一致
//     （详见 restoreInterpolatorFromConfig 注释）。
//   - 版本保护：捕获启动时的 fiveHoleRestoreEpoch / sevenHoleRestoreEpoch，
//     传给 restore 路径，写回前比对——若用户在恢复期间显式导入/清除了对应变体
//     的插值器（epoch 递增），跳过写回以保护用户最新状态。
func (m *TraversalManager) RestoreInterpolatorFromPersistedConfig() {
	m.mu.RLock()
	data := append(json.RawMessage(nil), m.configRaw...)
	loader := m.interpLoader
	fiveEpoch := m.fiveHoleRestoreEpoch
	sevenEpoch := m.sevenHoleRestoreEpoch
	m.mu.RUnlock()

	if len(data) == 0 {
		return
	}
	if loader == nil {
		// 加载器缺失同时阻塞两侧变体恢复，按类型分桶写入便于 CheckPreconditions
		// 无论激活侧是五孔还是七孔都能读到根因。
		msg := "启动恢复失败：未注入插值器加载端口 (InterpolatorLoader)"
		m.setFiveHoleRestoreErr(msg)
		m.setSevenHoleRestoreErr(msg)
		return
	}

	go func() {
		m.restoreInterpolatorFromConfig(data, loader, fiveEpoch, sevenEpoch)
	}()
}

// RestoreInterpolatorFromPersistedConfigSync restores persisted interpolators before returning.
// It preserves the legacy bounded, soft-failure behavior while giving managed factories an
// explicit publication barrier before the manager becomes observable.
func (m *TraversalManager) RestoreInterpolatorFromPersistedConfigSync() {
	m.mu.RLock()
	data := append(json.RawMessage(nil), m.configRaw...)
	loader := m.interpLoader
	fiveEpoch := m.fiveHoleRestoreEpoch
	sevenEpoch := m.sevenHoleRestoreEpoch
	m.mu.RUnlock()

	if len(data) == 0 {
		return
	}
	if loader == nil {
		msg := "启动恢复失败：未注入插值器加载端口 (InterpolatorLoader)"
		m.setFiveHoleRestoreErr(msg)
		m.setSevenHoleRestoreErr(msg)
		return
	}
	m.restoreInterpolatorFromConfig(data, loader, fiveEpoch, sevenEpoch)
}

// runLoaderWithTimeout 在 goroutine 中执行 loader 调用，等待 ctx.Done 或结果到达。
//
// 泛型参数 T 适配五孔 coreinterp.Interpolator 与七孔 seveninterp.Interpolator 两种签名，
// 消除此前 runLoaderWithTimeout / runSevenHoleLoaderWithTimeout 的重复实现。
//
// 返回值约定：
//   - 成功：(interp, nil, false)
//   - 加载失败：(zero, err, false)
//   - 超时：(zero, ctx.Err(), true) —— 标记 timedOut=true，调用方可据此决定不写 SetInterpolator
//
// 注意：底层 os.Open / Read 等系统调用无法被 ctx 真正中断，loader goroutine
// 可能在 ctx 超时后仍继续执行直到 syscall 返回；我们通过 channel 缓冲容量为 1
// 让那个 goroutine 写完就退出，避免泄漏。
func runLoaderWithTimeout[T any](ctx context.Context, load func() (T, error)) (T, error, bool) {
	type loadResult struct {
		value T
		err   error
	}
	done := make(chan loadResult, 1)
	go func() {
		value, err := load()
		done <- loadResult{value: value, err: err}
	}()
	select {
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err(), true
	case res := <-done:
		return res.value, res.err, false
	}
}

// restoreInterpolatorFromConfig 从已保存的配置 JSON 中提取插值文件路径并重新加载插值器。
//
// 双变体并行恢复（与原 switch-case 按 probeType 选唯一路径不同）：
//   - 五孔变体（prbFile / calibrationCsvFile / multiPrb）与七孔变体（sevenHolePrb）
//     在配置中可并存，启动时两侧独立恢复，互不阻塞。
//   - 任一变体缺失（字段为空）视为该变体未配置，跳过且不报错，不污染对应错误字段。
//   - 任一变体加载失败仅写入对应类型错误桶，另一侧仍可正常加载。
//   - 优先级：五孔按 新算法 CSV > 多 PRB > 单 PRB（与前端 TraversalPrbStep 一致），
//     七孔按 sevenHolePrb.kind 在 PRB 文件集与校准 CSV 之间选择。
//
// 失败处理：每个分支加载失败通过 setFiveHoleRestoreErr / setSevenHoleRestoreErr
// 记录原因，由 CheckPreconditions 经 InterpolatorRestoreErrFor(probeType) 在
// PRB 检查项中暴露给前端，避免前端基于 configRaw 静默推断为"已加载"而出现状态不一致。
//
// 五孔/七孔独立 ctx：两侧各持 restoreInterpolatorTimeout 超时窗口，避免一侧 I/O 慢
// 挤压另一侧可用时间。原实现共享单一 5s ctx，五孔加载耗 4.9s 时七孔只剩 0.1s 必然超时。
//
// 版本保护：fiveEpoch/sevenEpoch 由调用方在 goroutine 启动时捕获，传给两侧 restore
// 路径，写回前经 setInterpolatorFromRestore / setSevenHoleInterpolatorFromRestore 比对
// 当前 epoch——若用户在恢复期间显式导入/清除了对应变体，跳过写回以保护用户最新状态。
//
// 该函数通过 ports.InterpolatorLoader 接口加载文件，**不直接依赖
// adapters/interpolation 实现包**，遵守工作区六边形分层约束。
func (m *TraversalManager) restoreInterpolatorFromConfig(data []byte, loader ports.InterpolatorLoader, fiveEpoch, sevenEpoch uint64) {
	m.restoreInterpolatorFromConfigWithTimeout(data, loader, fiveEpoch, sevenEpoch, restoreInterpolatorTimeout)
}

func (m *TraversalManager) restoreInterpolatorFromConfigWithTimeout(data []byte, loader ports.InterpolatorLoader, fiveEpoch, sevenEpoch uint64, timeout time.Duration) {
	// 容错解析：prbFile 和 calibrationCsvFile 字段在前端均为对象结构
	var rawCfg traversalRestoreConfig
	if err := json.Unmarshal(data, &rawCfg); err != nil {
		msg := fmt.Sprintf("启动恢复失败：解析配置 JSON 出错: %v", err)
		slog.Error("interpolator restore failed",
			"component", "traversal",
			"error", err,
		)
		// 解析失败意味着无法判断两侧变体字段是否存在，两侧都报错。
		m.setFiveHoleRestoreErr(msg)
		m.setSevenHoleRestoreErr(msg)
		return
	}

	// 注意：此处不再无条件 resetAllRestoreErrs。
	// 旧实现入口处清空两侧错误，会让"上一次失败 + 本次配置变更（移除该变体）"
	// 静默清空陈旧错误。新实现依赖 setInterpolatorFromRestore 在成功路径清空错误，
	// 失败路径覆盖错误；未配置变体保留上一次错误信息更有助于用户排查。

	// 探针类型边界校验（双变体语义）：未知非空类型两侧同时报错；
	// 五孔字段与 sevenHolePrb 并存合法，两侧独立恢复（spec §5.2.1 第 6 条）。
	if rawCfg.ProbeType != "" && rawCfg.ProbeType != traversal.ProbeTypeFiveHole && rawCfg.ProbeType != traversal.ProbeTypeSevenHole {
		msg := fmt.Sprintf("启动恢复失败：未知探针类型 %q", rawCfg.ProbeType)
		m.setFiveHoleRestoreErr(msg)
		m.setSevenHoleRestoreErr(msg)
		return
	}

	// 五孔/七孔独立 ctx：两侧各持独立超时窗口，互不挤压。
	// 不复用父 ctx 的 deadline——父 ctx 由调用方在 RestoreInterpolatorFromPersistedConfig
	// 内创建为 5s 超时，五孔占用后七孔会被挤压。此处各自从 context.Background 派生，
	// 与原"父 ctx + 两侧共享"语义等价但隔离超时。
	// 五孔变体恢复：按字段存在性决定（无字段则跳过，不报错）。
	fiveCtx, fiveCancel := context.WithTimeout(context.Background(), timeout)
	m.restoreFiveHoleFromConfig(fiveCtx, &rawCfg, loader, fiveEpoch)
	fiveCancel()

	// 七孔变体恢复：与五孔独立，互不阻塞。
	sevenCtx, sevenCancel := context.WithTimeout(context.Background(), timeout)
	m.restoreSevenHoleFromConfig(sevenCtx, &rawCfg, loader, sevenEpoch)
	sevenCancel()
}

// traversalRestoreConfig 是 restoreInterpolatorFromConfig 解析持久化 JSON 用的
// 命名类型，供 restoreFiveHoleFromConfig / restoreSevenHoleFromConfig 共享，
// 避免在每个恢复函数内重复 anonymous struct 定义。
type traversalRestoreConfig struct {
	ProbeType              string `json:"probeType"`
	InterpolationAlgorithm string `json:"interpolationAlgorithm"`
	PrbFile                struct {
		FilePath string `json:"filePath"`
	} `json:"prbFile"`
	CalibrationCsvFile struct {
		FilePath string `json:"filePath"`
	} `json:"calibrationCsvFile"`
	UseMultiPrb bool `json:"useMultiPrb"`
	MultiPrb    *struct {
		Files []struct {
			FilePath string `json:"filePath"`
		} `json:"files"`
		MachNumbers       []float64 `json:"machNumbers"`
		InterpolationMode string    `json:"interpolationMode"`
	} `json:"multiPrb"`
	SevenHolePrb *sevenHolePrbConfig `json:"sevenHolePrb"`
}

// restoreFiveHoleFromConfig 按五孔字段存在性恢复五孔插值器。
//
// 优先级：新算法 CSV > 多 PRB > 单 PRB，与前端 TraversalPrbStep 一致。
// 三类字段均缺失时视为五孔变体未配置，跳过且不写入错误。
// 失败时通过 setFiveHoleRestoreErr 记录，不写回插值器。
//
// 通过 runLoaderWithTimeout 让 ctx 超时真正生效：超时后即便底层 I/O
// 仍在阻塞，主 goroutine 也不会调用 setInterpolatorFromRestore。
//
// fiveEpoch 由 goroutine 启动时捕获，传给 setInterpolatorFromRestore 比对当前 epoch；
// 若用户在恢复期间显式导入/清除了五孔插值器（epoch 递增），跳过写回以保护用户最新状态。
func (m *TraversalManager) restoreFiveHoleFromConfig(ctx context.Context, cfg *traversalRestoreConfig, loader ports.InterpolatorLoader, fiveEpoch uint64) {
	// 先按字段存在性选出五孔加载描述符；无匹配字段时视为该变体未配置。
	var spec *fiveHoleLoadSpec
	switch {
	case cfg.InterpolationAlgorithm == "new" && cfg.CalibrationCsvFile.FilePath != "":
		csvPath := cfg.CalibrationCsvFile.FilePath
		spec = &fiveHoleLoadSpec{
			loadType: "csv",
			path:     csvPath,
			load: func() (coreinterp.Interpolator, error) {
				return loader.LoadFiveHoleCSV(csvPath)
			},
		}

	case cfg.UseMultiPrb && cfg.MultiPrb != nil && len(cfg.MultiPrb.Files) > 0:
		filePaths := make([]string, 0, len(cfg.MultiPrb.Files))
		for _, f := range cfg.MultiPrb.Files {
			if f.FilePath != "" {
				filePaths = append(filePaths, f.FilePath)
			}
		}
		if len(filePaths) == 0 {
			m.setFiveHoleRestoreErr("启动恢复失败：多 PRB 配置中没有有效文件路径")
			return
		}
		mode := coreinterp.MultiPrbInterpolationMode(cfg.MultiPrb.InterpolationMode)
		machNumbers := append([]float64(nil), cfg.MultiPrb.MachNumbers...)
		spec = &fiveHoleLoadSpec{
			loadType:  "multi_prb",
			fileCount: len(filePaths),
			// 捕获 filePaths/machNumbers/mode 副本，避免闭包持有 cfg 指针。
			load: func() (coreinterp.Interpolator, error) {
				// Task 07：loader 新签名返回 *MultiPrbLoadMetadata，启动恢复路径
				// 不消费 metadata（仅显式 ImportMultiPRB 路径需要 Files/Warnings），
				// 这里丢弃 metadata 仅取 interpolator + err。
				interp, _, err := loader.LoadMultiPRB(filePaths, machNumbers, mode)
				return interp, err
			},
		}

	case cfg.PrbFile.FilePath != "":
		prbPath := cfg.PrbFile.FilePath
		spec = &fiveHoleLoadSpec{
			loadType: "prb",
			path:     prbPath,
			load: func() (coreinterp.Interpolator, error) {
				return loader.LoadPRB(prbPath)
			},
		}
	}

	if spec != nil {
		m.applyFiveHoleLoad(ctx, *spec, fiveEpoch)
	}
}

// fiveHoleLoadSpec 描述一次五孔加载任务的输入。
//
// 抽取为命名结构体（而非匿名闭包）的动机：让 applyFiveHoleLoad 可单元测试断言，
// 同时消除三处 switch-case 的重复超时/错误/slog/SetInterpolator 模板。
type fiveHoleLoadSpec struct {
	loadType  string // "csv" | "multi_prb" | "prb"，写入 slog.type 字段
	path      string // CSV/单 PRB 路径；multi_prb 模式留空
	fileCount int    // multi_prb 模式的文件数；其他模式为 0
	load      func() (coreinterp.Interpolator, error)
}

// applyFiveHoleLoad 是 restoreFiveHoleFromConfig 三分支共享的加载/写回路径。
//
// 流程：runLoaderWithTimeout → 超时/错误/空插值器短路 → setInterpolatorFromRestore 比对 epoch 写回。
// 成功写入时由 setInterpolatorFromRestore 清空 lastFiveHoleRestoreErr；
// 失败路径由本函数经 setFiveHoleRestoreErr 写入根因，便于 CheckPreconditions 暴露给前端。
//
// nil 插值器防御：loader 接口约定返回 (nil, nil) 视为加载成功但无数据，
// 但调用方 SetInterpolator(nil) 会让 hasLoadedInterpolator 返回 false 却无任何错误信息；
// 此处显式拦截并写入根因，避免"恢复成功但插值器为空"的静默状态。
func (m *TraversalManager) applyFiveHoleLoad(ctx context.Context, spec fiveHoleLoadSpec, fiveEpoch uint64) {
	interpolator, err, timedOut := runLoaderWithTimeout(ctx, spec.load)
	if timedOut {
		msg := fmt.Sprintf("启动恢复超时：加载%s文件超过 %s（path=%s, file_count=%d）",
			spec.loadType, restoreInterpolatorTimeout, spec.path, spec.fileCount)
		slog.Warn("interpolator restore timed out",
			"component", "traversal",
			"type", spec.loadType,
			"path", spec.path,
			"file_count", spec.fileCount,
			"timeout", restoreInterpolatorTimeout,
		)
		m.setFiveHoleRestoreErr(msg)
		return
	}
	if err != nil {
		slog.Error("interpolator restore failed",
			"component", "traversal",
			"type", spec.loadType,
			"path", spec.path,
			"file_count", spec.fileCount,
			"error", err,
		)
		m.setFiveHoleRestoreErr(fmt.Sprintf("启动恢复失败：加载%s文件出错: %v", spec.loadType, err))
		return
	}
	// loader 返回 (nil, nil) 视为加载成功但插值器为空——这种状态会让 hasLoadedInterpolator
	// 返回 false 却无 restoreErr，前端看到默认消息无法定位根因；此处显式报错。
	if interpolator == nil {
		msg := fmt.Sprintf("启动恢复失败：加载%s返回空插值器（path=%s, file_count=%d）",
			spec.loadType, spec.path, spec.fileCount)
		slog.Error("interpolator restore returned nil",
			"component", "traversal",
			"type", spec.loadType,
			"path", spec.path,
			"file_count", spec.fileCount,
		)
		m.setFiveHoleRestoreErr(msg)
		return
	}
	// epoch 不匹配返回 false：用户在恢复期间显式导入/清除了五孔插值器，
	// 当前结果已陈旧，跳过写回以保护用户最新状态。
	if !m.setInterpolatorFromRestore(interpolator, fiveEpoch) {
		slog.Info("interpolator restore skipped (user changed interpolator)",
			"component", "traversal",
			"type", spec.loadType,
			"path", spec.path,
		)
		return
	}
	slog.Info("interpolator restored",
		"component", "traversal",
		"type", spec.loadType,
		"path", spec.path,
		"file_count", spec.fileCount,
	)
}

// restoreSevenHoleFromConfig 按七孔字段存在性恢复七孔插值器。
//
// sevenHolePrb == nil 或文件集不完整时视为七孔变体未配置，跳过且不报错。
// 加载失败通过 setSevenHoleRestoreErr 记录，不写回插值器。
//
// sevenEpoch 由 goroutine 启动时捕获，传给 setSevenHoleInterpolatorFromRestore 比对当前 epoch；
// 若用户在恢复期间显式导入/清除了七孔插值器（epoch 递增），跳过写回以保护用户最新状态。
func (m *TraversalManager) restoreSevenHoleFromConfig(ctx context.Context, cfg *traversalRestoreConfig, loader ports.InterpolatorLoader, sevenEpoch uint64) {
	seven := cfg.SevenHolePrb
	// 七孔字段缺失或文件集不完整：视为该变体未配置，跳过（不写错误）。
	// 这与五孔字段缺失的处理保持对称——双变体语义下任一侧未配置都视为合法。
	if seven == nil || seven.InnerFile == nil || seven.InnerFile.FilePath == "" || len(seven.OuterFiles) != 6 {
		return
	}

	var outerPaths [6]string
	for i, f := range seven.OuterFiles {
		outerPaths[i] = f.FilePath
	}
	innerPath := seven.InnerFile.FilePath
	// 按 kind 选择数据源：校准 CSV（七孔校准导出）或 .prb 文件集（默认）。
	// kindLabel 在 slog 与错误消息中复用，避免散落的 map[bool]string 模式。
	isCalibrationCsv := seven.Kind == KindSevenHoleCalibrationCsv
	kindLabel := "PRB"
	if isCalibrationCsv {
		kindLabel = "校准CSV"
	}
	interpolator, err, timedOut := runLoaderWithTimeout(ctx, func() (seveninterp.Interpolator, error) {
		// Task 07：loader 新签名返回 *SevenHoleLoadMetadata，启动恢复路径
		// 不消费 metadata（仅显式 Import 路径需要 LoadedAtMs/ValidRange），
		// 这里丢弃 metadata 仅取 interpolator + err。
		if isCalibrationCsv {
			interp, _, err := loader.LoadSevenHoleCalibrationCSV(innerPath, outerPaths)
			return interp, err
		}
		interp, _, err := loader.LoadSevenHolePRB(innerPath, outerPaths)
		return interp, err
	})
	if timedOut {
		msg := fmt.Sprintf("启动恢复超时：加载七孔%s文件集超过 %s", kindLabel, restoreInterpolatorTimeout)
		slog.Warn("interpolator restore timed out",
			"component", "traversal",
			"type", "seven_hole_prb",
			"kind", seven.Kind,
			"inner_path", innerPath,
			"timeout", restoreInterpolatorTimeout,
		)
		m.setSevenHoleRestoreErr(msg)
		return
	}
	if err != nil {
		slog.Error("interpolator restore failed",
			"component", "traversal",
			"type", "seven_hole_prb",
			"kind", seven.Kind,
			"inner_path", innerPath,
			"error", err,
		)
		m.setSevenHoleRestoreErr(fmt.Sprintf("启动恢复失败：加载七孔%s文件集出错: %v", kindLabel, err))
		return
	}
	// loader 返回 (nil, nil) 防御：与五孔路径同语义。
	if interpolator == nil {
		msg := fmt.Sprintf("启动恢复失败：加载七孔%s文件集返回空插值器（inner_path=%s）", kindLabel, innerPath)
		slog.Error("interpolator restore returned nil",
			"component", "traversal",
			"type", "seven_hole_prb",
			"kind", seven.Kind,
			"inner_path", innerPath,
		)
		m.setSevenHoleRestoreErr(msg)
		return
	}
	// epoch 不匹配返回 false：用户在恢复期间显式导入/清除了七孔插值器，
	// 当前结果已陈旧，跳过写回以保护用户最新状态。
	if !m.setSevenHoleInterpolatorFromRestore(interpolator, sevenEpoch) {
		slog.Info("interpolator restore skipped (user changed interpolator)",
			"component", "traversal",
			"type", "seven_hole_prb",
			"kind", seven.Kind,
			"inner_path", innerPath,
		)
		return
	}
	slog.Info("interpolator restored",
		"component", "traversal",
		"type", "seven_hole_prb",
		"kind", seven.Kind,
		"inner_path", innerPath,
	)
}

// setFiveHoleRestoreErr 线程安全地写入五孔侧启动恢复错误消息，
// 空字符串表示无错误。该字段仅由 CheckPreconditions 经
// InterpolatorRestoreErrFor("five-hole") 读取。
func (m *TraversalManager) setFiveHoleRestoreErr(msg string) {
	m.mu.Lock()
	m.lastFiveHoleRestoreErr = msg
	m.mu.Unlock()
}

// setSevenHoleRestoreErr 与 setFiveHoleRestoreErr 同语义，按七孔类型分桶。
func (m *TraversalManager) setSevenHoleRestoreErr(msg string) {
	m.mu.Lock()
	m.lastSevenHoleRestoreErr = msg
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
	Kind       string                `json:"kind,omitempty"`
	InnerFile  *sevenHolePrbFileRef  `json:"innerFile"`
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
	// 布点路径是运动轴参与性的最终真相源：line/rectangle/sector 会把未使用坐标
	// 标为 NaN。校验原始配置后统一剔除对应绑定，使旧配置中的轴覆盖仍可读取，
	// 同时保证移动、状态校验、停机和断点恢复消费同一轴集。
	motionAxes = motionAxesForPath(motionAxes, points)
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
		// I-2 修复：把 cfg.Validation/cfg.Stabilization 写入 config 字段，
		// 让 ParseConfig 成为纯解析函数（无 manager 状态副作用）。
		// 装配路径（ParseAndStartTraversal）读取此字段调用 m.SetValidation/SetStabilization。
		Validation:    cfg.Validation,
		Stabilization: cfg.Stabilization,
	}
	// 压力类型兜底：空串与缺失均落 "gauge"，与历史行为一致，避免归一化逻辑误判绝压。
	pressureType := cfg.PProbePressureType
	if pressureType != "gauge" && pressureType != "absolute" {
		pressureType = "gauge"
	}
	config.PProbePressureType = pressureType
	// I-2 修复：ParseConfig 不再产生状态副作用（原先在此处调用 SetValidation/SetStabilization）。
	// 解析方法承担装配职责会污染 manager 状态——prepareStart (registry_admission.go)
	// 仅需解析配置用于校验，不应改变 m.validation/m.stabilization；CheckPreconditions
	// 调用 ParseConfig 后再读 m.validation 会读到上次解析注入的旧值。
	// 改由 ParseAndStartTraversal 等装配路径在 ParseConfig 之后显式调用 Set*，
	// 让 ParseConfig 成为纯解析函数。
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
	// I-2 修复：装配路径显式注入数据验证与稳定等待配置到 manager 持久字段。
	// 原先由 ParseConfig 隐式调用，会污染仅为校验而解析的调用方（如 prepareStart）。
	// 现在装配语义清晰：ParseConfig 是纯解析，ParseAndStartTraversal 是装配+启动。
	m.SetValidation(config.Validation)
	m.SetStabilization(config.Stabilization)

	// 后端再次校验采集态，防止绕过前端禁用按钮或确认对话框直接调用启动 API。
	// 遍历测试不再隐式发送 StartAcquisition，设备采集生命周期由操作员管理。
	// 多设备通道绑定（如五孔在设备 A、大气压/温度在设备 B）时逐台校验，
	// 任一设备未采集都拒绝启动，避免运行到首个点才暴露采样超时。
	m.mu.RLock()
	acqController := m.acquisitionController
	m.mu.RUnlock()
	if acqController != nil {
		if dev, abnormal := firstAbnormalAcquisitionDevice(acqController, config); abnormal {
			if dev.state == ports.AcquisitionReconnectRequired {
				return "", fmt.Errorf("device %s is not connected; reconnect it and start acquisition before traversal", dev.name)
			}
			return "", fmt.Errorf("device %s is not acquiring; start acquisition before traversal", dev.name)
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
