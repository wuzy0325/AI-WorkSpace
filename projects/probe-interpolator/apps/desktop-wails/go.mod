module probe-interpolator

go 1.20

require (
	ai-workspace/shared/algorithms/go/fivehole v0.0.0
	ai-workspace/shared/algorithms/go/sevenhole v0.0.0
	ai-workspace/shared/algorithms/go/threehole v0.0.0
	shared.local/device-sdk/go v0.0.0-00010101000000-000000000000
)

replace (
	ai-workspace/shared/algorithms/go/fivehole => ../../../../shared/algorithms/go/fivehole
	ai-workspace/shared/algorithms/go/sevenhole => ../../../../shared/algorithms/go/sevenhole
	ai-workspace/shared/algorithms/go/threehole => ../../../../shared/algorithms/go/threehole
	shared.local/device-sdk/go => ../../../../shared/device-sdk/go
)
