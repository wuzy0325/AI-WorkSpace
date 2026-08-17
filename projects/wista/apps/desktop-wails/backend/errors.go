package backend

import "errors"

// ErrDeviceNotFound 表示 GetStatus 等查询接口找不到对应设备状态。
// 前端的 deviceStore.ts 使用 `.catch(() => false)` 兼容此错误。
var ErrDeviceNotFound = errors.New("device not found")
