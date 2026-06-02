package profile

import sdkconfig "shared.local/device-sdk/go/motion/adapters/config"

type FileMotionProfileStore = sdkconfig.FileMotionProfileStore
type MemoryMotionProfileStore = sdkconfig.MemoryMotionProfileStore

var NewFileMotionProfileStore = sdkconfig.NewFileMotionProfileStore
var NewMemoryMotionProfileStore = sdkconfig.NewMemoryMotionProfileStore
