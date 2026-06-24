// Package usecase — traversal 配置层（从 traversal.go 拆分）
//
// 包含：traversalAPIConfig（前端 JSON 形态）、ParseAndStartTraversal（API 入口）、
// 持久化配置加载、isSubState 等子状态判定、角色 → 通道标签映射。
package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
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
	}
	if err := json.Unmarshal(data, &rawCfg); err != nil {
		msg := fmt.Sprintf("启动恢复失败：解析配置 JSON 出错: %v", err)
		log.Printf("[traversal] %s", msg)
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

	// 根据插值算法优先级加载（通过 runLoaderWithTimeout 让 ctx 超时真正生效：
	// 超时后即便底层 I/O 仍在阻塞，主 goroutine 也不会调用 SetInterpolator）
	switch {
	case rawCfg.InterpolationAlgorithm == "new" && rawCfg.CalibrationCsvFile.FilePath != "":
		// 新算法：从 CSV 标定文件加载
		csvPath := rawCfg.CalibrationCsvFile.FilePath
		interpolator, err, timedOut := runLoaderWithTimeout(ctx, func() (coreinterp.Interpolator, error) {
			return loader.LoadFiveHoleCSV(csvPath)
		})
		if timedOut {
			msg := fmt.Sprintf("启动恢复超时：加载 CSV 文件 %s 超过 %s", csvPath, restoreInterpolatorTimeout)
			log.Printf("[traversal] %s", msg)
			m.setInterpolatorRestoreErr(msg)
			return
		}
		if err != nil {
			msg := fmt.Sprintf("启动恢复失败：加载 CSV 文件 %s 出错: %v", csvPath, err)
			log.Printf("[traversal] %s", msg)
			m.setInterpolatorRestoreErr(msg)
			return
		}
		m.SetInterpolator(interpolator)
		log.Printf("[traversal] 已从 CSV 文件恢复插值器: %s", csvPath)

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
		machNumbers := rawCfg.MultiPrb.MachNumbers
		interpolator, err, timedOut := runLoaderWithTimeout(ctx, func() (coreinterp.Interpolator, error) {
			return loader.LoadMultiPRB(filePaths, machNumbers, mode)
		})
		if timedOut {
			msg := fmt.Sprintf("启动恢复超时：加载 %d 个多 PRB 文件超过 %s", len(filePaths), restoreInterpolatorTimeout)
			log.Printf("[traversal] %s", msg)
			m.setInterpolatorRestoreErr(msg)
			return
		}
		if err != nil {
			msg := fmt.Sprintf("启动恢复失败：加载多 PRB 文件出错: %v", err)
			log.Printf("[traversal] %s", msg)
			m.setInterpolatorRestoreErr(msg)
			return
		}
		m.SetInterpolator(interpolator)
		log.Printf("[traversal] 已从 %d 个 PRB 文件恢复插值器", len(filePaths))

	case rawCfg.PrbFile.FilePath != "":
		// 单 PRB 模式
		prbPath := rawCfg.PrbFile.FilePath
		interpolator, err, timedOut := runLoaderWithTimeout(ctx, func() (coreinterp.Interpolator, error) {
			return loader.LoadPRB(prbPath)
		})
		if timedOut {
			msg := fmt.Sprintf("启动恢复超时：加载 PRB 文件 %s 超过 %s", prbPath, restoreInterpolatorTimeout)
			log.Printf("[traversal] %s", msg)
			m.setInterpolatorRestoreErr(msg)
			return
		}
		if err != nil {
			msg := fmt.Sprintf("启动恢复失败：加载 PRB 文件 %s 出错: %v", prbPath, err)
			log.Printf("[traversal] %s", msg)
			m.setInterpolatorRestoreErr(msg)
			return
		}
		m.SetInterpolator(interpolator)
		log.Printf("[traversal] 已从 PRB 文件恢复插值器: %s", prbPath)
	}
}

// setInterpolatorRestoreErr 线程安全地写入启动恢复错误消息，
// 空字符串表示无错误。该字段仅由 CheckPreconditions 读取。
func (m *TraversalManager) setInterpolatorRestoreErr(msg string) {
	m.mu.Lock()
	m.lastInterpolatorRestoreErr = msg
	m.mu.Unlock()
}

func isSubState(s traversal.State) bool {
	return s == traversal.StateMoving || s == traversal.StateStabilizing ||
		s == traversal.StateAcquiring || s == traversal.StateSaving || s == traversal.StatePreparing
}

type traversalAPIConfig struct {
	Name   string `json:"name"`
	Layout struct {
		Pattern    string `json:"pattern"`
		SnakeOrder bool   `json:"snakeOrder"`
		Line       *struct {
			StartX        float64                 `json:"startX"`
			StartY        float64                 `json:"startY"`
			EndX          float64                 `json:"endX"`
			EndY          float64                 `json:"endY"`
			XStepSegments []traversal.StepSegment `json:"xStepSegments"`
			YStepSegments []traversal.StepSegment `json:"yStepSegments"`
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
			Points []struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
			} `json:"points"`
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
	} `json:"channels"`
	DwellTimeMs       int                             `json:"dwellTimeMs"`
	SamplesPerPoint   int                             `json:"samplesPerPoint"`
	SavePath          string                          `json:"savePath"`
	SaveFileName      string                          `json:"saveFileName"`
	SaveOptions       *traversal.SaveOptions          `json:"saveOptions,omitempty"`
	Validation        *traversal.DataValidationConfig `json:"validation,omitempty"`
	Stabilization     *traversal.StabilizationConfig  `json:"stabilization,omitempty"`
	InterpolationMode string                          `json:"interpolationMode,omitempty"`
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
	}
	// 回退使用 name 字段作为标签
	return name
}

func (m *TraversalManager) ParseAndStartTraversal(raw json.RawMessage) (string, error) {
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
		if err := m.Start(config); err != nil {
			return "", err
		}
		m.SaveConfigRaw(raw)
		return config.TaskID, nil
	}

	var cfg traversalAPIConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", fmt.Errorf("invalid request: %w", err)
	}

	points := traversal.PointsFromLayout(traversal.LayoutConfig{
		Pattern:    cfg.Layout.Pattern,
		SnakeOrder: cfg.Layout.SnakeOrder,
		Line: func() *traversal.LineLayout {
			if cfg.Layout.Line == nil {
				return nil
			}
			return &traversal.LineLayout{
				StartX: cfg.Layout.Line.StartX, StartY: cfg.Layout.Line.StartY,
				EndX: cfg.Layout.Line.EndX, EndY: cfg.Layout.Line.EndY,
				XStepSegments: cfg.Layout.Line.XStepSegments,
				YStepSegments: cfg.Layout.Line.YStepSegments,
			}
		}(),
		Rectangle: func() *traversal.RectangleLayout {
			if cfg.Layout.Rectangle == nil {
				return nil
			}
			return &traversal.RectangleLayout{
				XMin: cfg.Layout.Rectangle.XMin, XMax: cfg.Layout.Rectangle.XMax,
				XStepSegments: cfg.Layout.Rectangle.XStepSegments,
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
			cl := &traversal.CustomLayout{}
			for _, p := range cfg.Layout.Custom.Points {
				cl.Points = append(cl.Points, struct {
					X float64 `json:"x"`
					Y float64 `json:"y"`
				}{X: p.X, Y: p.Y})
			}
			return cl
		}(),
	})

	channels := make([]int, 0, len(cfg.Channels.ProbeChannels))
	channelLabels := make(map[int]string)
	deviceID := ""
	for _, probe := range cfg.Channels.ProbeChannels {
		if !probe.Enabled || probe.Channel.ChannelIndex < 0 {
			continue
		}
		if deviceID == "" {
			deviceID = probe.Channel.DeviceID
		}
		channels = append(channels, probe.Channel.ChannelIndex)
		// 通过 role/name 显式建立 channelIndex→label 映射，避免依赖通道索引顺序
		if label := roleToLabel(probe.Role, probe.Name); label != "" {
			channelLabels[probe.Channel.ChannelIndex] = label
		}
	}
	if deviceID == "" {
		return "", fmt.Errorf("deviceId is required")
	}
	if len(channels) == 0 {
		return "", fmt.Errorf("channels are required")
	}
	if len(points) == 0 {
		return "", fmt.Errorf("path is required")
	}

	dwell := time.Duration(cfg.DwellTimeMs) * time.Millisecond
	if dwell < 0 {
		dwell = 0
	}
	samplesPerPoint := cfg.SamplesPerPoint
	if samplesPerPoint <= 0 {
		samplesPerPoint = 1
	}
	config := traversal.Config{
		TaskID:            fmt.Sprintf("trav-%d", time.Now().UnixMilli()),
		DeviceID:          deviceID,
		Channels:          channels,
		Path:              points,
		DwellTimeMs:       cfg.DwellTimeMs,
		SamplesPerPoint:   samplesPerPoint,
		SavePath:          cfg.SavePath,
		SaveFileName:      cfg.SaveFileName,
		SaveOptions:       cfg.SaveOptions,
		ChannelLabels:     channelLabels,
		InterpolationMode: cfg.InterpolationMode,
	}
	// 注入数据验证与稳定等待配置（前端可选传入）
	m.SetValidation(cfg.Validation)
	m.SetStabilization(cfg.Stabilization)

	if err := m.Start(config); err != nil {
		return "", err
	}
	m.SaveConfigRaw(raw)
	if dwell > 0 {
		go m.RunTraversalLoop(dwell)
	}
	return config.TaskID, nil
}

// 任何退出路径都会调用 sink.FinalizeTraversal 关闭文件，保证落盘
