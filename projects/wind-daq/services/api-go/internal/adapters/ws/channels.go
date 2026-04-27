package ws

import "wind-daq/services/api-go/internal/ports"

const (
	ChannelDAQSnapshot       = ports.ChannelDAQSnapshot
	ChannelDeviceStatus      = ports.ChannelDeviceStatus
	ChannelMotionStatus      = ports.ChannelMotionStatus
	ChannelCalibProgress     = ports.ChannelCalibProgress
	ChannelCalibComplete     = ports.ChannelCalibComplete
	ChannelCalibRealtime     = ports.ChannelCalibRealtime
	ChannelTraversalProgress = ports.ChannelTraversalProgress
	ChannelTraversalComplete = ports.ChannelTraversalComplete
	ChannelTraversalError    = ports.ChannelTraversalError
)

type Message struct {
	Channel string      `json:"channel"`
	Data    interface{} `json:"data"`
}

type SubscribeRequest struct {
	Type     string   `json:"type"`
	Channels []string `json:"channels"`
}
