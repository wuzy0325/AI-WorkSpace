module wind-daq/apps/desktop-wails

go 1.20

require (
	shared.local/device-sdk/go v0.0.0
	wind-daq/services/api-go v0.0.0
)

require (
	ai-workspace/shared/algorithms/go/fivehole v0.0.0 // indirect
	shared.local/motion-control/go v0.0.0 // indirect
)

replace (
	ai-workspace/shared/algorithms/go/fivehole => ../../../../shared/algorithms/go/fivehole
	shared.local/device-sdk/go => ../../../../shared/device-sdk/go
	shared.local/motion-control/go => ../../../../shared/motion-control/go
	wind-daq/services/api-go => ../../services/api-go
)
