package backend

import "errors"

// ErrDeviceNotFound 表示 GetStatus 等查询接口找不到对应设备状态。
// HTTP handler 据此返回 404，前端 deviceStore.ts 用 .catch(()=>false) 兼容此签名。
//
// Win7 分支：从原 (DeviceState, bool) 返回签名改为 (DeviceState, error) 后引入，
// 便于 HTTP handler 统一映射 404/500 状态码。
var ErrDeviceNotFound = errors.New("device not found")
