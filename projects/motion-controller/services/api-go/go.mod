module motion-controller/services/api-go

go 1.20

require (
	shared.local/device-sdk/go v0.0.0
	shared.local/motion-control/go v0.0.0
)

replace (
	shared.local/device-sdk/go => ../../../../shared/device-sdk/go
	shared.local/motion-control/go => ../../../../shared/motion-control/go
)
