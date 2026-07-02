// Package usecase 提供数据保存领域的用例编排。
//
// StorageRecorder 是 usecase 层对 ports.RecordingSink 的封装：
//   - 提供 Start/Stop/Status 简单 API 给 API/Wails 层调用
//   - 监听 sink.Done() 信号，在 sink 自停止时同步自身 recording 状态
//   - Status 代理 sink.Status()，并在 sink 不可用时返回安全默认值
//   - 暴露 Errors() channel 用于上层订阅录制期间的错误事件
package usecase

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/storage"
	"wind-daq/services/api-go/internal/ports"
)

// StorageRecorder 数据保存用例，封装 RecordingSink 并向上提供简化 API。
type StorageRecorder struct {
	mu sync.RWMutex
	// sink 与 doneWatcher 在 Start 时设置，Stop 后 sink 保留（可复用），
	// doneWatcher 在 sink 自停止后停止并置 nil，下次 Start 重新创建。
	sink         ports.RecordingSink
	doneWatcher  *sinkDoneWatcher
	recording    bool
	lastConfig   storage.RecordingConfig
	// errCh 用于向上层推送录制期间的错误事件。
	// 容量 16 避免高频错误时阻塞 writer goroutine；满了就丢弃（错误本身已通过 Status 暴露）。
	errCh chan error
}

// sinkDoneWatcher 监听 sink.Done() 信号并触发 StorageRecorder 同步状态。
// 一个 watcher 对应一次 Start 会话；Stop 时通过 stopCh 终止 watcher goroutine。
type sinkDoneWatcher struct {
	recorder *StorageRecorder
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// NewStorageRecorder 创建 StorageRecorder。
// sink 可为 nil（构造时尚未装配），但 Start 前必须通过 sink 字段注入。
func NewStorageRecorder(sink ports.RecordingSink) *StorageRecorder {
	return &StorageRecorder{
		sink:  sink,
		errCh: make(chan error, 16),
	}
}

// Start 启动录制会话。
// 校验 config 必填字段后委托给 sink.Start，并启动 doneWatcher 监听自停止信号。
// 重复 Start 会返回错误（由 sink 的 CAS 保护）。
func (r *StorageRecorder) Start(config storage.RecordingConfig) error {
	slog.Info("StorageRecorder Start 开始",
		"component", "StorageRecorder",
		"outputDir", config.OutputDir,
		"filePrefix", config.FilePrefix,
		"format", config.Format,
		"rotationEnabled", config.FileRotation.Enabled,
		"stopConditions", config.StopConditions,
	)

	if strings.TrimSpace(config.OutputDir) == "" {
		err := fmt.Errorf("outputDir is required")
		slog.Error("StorageRecorder Start 失败", "component", "StorageRecorder", "error", err)
		return err
	}
	if strings.TrimSpace(config.FilePrefix) == "" {
		err := fmt.Errorf("filePrefix is required")
		slog.Error("StorageRecorder Start 失败", "component", "StorageRecorder", "error", err)
		return err
	}

	r.mu.Lock()
	sink := r.sink
	if sink == nil {
		r.mu.Unlock()
		err := fmt.Errorf("recording sink is required")
		slog.Error("StorageRecorder Start 失败", "component", "StorageRecorder", "error", err)
		return err
	}
	// 启动前先确保上一个 watcher 已终止（理论上 Stop 时已终止，这里防御性检查）
	if r.doneWatcher != nil {
		r.mu.Unlock()
		err := fmt.Errorf("recording already in progress")
		slog.Error("StorageRecorder Start 失败", "component", "StorageRecorder", "error", err)
		return err
	}
	r.mu.Unlock()

	if err := sink.Start(config); err != nil {
		slog.Error("StorageRecorder Start 失败", "component", "StorageRecorder", "error", err)
		return err
	}

	r.mu.Lock()
	r.lastConfig = config
	r.recording = true
	// 启动 doneWatcher：监听 sink.Done() 并在自停止时同步 recording 状态
	watcher := &sinkDoneWatcher{
		recorder: r,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
	r.doneWatcher = watcher
	r.mu.Unlock()

	go watcher.watch(sink)
	slog.Info("StorageRecorder Start 成功", "component", "StorageRecorder", "outputDir", config.OutputDir)
	return nil
}

// watch 监听 sink.Done() 或 watcher.stopCh，先到者触发相应动作。
// sink.Done() 触发：标记 recording=false，向 errCh 推送错误（如有）
// stopCh 触发：正常退出，不做状态变更（Stop 调用方负责更新 recording）
func (w *sinkDoneWatcher) watch(sink ports.RecordingSink) {
	defer close(w.doneCh)
	select {
	case <-sink.Done():
		// sink 自停止（条件达到或 I/O 错误）：同步状态
		w.recorder.mu.Lock()
		w.recorder.recording = false
		w.recorder.doneWatcher = nil
		status := sink.Status()
		w.recorder.mu.Unlock()
		slog.Warn("StorageRecorder 检测到 sink 自停止",
			"component", "StorageRecorder",
			"lastError", status.LastError,
			"fileCount", status.FileCount,
			"recordCount", status.RecordCount,
		)
		if status.LastError != "" {
			// 非阻塞推送错误事件
			select {
			case w.recorder.errCh <- fmt.Errorf("%s", status.LastError):
			default:
			}
		}
	case <-w.stopCh:
		// 正常 Stop：调用方负责更新 recording 状态
	}
}

// Stop 停止录制会话。
// 通知 sink.Stop drain 剩余数据，并终止 doneWatcher goroutine。
// 未在录制时调用为幂等 no-op。
func (r *StorageRecorder) Stop() error {
	r.mu.Lock()
	sink := r.sink
	watcher := r.doneWatcher
	wasRecording := r.recording
	r.mu.Unlock()

	if !wasRecording || sink == nil {
		slog.Warn("StorageRecorder Stop 跳过：未在录制", "component", "StorageRecorder")
		return nil
	}

	slog.Info("StorageRecorder Stop 开始", "component", "StorageRecorder", "outputDir", r.lastConfig.OutputDir)
	// 先终止 doneWatcher，避免与 sink.Stop 竞争状态更新
	if watcher != nil {
		close(watcher.stopCh)
		<-watcher.doneCh
	}

	err := sink.Stop()
	if err != nil {
		slog.Error("StorageRecorder Stop 失败", "component", "StorageRecorder", "error", err)
	} else {
		slog.Info("StorageRecorder Stop 成功", "component", "StorageRecorder", "outputDir", r.lastConfig.OutputDir)
	}

	r.mu.Lock()
	r.recording = false
	r.doneWatcher = nil
	r.mu.Unlock()
	return err
}

// Status 返回当前录制状态快照。
// 若 sink 不可用返回安全默认值（recording=false）。
// recording 字段由 StorageRecorder 自身维护，其他字段代理 sink.Status()。
func (r *StorageRecorder) Status() storage.RecordingStatus {
	r.mu.RLock()
	recording := r.recording
	sink := r.sink
	cfg := r.lastConfig
	r.mu.RUnlock()

	if sink == nil {
		return storage.RecordingStatus{Recording: false}
	}
	status := sink.Status()
	// 以 StorageRecorder 的 recording 状态为准（sink 自停止后 started 已 false，二者一致）
	status.Recording = recording
	// 若 sink 未启动（首次 Status 调用），OutputDir 兜底为 lastConfig
	if status.OutputDir == "" && cfg.OutputDir != "" {
		status.OutputDir = cfg.OutputDir
	}
	slog.Debug("StorageRecorder Status 查询",
		"component", "StorageRecorder",
		"recording", status.Recording,
		"outputDir", status.OutputDir,
		"fileSize", status.FileSize,
		"recordCount", status.RecordCount,
	)
	return status
}

// Errors 返回只读 channel，用于上层订阅录制期间的错误事件。
// 容量 16，满了就丢弃（错误本身已通过 Status().LastError 暴露）。
func (r *StorageRecorder) Errors() <-chan error {
	return r.errCh
}

// HandlePayload 接收设备数据并投递给 sink。
// 未在录制时直接返回 nil（不阻塞数据流）。
func (r *StorageRecorder) HandlePayload(payload device.DataPayload) error {
	r.mu.RLock()
	recording := r.recording
	sink := r.sink
	r.mu.RUnlock()
	if !recording || sink == nil {
		return nil
	}
	if err := sink.Write(payload); err != nil {
		slog.Error("StorageRecorder HandlePayload 写入失败",
			"component", "StorageRecorder",
			"deviceID", payload.DeviceID,
			"error", err,
		)
		return err
	}
	return nil
}
