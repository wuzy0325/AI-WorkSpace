package events

import (
	"log"
	"sync"
)

// Event 表示系统内部广播事件。
type Event struct {
	Type string
	Data any
}

// Bus 提供进程内发布订阅能力，用于解耦流程与推送。
type Bus struct {
	mu          sync.RWMutex
	nextID      int
	subscribers map[int]chan Event
}

// NewBus 创建事件总线。
func NewBus() *Bus {
	return &Bus{
		subscribers: make(map[int]chan Event),
	}
}

// Publish 广播事件到所有订阅者。
func (b *Bus) Publish(evt Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for id, ch := range b.subscribers {
		select {
		case ch <- evt:
		default:
			log.Printf("[bus] subscriber %d buffer full, dropping event: %s", id, evt.Type)
		}
	}
}

// Subscribe 注册订阅，返回事件通道与退订函数。
func (b *Bus) Subscribe() (<-chan Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := b.nextID
	b.nextID++

	ch := make(chan Event, 16)
	b.subscribers[id] = ch

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if c, ok := b.subscribers[id]; ok {
			delete(b.subscribers, id)
			close(c)
		}
	}

	return ch, unsubscribe
}

// GlobalBus 是全局事件总线单例，供 driver 和 api 包共同使用。
var GlobalBus = NewBus()
