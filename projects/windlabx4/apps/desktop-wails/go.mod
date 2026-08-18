module windlabx4/apps/desktop-wails

go 1.20

require (
	shared.local/device-sdk/go v0.0.0
	windlabx4/services/api-go v0.0.0
)

require (
	ai-workspace/shared/algorithms/go/fivehole v0.0.0 // indirect
	ai-workspace/shared/algorithms/go/sevenhole v0.0.0 // indirect
	golang.org/x/sys v0.20.0 // indirect
	golang.org/x/text v0.16.0 // indirect
	shared.local/motion-control/go v0.0.0 // indirect
)

replace (
	ai-workspace/shared/algorithms/go/fivehole => ../../../../shared/algorithms/go/fivehole
	ai-workspace/shared/algorithms/go/sevenhole => ../../../../shared/algorithms/go/sevenhole
	shared.local/device-sdk/go => ../../../../shared/device-sdk/go
	shared.local/motion-control/go => ../../../../shared/motion-control/go
	windlabx4/services/api-go => ../../services/api-go
)
