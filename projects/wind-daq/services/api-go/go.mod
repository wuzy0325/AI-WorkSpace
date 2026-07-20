module wind-daq/services/api-go

go 1.25.0

require (
	ai-workspace/shared/algorithms/go/fivehole v0.0.0
	ai-workspace/shared/algorithms/go/sevenhole v0.0.0
	golang.org/x/sys v0.43.0
	shared.local/device-sdk/go v0.0.0
	shared.local/motion-control/go v0.0.0
)

require golang.org/x/text v0.40.0

replace (
	ai-workspace/shared/algorithms/go/fivehole => ../../../../shared/algorithms/go/fivehole
	ai-workspace/shared/algorithms/go/sevenhole => ../../../../shared/algorithms/go/sevenhole
	shared.local/device-sdk/go => ../../../../shared/device-sdk/go
	shared.local/motion-control/go => ../../../../shared/motion-control/go
)
