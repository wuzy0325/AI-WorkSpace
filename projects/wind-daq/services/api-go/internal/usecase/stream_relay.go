package usecase

import (
	"context"
	"log/slog"
	"sync"

	"wind-daq/services/api-go/internal/core/device"
)

// relaySubscription 单个设备订阅的生命周期记录。
// cancel 通知 worker 退出；done 在 worker 完成 hub 侧 unsub() 之后关闭，
// 供 Unsubscribe/Stop 在 relay mutex 外做确定性等待。
type relaySubscription struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type DataStreamRelay struct {
	mu       sync.Mutex
	hub      *AcquisitionHub
	subs     map[string]*relaySubscription
	// retiring 记录已 cancel 但 worker 仍在执行 hub 侧 unsub() 的订阅。
	// 必须存在：Unsubscribe 在锁外等 sub.done 期间会释放 mu，此时并发 Stop 拿到锁
	// 只能看到 r.subs（已 delete），若不维护 retiring 集合，Stop 会直接 close(stopDone) 返回，
	// 违反"Stop 返回前所有 subscription 已注销"契约（worker 可能仍在 hub 侧未完成 unsub）。
	// Stop 因此把 retiring 也纳入等待集合，确保终止语义完整。
	retiring map[*relaySubscription]struct{}
	payloads chan device.DataPayload
	// stopped 为终止性标志：Stop 调用后 Subscribe 一律拒绝（记录 warn 日志）。
	// stopDone 在首个 Stop 完成所有 worker 等待后关闭；后续/并发的 Stop 调用
	// 通过它等待同一次终止完成，保证 Stop 幂等且返回即终止完毕。
	stopped  bool
	stopDone chan struct{}
}

func NewDataStreamRelay(hub *AcquisitionHub) *DataStreamRelay {
	return &DataStreamRelay{
		hub:      hub,
		subs:     make(map[string]*relaySubscription),
		retiring: make(map[*relaySubscription]struct{}),
		payloads: make(chan device.DataPayload, 64),
		stopDone: make(chan struct{}),
	}
}

func (r *DataStreamRelay) Subscribe(deviceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.stopped {
		slog.Warn("DataStreamRelay Subscribe 拒绝：relay 已停止", "component", "DataStreamRelay", "deviceID", deviceID)
		return
	}
	if _, exists := r.subs[deviceID]; exists {
		slog.Warn("DataStreamRelay Subscribe 跳过：设备已订阅", "component", "DataStreamRelay", "deviceID", deviceID)
		return
	}

	slog.Info("DataStreamRelay Subscribe", "component", "DataStreamRelay", "deviceID", deviceID)
	ch, unsub := r.hub.Subscribe(deviceID, 16)
	ctx, cancel := context.WithCancel(context.Background())
	sub := &relaySubscription{cancel: cancel, done: make(chan struct{})}
	r.subs[deviceID] = sub

	go func() {
		// defer 按 LIFO 执行：先 cancel 再 close(done)。
		// unsub() 在下方各 return 前同步执行，因此 done 关闭时 hub 侧订阅必然已注销。
		defer close(sub.done)
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				unsub()
				return
			case payload, ok := <-ch:
				if !ok {
					unsub()
					return
				}
				select {
				case <-ctx.Done():
					unsub()
					return
				case r.payloads <- payload:
				}
			}
		}
	}()
}

func (r *DataStreamRelay) Unsubscribe(deviceID string) {
	r.mu.Lock()
	sub, exists := r.subs[deviceID]
	if !exists {
		r.mu.Unlock()
		slog.Warn("DataStreamRelay Unsubscribe 跳过：设备未订阅", "component", "DataStreamRelay", "deviceID", deviceID)
		return
	}
	slog.Info("DataStreamRelay Unsubscribe", "component", "DataStreamRelay", "deviceID", deviceID)
	sub.cancel()
	delete(r.subs, deviceID)
	// 移入 retiring 集合：在锁外等待 sub.done 期间，并发的 Stop 能看到此 sub 并等待其完成。
	// 若不维护 retiring，Stop 拿到锁时 r.subs 已空，会跳过该订阅直接终止，
	// 违反"Stop 返回前所有 subscription 已注销"契约。
	r.retiring[sub] = struct{}{}
	r.mu.Unlock()

	// 在 relay mutex 外等待 worker 完成 hub 侧 unsub()：返回时订阅已确定注销。
	// worker 的取消路径只涉及内存操作，必然终止，无需超时兜底。
	<-sub.done

	// worker 已完成 unsub，从 retiring 移除。Stop 若在此期间已接管等待，
	// 它持有的 subs slice 仍包含此 sub 指针，会同样等 sub.done（已 close，立即返回）。
	// 因此这里 delete 是安全的，不会让 Stop 漏等。
	r.mu.Lock()
	delete(r.retiring, sub)
	r.mu.Unlock()
}

func (r *DataStreamRelay) Payloads() <-chan device.DataPayload {
	return r.payloads
}

// Stop 终止 relay：取消所有订阅，并在 relay mutex 外等待每个 worker 完成
// hub 侧 unsub() 后返回。Stop 是终止性的：返回后 Subscribe 一律被拒绝。
// Stop 幂等：重复/并发调用共享同一次终止，都会等待其完成，不重复 cancel。
//
// 等待集合 = r.subs + r.retiring：retiring 中的订阅是 Unsubscribe 已 cancel 但
// worker 仍在执行 unsub() 的中间态。Stop 必须等它们完成才能返回，否则违反
// "Stop 返回前所有 subscription 已注销"契约（worker 可能在 Stop 返回后才完成 unsub）。
// 接管后清空 retiring，避免后续 Unsubscribe 调用误操作已终止的集合。
func (r *DataStreamRelay) Stop() {
	r.mu.Lock()
	if r.stopped {
		done := r.stopDone
		r.mu.Unlock()
		<-done
		return
	}
	r.stopped = true

	count := len(r.subs) + len(r.retiring)
	slog.Info("DataStreamRelay Stop", "component", "DataStreamRelay", "activeSubscriptions", len(r.subs), "retiringSubscriptions", len(r.retiring))

	subs := make([]*relaySubscription, 0, count)
	for id, sub := range r.subs {
		sub.cancel()
		delete(r.subs, id)
		subs = append(subs, sub)
	}
	// 接管所有 retiring 订阅的等待：Unsubscribe 调用方仍在锁外等 sub.done，
	// 这里同时等不会冲突（done 关闭后所有等待者同时解除）。sub.cancel() 对已 cancel
	// 的 context 是幂等的，重复调用安全。
	for sub := range r.retiring {
		subs = append(subs, sub)
	}
	r.retiring = make(map[*relaySubscription]struct{})
	r.mu.Unlock()

	// 锁外等待所有 worker 完成 hub 侧 unsub()；payloads 满时 worker 的
	// 发送 select 会被 cancel 解除，不会永久阻塞。
	for _, sub := range subs {
		<-sub.done
	}
	close(r.stopDone)
}
