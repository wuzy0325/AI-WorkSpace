package ports

const (
	ChannelDAQSnapshot       = "daq:data-snapshot"
	ChannelDeviceStatus      = "device:status-updated"
	ChannelMotionStatus      = "motion:status-updated"
	ChannelCalibProgress     = "calibration:progress"
	ChannelCalibComplete     = "calibration:complete"
	ChannelCalibRealtime     = "calibration:realtime"
	ChannelTraversalProgress = "traversal:onProgress"
	ChannelTraversalComplete = "traversal:onComplete"
	ChannelTraversalError    = "traversal:onError"
)
