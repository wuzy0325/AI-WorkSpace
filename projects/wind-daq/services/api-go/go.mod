module wind-daq/services/api-go

go 1.25.0

require (
	ai-workspace/shared/algorithms/go/fivehole v0.0.0
	shared.local/device-sdk/go v0.0.0
	shared.local/motion-control/go v0.0.0
)

replace (
	ai-workspace/shared/algorithms/go/fivehole => ../../../../shared/algorithms/go/fivehole
	shared.local/device-sdk/go => ../../../../shared/device-sdk/go
	shared.local/motion-control/go => ../../../../shared/motion-control/go
)
