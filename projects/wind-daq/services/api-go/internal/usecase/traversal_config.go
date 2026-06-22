// Package usecase — traversal 配置层（从 traversal.go 拆分）
//
// 包含：traversalAPIConfig（前端 JSON 形态）、ParseAndStartTraversal（API 入口）、
// 持久化配置加载、isSubState 等子状态判定、角色 → 通道标签映射。
package usecase

import (
	"encoding/json"
	"fmt"
	"time"

	"wind-daq/services/api-go/internal/core/traversal"
)

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

	// 尝试从已保存的配置中提取 savePath，并探测断点文件是否存在
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
