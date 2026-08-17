package usecase

import (
	"sync"
	"sync/atomic"

	"windlabx4/services/api-go/internal/core/device"
)

// CalibrationStream distributes unthrottled raw frames before offsets are applied.
//
// P2-13：dropped 计数器统计因订阅者缓冲区满而被丢弃的帧数。
// 正常工况下应为 0；持续增长说明订阅者消费速率跟不上生产者
// （如 CalibrationSampler 在 GC 压力下短暂卡顿），可作为性能退化信号
// 通过 /api/calibration/status 或日志暴露给运维。
type CalibrationStream struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan device.DataPayload]struct{}
	dropped     atomic.Uint64
}

func NewCalibrationStream() *CalibrationStream {
	return &CalibrationStream{subscribers: make(map[string]map[chan device.DataPayload]struct{})}
}

func (s *CalibrationStream) Publish(payload device.DataPayload) {
	s.mu.RLock()
	subscribers := make([]chan device.DataPayload, 0, len(s.subscribers[payload.DeviceID]))
	for ch := range s.subscribers[payload.DeviceID] {
		subscribers = append(subscribers, ch)
	}
	s.mu.RUnlock()
	if len(subscribers) == 0 {
		return
	}
	payload.Channels = append([]float64(nil), payload.Channels...)
	payload.ChannelIndices = append([]int(nil), payload.ChannelIndices...)
	for _, ch := range subscribers {
		select {
		case ch <- payload:
		default:
			// 缓冲区满：订阅者消费落后，丢弃此帧避免阻塞生产者（设备 read loop）。
			// 累计计数供运维观测，持续增长需排查消费者为何卡顿。
			s.dropped.Add(1)
		}
	}
}

// Subscribe 注册一个订阅者，返回只读 channel 和取消订阅函数。
// buffer 为 channel 缓冲大小，小于 1 时强制为 1。
func (s *CalibrationStream) Subscribe(deviceID string, buffer int) (<-chan device.DataPayload, func()) {
	if buffer < 1 {
		buffer = 1
	}
	ch := make(chan device.DataPayload, buffer)
	s.mu.Lock()
	if s.subscribers[deviceID] == nil {
		s.subscribers[deviceID] = make(map[chan device.DataPayload]struct{})
	}
	s.subscribers[deviceID][ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.subscribers[deviceID], ch)
		if len(s.subscribers[deviceID]) == 0 {
			delete(s.subscribers, deviceID)
		}
		s.mu.Unlock()
	}
}

// Dropped 返回累计丢弃的帧数（因订阅者缓冲区满）。
// 用于运维监控：持续增长说明消费者速率落后于生产者。
func (s *CalibrationStream) Dropped() uint64 {
	return s.dropped.Load()
}
