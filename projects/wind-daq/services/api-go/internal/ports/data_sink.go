package ports

import "wind-daq/services/api-go/internal/core/device"

// ==================== 数据发布接口 ====================

// DataSink 数据接收回调函数类型
// 设备驱动通过此回调将采集数据推送出来
// 参数: payload 采集到的单帧数据
type DataSink func(payload device.DataPayload)

// DataPublisher 数据广播接口
// 将数据推送到前端或其他消费者
type DataPublisher interface {
	Broadcast(channel string, data interface{}) // 广播数据到指定通道
}
