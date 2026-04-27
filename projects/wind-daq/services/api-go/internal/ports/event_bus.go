package ports

// ==================== 事件总线接口 ====================
// 发布/订阅模式的事件系统
// 用于模块间松耦合通信

// EventBus 事件总线接口
// 支持主题订阅和发布
type EventBus interface {
	// Subscribe 订阅主题
	// 参数: topic 主题名称, handler 事件处理函数
	// 返回: func 取消订阅函数
	Subscribe(topic string, handler func(interface{})) func()
	// Publish 发布事件
	// 参数: topic 主题名称, event 事件数据
	Publish(topic string, event interface{})
}
