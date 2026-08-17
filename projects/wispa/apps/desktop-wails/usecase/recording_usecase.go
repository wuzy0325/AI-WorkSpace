package usecase

import (
	"fmt"
	"sync"

	"wispa/core"
	"wispa/ports"
)

// RecordingUsecase 录制业务逻辑
//
// 持有固定 recorder 实例（仅 CSV 录制器，Binary 格式已移除）。
// 多设备 1kHz 场景下 CSV 录制器已具备足够吞吐（异步队列 + bufio 缓冲）。
type RecordingUsecase struct {
	mu       sync.Mutex
	recorder ports.RecordingPort
}

// NewRecordingUsecase 创建录制 usecase
// recorder 为 CSV 录制器实例；若传入 nil 则 Start 时返回错误
func NewRecordingUsecase(recorder ports.RecordingPort) *RecordingUsecase {
	return &RecordingUsecase{recorder: recorder}
}

// Start 开始录制
func (uc *RecordingUsecase) Start(config core.RecordingConfig) error {
	uc.mu.Lock()
	defer uc.mu.Unlock()

	if uc.recorder == nil {
		return fmt.Errorf("recorder not configured")
	}
	return uc.recorder.Start(config)
}

// Write 异步投递数据快照（非阻塞，队列满时丢弃并计数）
func (uc *RecordingUsecase) Write(snapshot core.PressureSnapshot) error {
	uc.mu.Lock()
	recorder := uc.recorder
	uc.mu.Unlock()
	if recorder == nil {
		return fmt.Errorf("recorder not started")
	}
	return recorder.Write(snapshot)
}

// Stop 停止录制
func (uc *RecordingUsecase) Stop() error {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	if uc.recorder == nil {
		return nil
	}
	return uc.recorder.Stop()
}

// IsActive 无锁热路径判活，供 relayStream 每帧调用避免 Status() 锁争用。
// recorder 未创建时返回 false。
func (uc *RecordingUsecase) IsActive() bool {
	uc.mu.Lock()
	recorder := uc.recorder
	uc.mu.Unlock()
	if recorder == nil {
		return false
	}
	return recorder.IsActive()
}

// Status 获取录制状态
func (uc *RecordingUsecase) Status() core.RecordingSession {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	if uc.recorder == nil {
		return core.RecordingSession{Status: core.RecordingIdle}
	}
	return uc.recorder.Status()
}

// StopWithError 转发到 recorder，用于设备断连自动停止场景。
// 返回值与 recorder.StopWithError 一致：CAS 失败（用户已主动停止）返回 false。
func (uc *RecordingUsecase) StopWithError(msg string) bool {
	uc.mu.Lock()
	recorder := uc.recorder
	uc.mu.Unlock()
	if recorder == nil {
		return false
	}
	return recorder.StopWithError(msg)
}
